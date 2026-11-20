package main

import (
	"strings"
	"testing"
	"time"
)

// fixedSnap builds a deterministic snapshot independent of wall-clock elapsed,
// so assertions on values are stable.
func fixedSnap() *Snapshot {
	c := &Collector{startedAt: time.Now().Add(-10 * time.Second), seed: map[string]int64{"testdb": 1000, "demodb": 2000}}
	return &Snapshot{
		TS:    time.Now(),
		Order: []string{"testdb", "demodb"},
		PerDB: map[string]*dbSnap{"testdb": c.mock("testdb"), "demodb": c.mock("demodb")},
	}
}

func TestSchemaLoads(t *testing.T) {
	s, err := loadSchema()
	if err != nil {
		t.Fatal(err)
	}
	if s.Contract != "cubrid-open-api" || len(s.Groups) != 8 {
		t.Fatalf("unexpected schema: contract=%q groups=%d", s.Contract, len(s.Groups))
	}
	if n := len(allMetrics(s)); n < 40 {
		t.Fatalf("expected >=40 metrics, got %d", n)
	}
}

func TestExpositionShape(t *testing.T) {
	s, _ := loadSchema()
	out := renderOpenMetrics(s, buildSamples(s, fixedSnap(), time.Now()))
	// Every metric must carry HELP and TYPE lines.
	for _, m := range allMetrics(s) {
		if !strings.Contains(out, "# HELP "+m.Name+" ") {
			t.Errorf("missing HELP for %s", m.Name)
		}
		if !strings.Contains(out, "# TYPE "+m.Name+" "+m.Type) {
			t.Errorf("missing TYPE for %s", m.Name)
		}
	}
	// Spot-check key series and value formats.
	for _, want := range []string{
		`cubrid_up{db="testdb"} 1`,
		`cubrid_space_used_ratio{db="testdb"} 0.9375`,
		`cubrid_sessions{db="testdb",state="total"} `,
		`cubrid_ha_node_info{db="testdb",node="192.0.2.11",role="master",state="active"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q", want)
		}
	}
}

func TestBlksHitDerivation(t *testing.T) {
	s, _ := loadSchema()
	samples := buildSamples(s, fixedSnap(), time.Now())
	get := func(name string) float64 {
		for _, r := range samples[name] {
			if r.Labels["db"] == "testdb" {
				return r.Value
			}
		}
		t.Fatalf("no sample for %s", name)
		return 0
	}
	if h := get("cubrid_blks_hit_total"); h != get("cubrid_blks_fetched_total")-get("cubrid_blks_read_total") {
		t.Errorf("blks_hit must equal fetched-read, got %v", h)
	}
}

func TestJSONGroups(t *testing.T) {
	s, _ := loadSchema()
	snap := fixedSnap()
	j := buildJson(s, buildSamples(s, snap, time.Now()), snap, "testdb", "")
	dbs := j["databases"].(map[string]interface{})
	if _, ok := dbs["testdb"]; !ok {
		t.Fatal("testdb missing from JSON")
	}
	groups := dbs["testdb"].(map[string]interface{})
	if len(groups) != 8 {
		t.Errorf("expected 8 groups, got %d", len(groups))
	}
}

func TestParseStatdump(t *testing.T) {
	txt := "Num_tran_commits = 12345\n  Num_data_page_fetches   =   678\nUnrelated line = skip\nNum_future_counter = 9\n"
	m := parseStatdump(txt)
	if m["Num_tran_commits"] != 12345 || m["Num_data_page_fetches"] != 678 {
		t.Fatalf("parse failed: %+v", m)
	}
	// Unknown future counters are captured, not rejected (additions-only).
	if m["Num_future_counter"] != 9 {
		t.Errorf("future counter not parsed: %+v", m)
	}
}
