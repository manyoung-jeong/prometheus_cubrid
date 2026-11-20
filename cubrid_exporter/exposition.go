package main

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// escLabelValue escapes a label value per the OpenMetrics text format.
func escLabelValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return v
}

// fmtLabels renders labels in schema order first, then any remaining labels in
// sorted order for determinism.
func fmtLabels(order []string, labels map[string]string) string {
	parts := make([]string, 0, len(labels))
	seen := make(map[string]bool, len(order))
	for _, k := range order {
		if v, ok := labels[k]; ok {
			parts = append(parts, k+`="`+escLabelValue(v)+`"`)
			seen[k] = true
		}
	}
	extra := make([]string, 0)
	for k := range labels {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		parts = append(parts, k+`="`+escLabelValue(labels[k])+`"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// fmtNum renders a value the way the Node PoC does: integers without a decimal
// point, other values in shortest round-trip form.
func fmtNum(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// renderOpenMetrics serializes the samples in schema order with HELP/TYPE lines,
// producing Prometheus exposition text (version 0.0.4).
func renderOpenMetrics(s *Schema, samples SampleSet) string {
	var b strings.Builder
	for _, m := range allMetrics(s) {
		b.WriteString("# HELP " + m.Name + " " + m.Help + "\n")
		b.WriteString("# TYPE " + m.Name + " " + m.Type + "\n")
		for _, r := range samples[m.Name] {
			b.WriteString(m.Name + fmtLabels(m.Labels, r.Labels) + " " + fmtNum(r.Value) + "\n")
		}
	}
	return b.String()
}
