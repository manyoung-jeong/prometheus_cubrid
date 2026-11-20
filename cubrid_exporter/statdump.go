package main

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

// statLine matches a "Num_xxx = 123" counter line from `cubrid statdump`.
var statLine = regexp.MustCompile(`^\s*(Num_[A-Za-z0-9_]+)\s*=\s*([0-9]+)`)

// parseStatdump extracts Num_* counters from `cubrid statdump` text. Unknown
// lines are ignored, so new counters in future CUBRID versions never break the
// exporter (additions-only contract).
func parseStatdump(text string) map[string]float64 {
	out := map[string]float64{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		m := statLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		if v, err := strconv.ParseFloat(m[2], 64); err == nil {
			out[m[1]] = v
		}
	}
	return out
}
