package shape

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/potatoinspector/potato-inspector/internal/profiles"
)

type Manager struct {
	mu     sync.RWMutex
	iface  string
	active string
	status string
}

func New(iface string) *Manager {
	return &Manager{iface: iface, active: "passthrough", status: "no qdisc"}
}

func (m *Manager) ActiveID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

func (m *Manager) Status() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = exec.Command("tc", "qdisc", "del", "dev", m.iface, "root").Run()
	_ = exec.Command("tc", "qdisc", "del", "dev", m.iface, "ingress").Run()
	_ = exec.Command("tc", "qdisc", "del", "dev", "ifb0", "root").Run()
	_ = exec.Command("ip", "link", "set", "dev", "ifb0", "down").Run()
	m.active = "passthrough"
	m.status = "no qdisc"
	return nil
}

// Apply shapes inner traffic on the WG tun (never the UDP listen socket).
// Download = internet → client = ingress on tun (via IFB).
// Upload = client → internet = egress on tun.
// Delay is one-way ms; RTT ≈ 2×.
func (m *Manager) Apply(p profiles.Profile) error {
	if p.Passthrough || (p.DelayMs == 0 && p.DownloadMbps == 0 && p.UploadMbps == 0 && p.LossPercent == 0) {
		if err := m.Clear(); err != nil {
			return err
		}
		m.mu.Lock()
		m.active = p.ID
		m.status = "passthrough"
		m.mu.Unlock()
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	_ = exec.Command("tc", "qdisc", "del", "dev", m.iface, "root").Run()
	_ = exec.Command("tc", "qdisc", "del", "dev", m.iface, "ingress").Run()
	_ = exec.Command("tc", "qdisc", "del", "dev", "ifb0", "root").Run()

	jitter := p.DelayMs / 10

	// Egress = upload
	if err := applyHTBNetem(m.iface, p.DelayMs, jitter, p.LossPercent, p.UploadMbps); err != nil {
		return fmt.Errorf("egress(upload): %w", err)
	}

	// Ingress = download via IFB
	if err := setupIFB(m.iface); err != nil {
		return fmt.Errorf("ifb: %w", err)
	}
	if err := applyHTBNetem("ifb0", p.DelayMs, jitter, p.LossPercent, p.DownloadMbps); err != nil {
		return fmt.Errorf("ingress(download): %w", err)
	}

	m.active = p.ID
	m.status = fmt.Sprintf("delay=%dms(one-way≈%dms RTT) loss=%.2f%% down=%.1fMbps up=%.1fMbps",
		p.DelayMs, p.DelayMs*2, p.LossPercent, p.DownloadMbps, p.UploadMbps)
	return nil
}

func applyHTBNetem(iface string, delayMs, jitterMs int, loss, rateMbps float64) error {
	_ = exec.Command("tc", "qdisc", "del", "dev", iface, "root").Run()

	if rateMbps > 0 {
		rate := fmt.Sprintf("%.0fkbit", rateMbps*1000)
		cmds := [][]string{
			{"qdisc", "replace", "dev", iface, "root", "handle", "1:", "htb", "default", "10"},
			{"class", "replace", "dev", iface, "parent", "1:", "classid", "1:10", "htb", "rate", rate, "ceil", rate},
		}
		for _, c := range cmds {
			if out, err := exec.Command("tc", c...).CombinedOutput(); err != nil {
				return fmt.Errorf("tc %v: %w (%s)", c, err, strings.TrimSpace(string(out)))
			}
		}
		args := []string{"qdisc", "replace", "dev", iface, "parent", "1:10", "handle", "20:", "netem"}
		args = append(args, netemArgs(delayMs, jitterMs, loss)...)
		if len(args) > 8 {
			if out, err := exec.Command("tc", args...).CombinedOutput(); err != nil {
				return fmt.Errorf("tc netem: %w (%s)", err, strings.TrimSpace(string(out)))
			}
		}
		return nil
	}

	args := []string{"qdisc", "replace", "dev", iface, "root", "handle", "1:", "netem"}
	args = append(args, netemArgs(delayMs, jitterMs, loss)...)
	if len(netemArgs(delayMs, jitterMs, loss)) == 0 {
		return nil
	}
	if out, err := exec.Command("tc", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tc netem: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func netemArgs(delayMs, jitterMs int, loss float64) []string {
	var a []string
	if delayMs > 0 {
		a = append(a, "delay", fmt.Sprintf("%dms", delayMs))
		if jitterMs > 0 {
			a = append(a, fmt.Sprintf("%dms", jitterMs))
		}
	}
	if loss > 0 {
		a = append(a, "loss", fmt.Sprintf("%f%%", loss))
	}
	return a
}

func setupIFB(iface string) error {
	_ = exec.Command("modprobe", "ifb").Run()
	_ = exec.Command("ip", "link", "add", "ifb0", "type", "ifb").Run()
	if out, err := exec.Command("ip", "link", "set", "dev", "ifb0", "up").CombinedOutput(); err != nil {
		return fmt.Errorf("ifb up: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	_ = exec.Command("tc", "qdisc", "del", "dev", iface, "ingress").Run()
	if out, err := exec.Command("tc", "qdisc", "add", "dev", iface, "handle", "ffff:", "ingress").CombinedOutput(); err != nil {
		return fmt.Errorf("ingress qdisc: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	redir := []string{"filter", "add", "dev", iface, "parent", "ffff:", "protocol", "ip", "u32",
		"match", "u32", "0", "0", "action", "mirred", "egress", "redirect", "dev", "ifb0"}
	if out, err := exec.Command("tc", redir...).CombinedOutput(); err != nil {
		return fmt.Errorf("redirect ifb: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) QdiscDump() string {
	out1, _ := exec.Command("tc", "qdisc", "show", "dev", m.iface).CombinedOutput()
	out2, _ := exec.Command("tc", "qdisc", "show", "dev", "ifb0").CombinedOutput()
	return strings.TrimSpace(string(out1) + "\n" + string(out2))
}
