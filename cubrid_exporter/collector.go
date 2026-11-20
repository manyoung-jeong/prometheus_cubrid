package main

import (
	"math"
	"sync"
	"time"
)

// haSnap holds the HA/replication fields for one database.
type haSnap struct {
	Node                    string
	Role                    string
	State                   string
	HasNode                 bool
	HasDelay                bool
	ReplicationDelaySeconds float64
	CopyLogLagPages         float64
	ApplyLogLagPages        float64
}

// dbSnap is one database's sampled values at a point in time.
type dbSnap struct {
	Up       float64
	Uptime   float64
	Counters map[string]float64
	Gauges   map[string]float64
	HA       haSnap
	Space    map[string]float64
}

// Snapshot is the cached result of one collection cycle across all databases.
// Order preserves the configured DB order so output is deterministic.
type Snapshot struct {
	TS    time.Time
	Order []string
	PerDB map[string]*dbSnap
}

// Collector periodically collects a snapshot and serves it from cache, so that
// /metrics and JSON requests never fork statdump per request (the low-overhead
// pattern validated in the DPM soak test).
type Collector struct {
	source    string
	dbs       []string
	refresh   time.Duration
	startedAt time.Time
	seed      map[string]int64

	mu   sync.RWMutex
	snap *Snapshot
	stop chan struct{}
}

// NewCollector builds a collector for the given source and databases.
func NewCollector(source string, dbs []string, refresh time.Duration) *Collector {
	seed := make(map[string]int64, len(dbs))
	for i, d := range dbs {
		seed[d] = int64(1000 * (i + 1))
	}
	return &Collector{
		source:    source,
		dbs:       dbs,
		refresh:   refresh,
		startedAt: time.Now(),
		seed:      seed,
		stop:      make(chan struct{}),
	}
}

// Start performs the first collection and launches the background refresh loop.
func (c *Collector) Start() *Collector {
	c.Refresh()
	go func() {
		t := time.NewTicker(c.refresh)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				c.Refresh()
			case <-c.stop:
				return
			}
		}
	}()
	return c
}

// Stop halts the background refresh loop.
func (c *Collector) Stop() { close(c.stop) }

// SnapshotNow returns the most recent cached snapshot (may be nil while warming up).
func (c *Collector) SnapshotNow() *Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snap
}

// Refresh collects every database once and replaces the cached snapshot.
func (c *Collector) Refresh() {
	perDB := make(map[string]*dbSnap, len(c.dbs))
	for _, db := range c.dbs {
		s, err := c.collectOne(db)
		if err != nil {
			// A collection error marks the DB down but keeps serving others.
			s = &dbSnap{Up: 0, Counters: map[string]float64{}, Gauges: map[string]float64{}, Space: map[string]float64{}}
		}
		perDB[db] = s
	}
	snap := &Snapshot{TS: time.Now(), Order: c.dbs, PerDB: perDB}
	c.mu.Lock()
	c.snap = snap
	c.mu.Unlock()
}

func (c *Collector) collectOne(db string) (*dbSnap, error) {
	if c.source == "statdump" {
		// Live: read real statistics from the running CUBRID server via
		// read-only CLI commands. The metric contract (schema) is unchanged.
		return c.collectLive(db)
	}
	return c.mock(db), nil
}

// mock produces deterministic data without a live DB. Counters grow in
// proportion to elapsed seconds so rate() works in a demo.
func (c *Collector) mock(db string) *dbSnap {
	elapsed := int64(math.Max(1, math.Floor(time.Since(c.startedAt).Seconds())))
	s := c.seed[db]
	grow := func(perSec float64) float64 {
		return float64(s + int64(math.Floor(perSec*float64(elapsed))))
	}
	fetches := grow(5200)
	ioreads := grow(85)

	node, role := "192.0.2.12", "slave"
	if db == "testdb" {
		node, role = "192.0.2.11", "master"
	}
	var replDelay, applyLag float64
	if db != "testdb" {
		replDelay = float64(elapsed % 2)
		applyLag = float64(elapsed % 4)
	}

	return &dbSnap{
		Up:     1,
		Uptime: float64(elapsed + 3600),
		Counters: map[string]float64{
			"Num_tran_commits":                  grow(42),
			"Num_tran_rollbacks":                grow(2),
			"Num_query_selects":                 grow(310),
			"Num_query_inserts":                 grow(90),
			"Num_query_updates":                 grow(55),
			"Num_query_deletes":                 grow(12),
			"Num_query_sscans":                  grow(40),
			"Num_query_iscans":                  grow(260),
			"Num_data_page_fetches":             fetches,
			"Num_data_page_ioreads":             ioreads,
			"Num_sort_io_pages":                 grow(3),
			"Num_data_page_iowrites":            grow(70),
			"Num_log_page_ioreads":              grow(6),
			"Num_log_page_iowrites":             grow(48),
			"Num_file_ioreads":                  grow(9),
			"Num_file_iowrites":                 grow(14),
			"Num_object_locks_acquired":         grow(520),
			"Num_object_locks_waits":            grow(4),
			"Num_object_locks_time_waited_usec": grow(900000),
			"Num_data_page_flushed":             grow(66),
			"Num_adaptive_flush_pages":          grow(40),
			"checkpoints":                       grow(0.02),
		},
		Gauges: map[string]float64{
			"sessions_active":      float64(8 + (elapsed % 5)),
			"sessions_idle":        float64(30 + (elapsed % 11)),
			"sessions_waiting":     float64(elapsed % 3),
			"max_clients":          100,
			"oldest_xact_seconds":  float64(2 + (elapsed % 7)),
			"page_size_bytes":      16384,
			"lock_current_waiters": float64(elapsed % 3),
		},
		HA: haSnap{
			Node:                    node,
			Role:                    role,
			State:                   "active",
			HasNode:                 true,
			HasDelay:                true,
			ReplicationDelaySeconds: replDelay,
			CopyLogLagPages:         0,
			ApplyLogLagPages:        applyLag,
		},
		Space: map[string]float64{
			"total_bytes": 2147483648,
			"data_bytes":  1610612736,
			"index_bytes": 268435456,
			"log_bytes":   134217728,
			"free_bytes":  134217728,
			"used_ratio":  0.9375,
		},
	}
}
