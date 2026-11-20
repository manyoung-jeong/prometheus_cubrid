// Package main implements the CUBRID metrics exporter: a single-binary
// sidecar that exposes CUBRID runtime statistics as OpenMetrics (/metrics,
// for Prometheus) and JSON (/v1/stat/*, for SI integrations), at PostgreSQL
// pg_stat_* parity. It is a Go port of the Node.js PoC under open_api/, and
// shares one source of truth: schema/metrics.v1.json.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// schemaJSON is the metric contract (single source of truth), embedded into
// the binary so the exporter ships as one self-contained file.
//
//go:embed schema/metrics.v1.json
var schemaJSON []byte

// Metric is one metric in the contract. The last three fields are injected
// from the owning group when the schema is flattened.
type Metric struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Unit   string   `json:"unit"`
	Labels []string `json:"labels"`
	Source string   `json:"source"`
	PG     string   `json:"pg"`
	Help   string   `json:"help"`
	HelpKo string   `json:"help_ko"`

	Group  string `json:"-"`
	View   string `json:"-"`
	PGView string `json:"-"`
}

// Group is a view-level grouping of metrics (one pg_stat_* equivalent).
type Group struct {
	ID      string   `json:"id"`
	View    string   `json:"view"`
	PGView  string   `json:"pg_view"`
	TitleKo string   `json:"title_ko"`
	Metrics []Metric `json:"metrics"`
}

// Schema is the whole metric contract.
type Schema struct {
	Contract  string   `json:"contract"`
	Version   string   `json:"version"`
	Namespace string   `json:"namespace"`
	Groups    []Group  `json:"groups"`
	Notes     []string `json:"notes"`
}

// loadSchema parses the embedded contract.
func loadSchema() (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(schemaJSON, &s); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	return &s, nil
}

// allMetrics flattens the schema to an ordered slice, preserving group and
// metric order (the OpenMetrics output order depends on it).
func allMetrics(s *Schema) []Metric {
	out := make([]Metric, 0, 64)
	for _, g := range s.Groups {
		for _, m := range g.Metrics {
			m.Group = g.ID
			m.View = g.View
			m.PGView = g.PGView
			out = append(out, m)
		}
	}
	return out
}
