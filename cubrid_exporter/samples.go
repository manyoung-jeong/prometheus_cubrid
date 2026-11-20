package main

import (
	"math"
	"time"
)

// Sample is one time series point: a metric's labels and value.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// SampleSet maps a metric name to its emitted samples, in DB order.
type SampleSet map[string][]Sample

// buildSamples is the binding layer: snapshot values -> metric samples. The
// schema names the metrics (the contract); this only says which snapshot value
// each metric reads. It mirrors the Node PoC's samples.js exactly.
func buildSamples(_ *Schema, snap *Snapshot, now time.Time) SampleSet {
	out := SampleSet{}
	push := func(name string, labels map[string]string, value float64, ok bool) {
		if !ok || math.IsNaN(value) {
			return
		}
		out[name] = append(out[name], Sample{Labels: labels, Value: value})
	}

	for _, db := range snap.Order {
		s := snap.PerDB[db]
		if s == nil {
			continue
		}
		c := s.Counters
		g := s.Gauges
		base := func() map[string]string { return map[string]string{"db": db} }
		ageSec := math.Round(now.Sub(snap.TS).Seconds())
		if ageSec < 0 {
			ageSec = 0
		}
		ctr := func(k string) (float64, bool) { v, ok := c[k]; return v, ok }
		gau := func(k string) (float64, bool) { v, ok := g[k]; return v, ok }

		// availability
		push("cubrid_up", base(), s.Up, true)
		push("cubrid_uptime_seconds", base(), s.Uptime, true)
		push("cubrid_snapshot_age_seconds", base(), ageSec, true)

		// database
		v, ok := ctr("Num_tran_commits")
		push("cubrid_xact_commits_total", base(), v, ok)
		v, ok = ctr("Num_tran_rollbacks")
		push("cubrid_xact_rollbacks_total", base(), v, ok)
		v, ok = ctr("Num_query_selects")
		push("cubrid_sql_selects_total", base(), v, ok)
		v, ok = ctr("Num_query_inserts")
		push("cubrid_sql_inserts_total", base(), v, ok)
		v, ok = ctr("Num_query_updates")
		push("cubrid_sql_updates_total", base(), v, ok)
		v, ok = ctr("Num_query_deletes")
		push("cubrid_sql_deletes_total", base(), v, ok)
		v, ok = ctr("Num_query_sscans")
		push("cubrid_seq_scans_total", base(), v, ok)
		v, ok = ctr("Num_query_iscans")
		push("cubrid_idx_scans_total", base(), v, ok)
		fetches, okF := ctr("Num_data_page_fetches")
		push("cubrid_blks_fetched_total", base(), fetches, okF)
		ioreads, okR := ctr("Num_data_page_ioreads")
		push("cubrid_blks_read_total", base(), ioreads, okR)
		if okF && okR {
			push("cubrid_blks_hit_total", base(), fetches-ioreads, true)
		}
		v, ok = ctr("Num_sort_io_pages")
		push("cubrid_temp_pages_total", base(), v, ok)

		// activity
		sa, okA := gau("sessions_active")
		push("cubrid_sessions", withState(db, "active"), sa, okA)
		si, okI := gau("sessions_idle")
		push("cubrid_sessions", withState(db, "idle"), si, okI)
		sw, okW := gau("sessions_waiting")
		push("cubrid_sessions", withState(db, "waiting"), sw, okW)
		if okA {
			total := sa
			if okI {
				total += si
			}
			if okW {
				total += sw
			}
			push("cubrid_sessions", withState(db, "total"), total, true)
		}
		v, ok = gau("max_clients")
		push("cubrid_max_clients", base(), v, ok)
		v, ok = gau("oldest_xact_seconds")
		push("cubrid_oldest_xact_seconds", base(), v, ok)

		// io
		v, ok = ctr("Num_data_page_ioreads")
		push("cubrid_data_pages_read_total", base(), v, ok)
		v, ok = ctr("Num_data_page_iowrites")
		push("cubrid_data_pages_written_total", base(), v, ok)
		v, ok = ctr("Num_log_page_ioreads")
		push("cubrid_log_pages_read_total", base(), v, ok)
		v, ok = ctr("Num_log_page_iowrites")
		push("cubrid_log_pages_written_total", base(), v, ok)
		v, ok = ctr("Num_file_ioreads")
		push("cubrid_file_reads_total", base(), v, ok)
		v, ok = ctr("Num_file_iowrites")
		push("cubrid_file_writes_total", base(), v, ok)
		v, ok = gau("page_size_bytes")
		push("cubrid_page_size_bytes", base(), v, ok)

		// lock
		v, ok = ctr("Num_object_locks_acquired")
		push("cubrid_locks_acquired_total", base(), v, ok)
		v, ok = ctr("Num_object_locks_waits")
		push("cubrid_locks_waits_total", base(), v, ok)
		if usec, okU := ctr("Num_object_locks_time_waited_usec"); okU {
			push("cubrid_lock_wait_seconds_total", base(), round3(usec/1e6), true)
		}
		v, ok = gau("lock_current_waiters")
		push("cubrid_lock_current_waiters", base(), v, ok)

		// bgwriter
		v, ok = ctr("Num_data_page_flushed")
		push("cubrid_pages_flushed_total", base(), v, ok)
		v, ok = ctr("Num_adaptive_flush_pages")
		push("cubrid_adaptive_flush_pages_total", base(), v, ok)
		v, ok = ctr("checkpoints")
		push("cubrid_checkpoints_total", base(), v, ok)

		// ha
		h := s.HA
		if h.HasNode {
			push("cubrid_ha_node_info", map[string]string{"db": db, "node": h.Node, "role": h.Role, "state": h.State}, 1, true)
			if h.HasDelay {
				withNode := func() map[string]string { return map[string]string{"db": db, "node": h.Node} }
				push("cubrid_ha_replication_delay_seconds", withNode(), h.ReplicationDelaySeconds, true)
				push("cubrid_ha_copy_log_lag_pages", withNode(), h.CopyLogLagPages, true)
				push("cubrid_ha_apply_log_lag_pages", withNode(), h.ApplyLogLagPages, true)
			}
		}

		// space
		sp := s.Space
		spv := func(k string) (float64, bool) { v, ok := sp[k]; return v, ok }
		v, ok = spv("total_bytes")
		push("cubrid_space_total_bytes", base(), v, ok)
		v, ok = spv("data_bytes")
		push("cubrid_space_data_bytes", base(), v, ok)
		v, ok = spv("index_bytes")
		push("cubrid_space_index_bytes", base(), v, ok)
		v, ok = spv("log_bytes")
		push("cubrid_space_log_bytes", base(), v, ok)
		v, ok = spv("free_bytes")
		push("cubrid_space_free_bytes", base(), v, ok)
		v, ok = spv("used_ratio")
		push("cubrid_space_used_ratio", base(), v, ok)
	}
	return out
}

func withState(db, state string) map[string]string {
	return map[string]string{"db": db, "state": state}
}

// round3 rounds to 3 decimals, matching the Node PoC's +(x).toFixed(3).
func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
