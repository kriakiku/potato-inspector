package catalog

import (
	"net"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// ProbeHostRtt measures approximate RTT (ms) to each destination target via TCP connect.
func (m *Manager) ProbeHostRtt() map[string]int {
	dests := m.Destinations()
	out := make(map[string]int, len(dests))
	for id, d := range dests {
		target := d.Target
		if target == "" {
			continue
		}
		ms := probeTCP(target, 443)
		if ms <= 0 {
			ms = probeTCP(target, 80)
		}
		if ms > 0 {
			out[id] = ms
		}
	}
	if _, ok := dests["cf"]; ok {
		if ms := probeHTTP("https://speed.cloudflare.com/__down?bytes=0"); ms > 0 {
			out["cf"] = ms
		}
	}
	m.SetHostRtt(out)
	return out
}

func probeTCP(host string, port int) int {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var samples []int
	for i := 0; i < 3; i++ {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			continue
		}
		ms := int(time.Since(start).Milliseconds())
		_ = conn.Close()
		if ms > 0 {
			samples = append(samples, ms)
		}
	}
	return medianInt(samples)
}

func probeHTTP(url string) int {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	var samples []int
	for i := 0; i < 3; i++ {
		start := time.Now()
		req, err := http.NewRequest(http.MethodHead, url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			req, _ = http.NewRequest(http.MethodGet, url, nil)
			resp, err = client.Do(req)
			if err != nil {
				continue
			}
		}
		ms := int(time.Since(start).Milliseconds())
		_ = resp.Body.Close()
		if ms > 0 {
			samples = append(samples, ms)
		}
	}
	return medianInt(samples)
}

func medianInt(samples []int) int {
	if len(samples) == 0 {
		return 0
	}
	sort.Ints(samples)
	return samples[len(samples)/2]
}
