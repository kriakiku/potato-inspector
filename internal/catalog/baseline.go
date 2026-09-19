package catalog

import (
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/kriakiku/potato-network/internal/netmark"
)

// ProbeHostRtt measures approximate RTT (ms) to each destination via TCP connect.
// For cf we intentionally do NOT use HTTPS TTFB (TLS+HTTP) — that inflates ~10× vs
// true edge RTT and would zero-out last-mile delay for nearby countries.
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
	m.SetHostRtt(out)
	return out
}

func probeTCP(host string, port int) int {
	// Resolve once so samples are connect RTT, not DNS+connect.
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return probeTCPDial(net.JoinHostPort(host, strconv.Itoa(port)))
	}
	ip := ips[0].String()
	if ips[0].To4() == nil {
		ip = "[" + ip + "]"
	}
	return probeTCPDial(net.JoinHostPort(ip, strconv.Itoa(port)))
}

func probeTCPDial(addr string) int {
	var samples []int
	// Discard first connect (slow-path / SYN quirks); median of the rest.
	for i := 0; i < 4; i++ {
		start := time.Now()
		// Marked dial: skip MITM REDIRECT so :443 probes hit the real dest, not local proxy.
		conn, err := netmark.DialTimeout("tcp", addr, 3*time.Second)
		if err != nil {
			continue
		}
		ms := int(time.Since(start).Milliseconds())
		_ = conn.Close()
		if ms > 0 && i > 0 {
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
