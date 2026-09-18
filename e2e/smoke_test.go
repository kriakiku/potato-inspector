//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	defaultAPI       = "http://127.0.0.1:7783"
	defaultContainer = "potatonetwork-e2e"
	pathDelayMs      = 250 // testdata/e2e/data/rules.expr
)

func apiBase() string {
	if v := strings.TrimSpace(os.Getenv("POTATONETWORK_E2E_API")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultAPI
}

func containerName() string {
	if v := strings.TrimSpace(os.Getenv("POTATONETWORK_E2E_CONTAINER")); v != "" {
		return v
	}
	return defaultContainer
}

func TestE2E_APIHealth(t *testing.T) {
	waitHealthy(t, 60*time.Second)
	code, body := apiGET(t, "/v1/health")
	if code != 200 {
		t.Fatalf("health status=%d body=%s", code, body)
	}
	var h map[string]any
	if err := json.Unmarshal([]byte(body), &h); err != nil {
		t.Fatal(err)
	}
	if h["ok"] != true {
		t.Fatalf("health not ok: %s", body)
	}
}

func TestE2E_APIExemptUnderShape(t *testing.T) {
	waitHealthy(t, 60*time.Second)
	putBaseline(t)
	putProfile(t, "AF", "typical") // high CF RTT → measurable last-mile

	// Control-plane must stay fast while shaping is on.
	start := time.Now()
	code, _ := apiGET(t, "/v1/health")
	elapsed := time.Since(start)
	if code != 200 {
		t.Fatalf("health status=%d", code)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("API health took %s under shape (want <500ms) — likely not exempt", elapsed)
	}
}

func TestE2E_ProfileAppliesLossAndDelayFields(t *testing.T) {
	waitHealthy(t, 60*time.Second)
	putBaseline(t)
	putProfile(t, "AF", "poor")

	code, body := apiGET(t, "/v1/profile")
	if code != 200 {
		t.Fatalf("profile status=%d body=%s", code, body)
	}
	var p struct {
		Country     string  `json:"country"`
		Tier        string  `json:"tier"`
		DelayMs     int     `json:"delayMs"`
		LossPercent float64 `json:"lossPercent"`
		Passthrough bool    `json:"passthrough"`
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	if p.Passthrough || p.Country != "AF" || p.Tier != "poor" {
		t.Fatalf("unexpected profile: %+v", p)
	}
	if p.DelayMs < 10 {
		t.Fatalf("DelayMs=%d too small for AF/poor with baseline cf=5", p.DelayMs)
	}
	if p.LossPercent <= 0 {
		t.Fatalf("LossPercent=%v want >0", p.LossPercent)
	}
}

func TestE2E_PathDelayViaLocalOrigin(t *testing.T) {
	waitHealthy(t, 60*time.Second)
	waitOrigin(t, 30*time.Second)
	putPassthrough(t)

	fast := httpTTFB(t)
	t.Logf("passthrough TTFB=%s", fast)

	putBaseline(t)
	putProfile(t, "AF", "typical") // enables MITM path delay from rules.expr

	slow := httpTTFB(t)
	t.Logf("shaped TTFB=%s", slow)

	// Local origin: loopback bypasses netem; assert rules delay_ms only.
	minExtra := time.Duration(pathDelayMs-50) * time.Millisecond
	if slow < fast+minExtra {
		t.Fatalf("shaped TTFB %s not enough slower than passthrough %s (want +≥%s)", slow, fast, minExtra)
	}
}

func waitOrigin(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		out, err := exec.Command(
			"docker", "run", "--rm",
			"--network", "container:"+containerName(),
			"curlimages/curl:8.5.0",
			"-sS", "--max-time", "3",
			"http://127.0.0.1/",
		).CombinedOutput()
		last = strings.TrimSpace(string(out))
		if err == nil && strings.Contains(last, "potato-e2e-origin") {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("local origin :80 not ready within %s: last=%q", timeout, last)
}

func waitHealthy(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		code, body := apiGETQuiet("/v1/health")
		last = fmt.Sprintf("%d %s", code, body)
		if code == 200 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("API not healthy within %s: last=%s", timeout, last)
}

func apiGET(t *testing.T, path string) (int, string) {
	t.Helper()
	code, body := apiGETQuiet(path)
	return code, body
}

func apiGETQuiet(path string) (int, string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(apiBase() + path)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, string(b)
}

func apiJSON(t *testing.T, method, path string, payload any) (int, string) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, apiBase()+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, string(b)
}

func putBaseline(t *testing.T) {
	t.Helper()
	code, body := apiJSON(t, http.MethodPut, "/v1/baseline", map[string]any{
		"hostRtt": map[string]int{"cf": 5},
	})
	if code != 200 {
		t.Fatalf("PUT baseline status=%d body=%s", code, body)
	}
}

func putProfile(t *testing.T, country, tier string) {
	t.Helper()
	code, body := apiJSON(t, http.MethodPut, "/v1/profile", map[string]any{
		"country": country,
		"tier":    tier,
	})
	if code != 200 {
		t.Fatalf("PUT profile status=%d body=%s", code, body)
	}
	// Allow qdisc / nft to settle.
	time.Sleep(500 * time.Millisecond)
}

func putPassthrough(t *testing.T) {
	t.Helper()
	code, body := apiJSON(t, http.MethodPut, "/v1/profile", map[string]any{
		"passthrough": true,
	})
	if code != 200 {
		t.Fatalf("PUT passthrough status=%d body=%s", code, body)
	}
	time.Sleep(300 * time.Millisecond)
}

func originURL() string {
	if v := strings.TrimSpace(os.Getenv("POTATONETWORK_E2E_ORIGIN")); v != "" {
		return v
	}
	return "http://127.0.0.1/"
}

func httpTTFB(t *testing.T) time.Duration {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	cname := containerName()
	out, err := exec.Command(
		"docker", "run", "--rm",
		"--network", "container:"+cname,
		"curlimages/curl:8.5.0",
		"-sS", "-o", "/dev/null",
		"-w", "%{time_starttransfer}",
		"--connect-timeout", "15",
		"--max-time", "60",
		originURL(),
	).CombinedOutput()
	if err != nil {
		t.Fatalf("curl in netns: %v\n%s", err, out)
	}
	sec, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatalf("parse TTFB %q: %v", out, err)
	}
	return time.Duration(sec * float64(time.Second))
}
