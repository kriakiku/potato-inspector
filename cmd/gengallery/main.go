//go:build gallery

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	neutralCodes = map[string]struct{}{"AP": {}, "AN": {}, "BY": {}, "RU": {}}
	destShort    = map[string]string{
		"cf":                 "CF",
		"aws-eu-central-1":   "FRA",
		"aws-eu-west-1":      "DUB",
		"aws-us-east-1":      "IAD",
		"aws-us-west-2":      "PDX",
		"aws-ap-southeast-1": "SIN",
		"aws-ap-northeast-1": "NRT",
		"aws-ap-south-1":     "BOM",
	}
)

func main() {
	root := findRoot()
	catalogPath := filepath.Join(root, "internal", "catalog", "data", "catalog.json")
	outPath := filepath.Join(root, "website", "content", "docs", "profiles-gallery.md")

	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		fatal(err)
	}
	var cat struct {
		GeneratedAt  string                    `json:"generatedAt"`
		Source       string                    `json:"source"`
		Destinations map[string]map[string]any `json:"destinations"`
		Countries    []map[string]any          `json:"countries"`
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		fatal(err)
	}

	dests := destOrder(cat.Destinations)
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: Profiles gallery\n")
	b.WriteString("weight: 70\n")
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "Generated from catalog `source=%s` at `%s`.\n\n", cat.Source, cat.GeneratedAt)
	b.WriteString("RTT columns are milliseconds to catalog destinations. ")
	b.WriteString("Last-mile one-way delay at runtime ≈ `(rtt_cf − host_cf) / 2`.\n\n")
	b.WriteString("Destinations: ")
	parts := make([]string, 0, len(dests))
	for _, d := range dests {
		label := d
		if m := cat.Destinations[d]; m != nil {
			if l, ok := m["label"].(string); ok && l != "" {
				label = l
			}
		}
		parts = append(parts, fmt.Sprintf("`%s` = %s", shortLabel(d, cat.Destinations), label))
	}
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(".\n\n")

	headers := []string{"Tier", "↓ Mbps", "↑ Mbps", "Loss %"}
	for _, d := range dests {
		headers = append(headers, shortLabel(d, cat.Destinations))
	}
	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = "---"
	}

	sort.Slice(cat.Countries, func(i, j int) bool {
		ni, _ := cat.Countries[i]["name"].(string)
		nj, _ := cat.Countries[j]["name"].(string)
		return ni < nj
	})

	for _, c := range cat.Countries {
		cid, _ := c["id"].(string)
		name, _ := c["name"].(string)
		if name == "" {
			name = cid
		}
		nearest, _ := c["nearestAws"].(string)
		title := fmt.Sprintf("## %s %s (`%s`)", flagEmoji(cid), name, cid)
		if nearest != "" {
			title += fmt.Sprintf(" · nearest AWS `%s`", shortLabel(nearest, cat.Destinations))
		}
		b.WriteString(title)
		b.WriteString("\n\n")
		b.WriteString("| " + strings.Join(headers, " | ") + " |\n")
		b.WriteString("| " + strings.Join(sep, " | ") + " |\n")

		tiers, _ := c["tiers"].(map[string]any)
		for _, tier := range []string{"stable", "typical", "poor"} {
			t, _ := tiers[tier].(map[string]any)
			if t == nil {
				t = map[string]any{}
			}
			rtt, _ := t["rttToDest"].(map[string]any)
			cells := []string{
				tier,
				fmtAny(t["downloadMbps"]),
				fmtAny(t["uploadMbps"]),
				fmtAny(t["lossPercent"]),
			}
			for _, d := range dests {
				if rtt == nil {
					cells = append(cells, "—")
					continue
				}
				cells = append(cells, fmtAny(rtt[d]))
			}
			b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
		b.WriteString("\n")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(outPath, []byte(b.String()), 0o644); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", outPath)
}

func findRoot() string {
	wd, _ := os.Getwd()
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return wd
		}
		dir = parent
	}
}

func destOrder(destinations map[string]map[string]any) []string {
	preferred := []string{
		"cf", "aws-eu-central-1", "aws-eu-west-1", "aws-us-east-1",
		"aws-us-west-2", "aws-ap-southeast-1", "aws-ap-northeast-1", "aws-ap-south-1",
	}
	var out []string
	seen := map[string]struct{}{}
	for _, k := range preferred {
		if _, ok := destinations[k]; ok {
			out = append(out, k)
			seen[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(destinations))
	for k := range destinations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := seen[k]; !ok {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return preferred
	}
	return out
}

func shortLabel(id string, destinations map[string]map[string]any) string {
	if s, ok := destShort[id]; ok {
		return s
	}
	if m := destinations[id]; m != nil {
		if r, ok := m["awsRegion"].(string); ok && r != "" {
			return r
		}
	}
	return id
}

func flagEmoji(code string) string {
	cc := strings.ToUpper(strings.TrimSpace(code))
	if _, ok := neutralCodes[cc]; ok {
		return "●"
	}
	if len(cc) != 2 {
		return "●"
	}
	for _, c := range cc {
		if c < 'A' || c > 'Z' {
			return "●"
		}
	}
	r1 := rune(0x1F1E6 + int(cc[0]-'A'))
	r2 := rune(0x1F1E6 + int(cc[1]-'A'))
	return string([]rune{r1, r2})
}

func fmtAny(v any) string {
	if v == nil {
		return "—"
	}
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%g", x)
		}
		return fmt.Sprintf("%v", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
