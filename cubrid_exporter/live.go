package main

import (
	"context"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// runCmd runs a read-only CUBRID command with a timeout and returns its output.
func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// collectLive gathers one database's real statistics from the running CUBRID
// server using only read-only CLI commands (statdump, spacedb, tranlist,
// heartbeat). The metric contract is unchanged; this just fills the snapshot.
// If the server is unreachable the DB degrades to cubrid_up=0 (no crash).
func (c *Collector) collectLive(db string) (*dbSnap, error) {
	target := db + "@localhost"
	s := &dbSnap{
		Counters: map[string]float64{},
		Gauges:   map[string]float64{},
		Space:    map[string]float64{},
	}

	// --- statdump: global counters (database/io/lock/bgwriter) ---
	out, err := runCmd(20*time.Second, "cubrid", "statdump", target)
	if err != nil || !strings.Contains(out, "Num_") {
		s.Up = 0
		return s, nil
	}
	s.Up = 1
	s.Counters = parseStatdump(out)
	if v, ok := s.Counters["Num_log_end_checkpoints"]; ok {
		s.Counters["checkpoints"] = v
	}
	s.Gauges["page_size_bytes"] = 16384

	// --- spacedb -s: storage summary ---
	if so, e := runCmd(20*time.Second, "cubrid", "spacedb", "-s", target); e == nil {
		parseSpacedb(so, s)
	}

	// --- paramdump: configured limits (max_clients) ---
	if po, e := runCmd(15*time.Second, "cubrid", "paramdump", target); e == nil {
		if mc := parseParam(po, "max_clients"); mc > 0 {
			s.Gauges["max_clients"] = mc
		}
	}

	// --- tranlist: active sessions, oldest transaction age, lock waiters ---
	if to, e := runCmd(15*time.Second, "cubrid", "tranlist", target); e == nil {
		n, oldest, waiters := parseTranlist(to)
		if n >= 0 {
			s.Gauges["sessions_active"] = float64(n)
		}
		s.Gauges["oldest_xact_seconds"] = oldest
		s.Gauges["lock_current_waiters"] = float64(waiters)
	}

	// --- heartbeat + applyinfo: HA role/state and replication delay (HA only) ---
	parseHeartbeat(db, s)

	return s, nil
}

var paramLine = regexp.MustCompile(`(?m)^\s*([a-z_]+)\s*=\s*([0-9]+)`)

// parseParam reads an integer server parameter from `cubrid paramdump` output.
func parseParam(text, key string) float64 {
	for _, m := range paramLine.FindAllStringSubmatch(text, -1) {
		if m[1] == key {
			if v, err := strconv.ParseFloat(m[2], 64); err == nil {
				return v
			}
		}
	}
	return 0
}

// parseTranlist extracts the active-transaction count, the longest running
// transaction's age (max "Tran time"), and the number of sessions currently
// waiting on a lock ("Wait for lock holder" != -1) from `cubrid tranlist`.
func parseTranlist(text string) (sessions int, oldest float64, waiters int) {
	for _, ln := range strings.Split(text, "\n") {
		if !tranRow.MatchString(ln) {
			continue
		}
		sessions++
		f := strings.Fields(ln)
		// index user host pid program query_time tran_time wait_for_lock ...
		if len(f) >= 8 {
			if t, err := strconv.ParseFloat(f[6], 64); err == nil && t > oldest {
				oldest = t
			}
			if f[7] != "-1" {
				waiters++
			}
		}
	}
	return sessions, oldest, waiters
}

var (
	tranRow   = regexp.MustCompile(`^\s*\d+\(`)
	hbCurrent = regexp.MustCompile(`current (\S+), state (\w+)`)
)

// toBytes converts a "3.0 G" style size to whole bytes (rounded, so byte
// gauges render as integers rather than in exponential notation).
func toBytes(num, unit string) float64 {
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(unit) {
	case "T":
		v *= 1024 * 1024 * 1024 * 1024
	case "G":
		v *= 1024 * 1024 * 1024
	case "M":
		v *= 1024 * 1024
	case "K":
		v *= 1024
	}
	return math.Round(v)
}

// parseSpacedb fills space.* from `cubrid spacedb -s` summary output. The grand
// total row begins with "-"; the "PERMANENT DATA" row gives the data size.
func parseSpacedb(text string, s *dbSnap) {
	for _, ln := range strings.Split(text, "\n") {
		f := strings.Fields(ln)
		if len(f) < 4 {
			continue
		}
		// Grand total row: "-  -  <count> <used> <u> <free> <u> <total> <u>"
		if f[0] == "-" {
			if used, free, total, ok := lastThreeSizes(f); ok {
				s.Space["total_bytes"] = total
				s.Space["free_bytes"] = free
				if total > 0 {
					s.Space["used_ratio"] = round3(used / total)
				}
			}
		}
		// Data volume row.
		if strings.Contains(ln, "PERMANENT DATA") {
			if used, _, _, ok := lastThreeSizes(f); ok {
				s.Space["data_bytes"] = used
			}
		}
	}
}

// lastThreeSizes parses the trailing used/free/total size columns of a spacedb
// row (each column is "<number> <unit>").
func lastThreeSizes(f []string) (used, free, total float64, ok bool) {
	// Collect (number, unit) pairs across the row.
	type pair struct{ num, unit string }
	var pairs []pair
	for i := 0; i < len(f)-1; i++ {
		if _, err := strconv.ParseFloat(f[i], 64); err == nil {
			u := f[i+1]
			if u == "T" || u == "G" || u == "M" || u == "K" || u == "B" {
				pairs = append(pairs, pair{f[i], u})
			}
		}
	}
	if len(pairs) < 3 {
		return 0, 0, 0, false
	}
	n := len(pairs)
	used = toBytes(pairs[n-3].num, pairs[n-3].unit)
	free = toBytes(pairs[n-2].num, pairs[n-2].unit)
	total = toBytes(pairs[n-1].num, pairs[n-1].unit)
	return used, free, total, true
}

// parseHeartbeat sets HA role/state for a database if it is managed by HA, and
// fills replication delay from applyinfo on the copied log path.
func parseHeartbeat(db string, s *dbSnap) {
	out, err := runCmd(15*time.Second, "cubrid", "heartbeat", "status")
	if err != nil {
		return
	}
	// This database is HA-managed only if a "Server <db>" line is present.
	if !regexp.MustCompile(`Server ` + regexp.QuoteMeta(db) + `\b`).MatchString(out) {
		return
	}
	node, role := "", "unknown"
	if m := hbCurrent.FindStringSubmatch(out); m != nil {
		node, role = m[1], m[2]
	}
	state := "active"
	if strings.Contains(out, "Server "+db+" (pid") && !strings.Contains(out, "registered_and_active") {
		state = "standby"
	}
	s.HA = haSnap{Node: node, Role: role, State: state, HasNode: true}

	// Copied-log path from the Copylogdb line, e.g.
	//   Copylogdb <db>@<peer>:/path/to/<db>_<peer>
	cp := regexp.MustCompile(`Copylogdb ` + regexp.QuoteMeta(db) + `@\S+?:(\S+)`).FindStringSubmatch(out)
	if cp != nil && node != "" {
		collectApplyinfo(db, node, cp[1], s)
	}
}

var (
	aiPageCount = regexp.MustCompile(`Delayed log page count\s*:\s*(-?[0-9]+)`)
	aiDelaySec  = regexp.MustCompile(`Estimated Delay\s*:\s*([0-9]+)`)
)

// collectApplyinfo fills HA copy/apply lag and replication delay from
// `cubrid applyinfo`. "- second(s)" (no delay) parses as 0.
func collectApplyinfo(db, remoteHost, copiedLogPath string, s *dbSnap) {
	out, err := runCmd(15*time.Second, "cubrid", "applyinfo", "-a", "-r", remoteHost, "-L", copiedLogPath, db)
	if err != nil {
		return
	}
	// Two "Delayed log page count" blocks: [0]=copy lag, [1]=apply lag.
	pages := aiPageCount.FindAllStringSubmatch(out, -1)
	if len(pages) >= 2 {
		s.HA.CopyLogLagPages = math.Max(0, atof(pages[0][1]))
		s.HA.ApplyLogLagPages = math.Max(0, atof(pages[1][1]))
	}
	// Replication delay = apply-side estimated delay (0 when "- second(s)").
	delays := aiDelaySec.FindAllStringSubmatch(out, -1)
	if len(delays) >= 2 {
		s.HA.ReplicationDelaySeconds = atof(delays[1][1])
	}
	s.HA.HasDelay = true
}

func atof(x string) float64 {
	v, _ := strconv.ParseFloat(x, 64)
	return v
}
