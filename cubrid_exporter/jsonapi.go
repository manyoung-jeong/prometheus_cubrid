package main

import (
	"strings"
	"time"
)

// shortKey turns cubrid_xxx_total into xxx for the JSON view.
func shortKey(name string) string {
	name = strings.TrimPrefix(name, "cubrid_")
	name = strings.TrimSuffix(name, "_total")
	return name
}

// buildJson builds the JSON view for GET /v1/stat/all and /v1/stat/<group>.
// It is derived from the same schema and samples as /metrics (no drift).
func buildJson(s *Schema, samples SampleSet, snap *Snapshot, db, group string) map[string]interface{} {
	metrics := allMetrics(s)
	var dbs []string
	if db != "" {
		dbs = []string{db}
	} else {
		dbs = snap.Order
	}

	databases := map[string]interface{}{}
	for _, d := range dbs {
		if snap.PerDB[d] == nil {
			continue
		}
		groups := map[string]interface{}{}
		for _, m := range metrics {
			if group != "" && m.Group != group {
				continue
			}
			var rows []Sample
			for _, r := range samples[m.Name] {
				if r.Labels["db"] == d {
					rows = append(rows, r)
				}
			}
			if len(rows) == 0 {
				continue
			}
			grp, ok := groups[m.Group].(map[string]interface{})
			if !ok {
				grp = map[string]interface{}{
					"view":    m.View,
					"pg_view": m.PGView,
					"metrics": map[string]interface{}{},
				}
				groups[m.Group] = grp
			}
			gm := grp["metrics"].(map[string]interface{})
			key := shortKey(m.Name)

			extra := 0
			for _, l := range m.Labels {
				if l != "db" {
					extra++
				}
			}
			if extra == 0 && len(rows) == 1 {
				gm[key] = map[string]interface{}{
					"value": rows[0].Value, "type": m.Type, "unit": m.Unit, "source": m.Source,
				}
			} else {
				series := make([]map[string]interface{}, 0, len(rows))
				for _, r := range rows {
					lab := map[string]string{}
					for k, v := range r.Labels {
						if k != "db" {
							lab[k] = v
						}
					}
					series = append(series, map[string]interface{}{"labels": lab, "value": r.Value})
				}
				gm[key] = map[string]interface{}{
					"type": m.Type, "unit": m.Unit, "source": m.Source, "series": series,
				}
			}
		}
		databases[d] = groups
	}

	return map[string]interface{}{
		"contract":      s.Contract,
		"version":       s.Version,
		"snapshot_time": snap.TS.UTC().Format(time.RFC3339),
		"databases":     databases,
	}
}
