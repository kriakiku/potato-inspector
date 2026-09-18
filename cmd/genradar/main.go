//go:build radar

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "embed"
)

//go:embed data/country_meta.json
var countryMetaJSON []byte

//go:embed data/seed_lastmile.json
var seedLastmileJSON []byte

type awsDest struct {
	Label  string
	Region string
	Lat    float64
	Lon    float64
	Target string
}

var awsDests = map[string]awsDest{
	"aws-eu-central-1":   {Label: "AWS Frankfurt", Region: "eu-central-1", Lat: 50.11, Lon: 8.68, Target: "ec2.eu-central-1.amazonaws.com"},
	"aws-eu-west-1":      {Label: "AWS Ireland", Region: "eu-west-1", Lat: 53.35, Lon: -6.26, Target: "ec2.eu-west-1.amazonaws.com"},
	"aws-us-east-1":      {Label: "AWS N. Virginia", Region: "us-east-1", Lat: 39.04, Lon: -77.49, Target: "ec2.us-east-1.amazonaws.com"},
	"aws-us-west-2":      {Label: "AWS Oregon", Region: "us-west-2", Lat: 45.87, Lon: -119.69, Target: "ec2.us-west-2.amazonaws.com"},
	"aws-ap-southeast-1": {Label: "AWS Singapore", Region: "ap-southeast-1", Lat: 1.35, Lon: 103.82, Target: "ec2.ap-southeast-1.amazonaws.com"},
	"aws-ap-northeast-1": {Label: "AWS Tokyo", Region: "ap-northeast-1", Lat: 35.68, Lon: 139.69, Target: "ec2.ap-northeast-1.amazonaws.com"},
	"aws-ap-south-1":     {Label: "AWS Mumbai", Region: "ap-south-1", Lat: 19.08, Lon: 72.88, Target: "ec2.ap-south-1.amazonaws.com"},
}

type countryMeta struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type seedLM struct {
	Down  float64 `json:"down"`
	Up    float64 `json:"up"`
	Loss  float64 `json:"loss"`
	RttCF int     `json:"rtt_cf"`
}

func main() {
	root := findRoot()
	meta := map[string]countryMeta{}
	seed := map[string]seedLM{}
	mustJSON(countryMetaJSON, &meta)
	mustJSON(seedLastmileJSON, &seed)

	token := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN"))
	cloudpingURL := getenv("CLOUDPING_URL", "https://www.cloudping.co/api/latencies")

	if token != "" {
		fmt.Fprintln(os.Stderr, "CLOUDFLARE_API_TOKEN set — using Radar last-mile")
	} else {
		fmt.Fprintln(os.Stderr, "CLOUDFLARE_API_TOKEN missing — seed last-mile only")
	}

	fmt.Fprintf(os.Stderr, "fetching CloudPing matrix… (%s)\n", cloudpingURL)
	matrix, err := fetchCloudping(cloudpingURL)
	source := "seed+geo"
	if err != nil {
		fmt.Fprintf(os.Stderr, "cloudping failed (%v), using geo backbone\n", err)
		matrix = map[string]map[string]float64{}
		if token != "" {
			source = "radar+geo"
		}
	} else {
		fmt.Fprintf(os.Stderr, "cloudping regions: %d\n", len(matrix))
		if token != "" {
			source = "radar+cloudping"
		} else {
			source = "seed+cloudping"
		}
	}

	countries := countryList(token, meta)
	fmt.Fprintf(os.Stderr, "countries to process: %d\n", len(countries))

	var outCountries []map[string]any
	radarOK, radarFail, seedOK := 0, 0, 0
	total := len(countries)
	for i, c := range countries {
		lat, lon := c.lat, c.lon
		if lat == 0 && lon == 0 {
			if m, ok := meta[c.code]; ok {
				lat, lon = m.Lat, m.Lon
			}
		}
		down, up, loss, rttCF := 40.0, 15.0, 0.4, 40
		if s, ok := seed[c.code]; ok {
			down, up, loss, rttCF = s.Down, s.Up, s.Loss, s.RttCF
		}
		rttSource := "seed"

		if token != "" {
			if summary, err := radarSummary(token, c.code); err == nil && summary != nil {
				if v, ok := asFloat(summary["bandwidthDownload"]); ok {
					down = v
				}
				if v, ok := asFloat(summary["bandwidthUpload"]); ok {
					up = v
				}
				if v, ok := asFloat(summary["packetLoss"]); ok {
					loss = v
				}
				if v, ok := asFloat(summary["latencyIdle"]); ok {
					rttCF = max(1, int(math.Round(v)))
				}
				rttSource = "radar"
				radarOK++
			} else {
				radarFail++
				fmt.Fprintf(os.Stderr, "  [%d/%d] %s radar miss → seed\n", i+1, total, c.code)
			}
			time.Sleep(150 * time.Millisecond)
		} else {
			seedOK++
		}

		nearestID := nearestAWS(lat, lon)
		nearestRegion := awsDests[nearestID].Region
		rtt := map[string]int{"cf": max(1, rttCF)}
		for destID, meta := range awsDests {
			bb := backboneMS(matrix, nearestRegion, meta.Region)
			rtt[destID] = max(rtt["cf"], int(math.Round(float64(rtt["cf"])+bb)))
		}

		outCountries = append(outCountries, map[string]any{
			"id":         c.code,
			"name":       c.name,
			"nearestAws": nearestID,
			"rttSource":  rttSource,
			"tiers":      buildTiers(down, up, loss, rtt),
		})

		if token != "" || i == 0 || i+1 == total || (i+1)%25 == 0 {
			fmt.Fprintf(os.Stderr, "  [%d/%d] %s %s · %s · ↓%.0f/↑%.0f loss=%.2f%% cf=%dms · nearest=%s\n",
				i+1, total, c.code, c.name, rttSource, down, up, loss, rtt["cf"], nearestID)
		}
	}

	catalog := map[string]any{
		"generatedAt":  time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"source":       source,
		"destinations": buildDestinations(),
		"countries":    outCountries,
	}
	text, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		fatal(err)
	}
	text = append(text, '\n')

	out := filepath.Join(root, "catalog.json")
	embed := filepath.Join(root, "internal", "catalog", "data", "catalog.json")
	mustWrite(out, text)
	mustWrite(embed, text)
	fmt.Fprintf(os.Stderr, "done: %d countries · source=%s · radar_ok=%d radar_fail=%d seed=%d\n",
		len(outCountries), source, radarOK, radarFail, seedOK)
	fmt.Fprintf(os.Stderr, "wrote %s\n", out)
	fmt.Fprintf(os.Stderr, "wrote %s\n", embed)
}

type countryRow struct {
	code, name string
	lat, lon   float64
}

func countryList(token string, meta map[string]countryMeta) []countryRow {
	if token != "" {
		locs, err := radarLocations(token)
		if err == nil && len(locs) > 0 {
			var out []countryRow
			for _, loc := range locs {
				code := strings.ToUpper(fmt.Sprint(loc["alpha2"]))
				if len(code) != 2 {
					continue
				}
				name := fmt.Sprint(loc["name"])
				if name == "" || name == "<nil>" {
					name = code
				}
				lat, lon := 0.0, 0.0
				if m, ok := meta[code]; ok {
					lat, lon = m.Lat, m.Lon
				} else {
					lat, _ = asFloat(loc["latitude"])
					lon, _ = asFloat(loc["longitude"])
				}
				out = append(out, countryRow{code, name, lat, lon})
			}
			if len(out) > 0 {
				sort.Slice(out, func(i, j int) bool { return out[i].code < out[j].code })
				fmt.Fprintf(os.Stderr, "radar locations usable: %d\n", len(out))
				return out
			}
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "radar locations failed: %v\n", err)
		}
	}
	fmt.Fprintf(os.Stderr, "using COUNTRY_META seed list (%d countries)\n", len(meta))
	codes := make([]string, 0, len(meta))
	for c := range meta {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	out := make([]countryRow, 0, len(codes))
	for _, c := range codes {
		m := meta[c]
		out = append(out, countryRow{c, m.Name, m.Lat, m.Lon})
	}
	return out
}

func buildDestinations() map[string]any {
	dests := map[string]any{
		"cf": map[string]any{
			"label": "Cloudflare Edge", "calibrate": "cf", "target": "speed.cloudflare.com",
		},
	}
	for id, m := range awsDests {
		dests[id] = map[string]any{
			"label": m.Label, "calibrate": id, "target": m.Target, "awsRegion": m.Region,
		}
	}
	return dests
}

func buildTiers(down, up, loss float64, rtt map[string]int) map[string]any {
	return map[string]any{
		"stable": map[string]any{
			"downloadMbps": round1(down * 1.25),
			"uploadMbps":   round1(up * 1.25),
			"lossPercent":  round2(math.Max(0.05, loss*0.3)),
			"rttToDest":    tierRTT(rtt, 0.85),
		},
		"typical": map[string]any{
			"downloadMbps": round1(down),
			"uploadMbps":   round1(up),
			"lossPercent":  round2(loss),
			"rttToDest":    copyIntMap(rtt),
		},
		"poor": map[string]any{
			"downloadMbps": round1(math.Max(1.5, down*0.2)),
			"uploadMbps":   round1(math.Max(0.5, up*0.2)),
			"lossPercent":  round2(math.Max(1.0, loss*3)),
			"rttToDest":    tierRTT(rtt, 1.4),
		},
	}
}

func tierRTT(base map[string]int, mult float64) map[string]int {
	out := make(map[string]int, len(base))
	for k, v := range base {
		out[k] = max(1, int(math.Round(float64(v)*mult)))
	}
	return out
}

func nearestAWS(lat, lon float64) string {
	best, bestD := "aws-eu-central-1", 1e18
	for id, m := range awsDests {
		d := haversineKM(lat, lon, m.Lat, m.Lon)
		if d < bestD {
			best, bestD = id, d
		}
	}
	return best
}

func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dphi := (lat2 - lat1) * math.Pi / 180
	dlmb := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dphi/2)*math.Sin(dphi/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dlmb/2)*math.Sin(dlmb/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

func geoBackboneMS(a, b awsDest) float64 {
	km := haversineKM(a.Lat, a.Lon, b.Lat, b.Lon)
	return (2 * km / 200.0) * 1.5
}

func fetchCloudping(url string) (map[string]map[string]float64, error) {
	raw, err := httpJSON(url, nil)
	if err != nil {
		return nil, err
	}
	data, _ := raw["data"].(map[string]any)
	out := map[string]map[string]float64{}
	for src, rowAny := range data {
		row, ok := rowAny.(map[string]any)
		if !ok {
			continue
		}
		out[src] = map[string]float64{}
		for dst, val := range row {
			if f, ok := asFloat(val); ok {
				out[src][dst] = f
			}
		}
	}
	return out, nil
}

func backboneMS(matrix map[string]map[string]float64, srcRegion, dstRegion string) float64 {
	if srcRegion == dstRegion {
		return 0
	}
	if row := matrix[srcRegion]; row != nil {
		if v, ok := row[dstRegion]; ok {
			return math.Max(0, v)
		}
	}
	if row := matrix[dstRegion]; row != nil {
		if v, ok := row[srcRegion]; ok {
			return math.Max(0, v)
		}
	}
	var srcMeta, dstMeta *awsDest
	for _, m := range awsDests {
		mm := m
		if m.Region == srcRegion {
			srcMeta = &mm
		}
		if m.Region == dstRegion {
			dstMeta = &mm
		}
	}
	if srcMeta != nil && dstMeta != nil {
		return geoBackboneMS(*srcMeta, *dstMeta)
	}
	return 80
}

func radarLocations(token string) ([]map[string]any, error) {
	url := "https://api.cloudflare.com/client/v4/radar/entities/locations?format=json&limit=500"
	raw, err := httpJSON(url, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return nil, err
	}
	result, _ := raw["result"].(map[string]any)
	locs, _ := result["locations"].([]any)
	out := make([]map[string]any, 0, len(locs))
	for _, l := range locs {
		if m, ok := l.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func radarSummary(token, country string) (map[string]any, error) {
	url := "https://api.cloudflare.com/client/v4/radar/quality/speed/summary?location=" + country + "&format=json"
	raw, err := httpJSON(url, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return nil, err
	}
	if ok, _ := raw["success"].(bool); !ok {
		return nil, fmt.Errorf("radar unsuccessful")
	}
	result, _ := raw["result"].(map[string]any)
	summary, _ := result["summary_0"].(map[string]any)
	return summary, nil
}

func httpJSON(url string, headers map[string]string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
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

func mustJSON(b []byte, dest any) {
	if err := json.Unmarshal(b, dest); err != nil {
		fatal(err)
	}
}

func mustWrite(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatal(err)
	}
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		var f float64
		_, err := fmt.Sscan(x, &f)
		return f, err == nil
	default:
		return 0, false
	}
}

func copyIntMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
