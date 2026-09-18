package ignore

import (
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
)

const (
	IPSetName  = "potato_ignore"
	IPSetName6 = "potato_ignore6"
	MarkIgnore = "0x50"
)

// Runtime holds system + custom ignore state and feeds DNS answers into ipsets for shape exemption.
type Runtime struct {
	mu      sync.RWMutex
	system  bool
	custom  []CustomEntry
}

func NewRuntime(systemOn bool, custom []CustomEntry) *Runtime {
	if custom == nil {
		custom = []CustomEntry{}
	}
	r := &Runtime{system: systemOn, custom: append([]CustomEntry(nil), custom...)}
	_ = EnsureIPSets()
	if !r.Active() {
		_ = FlushIPSets()
	}
	return r
}

// Active is true when builtin pack is on or any custom entry is enabled.
func (r *Runtime) Active() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeLocked()
}

func (r *Runtime) activeLocked() bool {
	if r.system {
		return true
	}
	for _, e := range r.custom {
		if e.Enabled {
			return true
		}
	}
	return false
}

func (r *Runtime) SystemEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.system
}

func (r *Runtime) Custom() []CustomEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CustomEntry, len(r.custom))
	copy(out, r.custom)
	return out
}

func (r *Runtime) SetSystemEnabled(on bool) {
	r.mu.Lock()
	r.system = on
	active := r.activeLocked()
	r.mu.Unlock()
	_ = EnsureIPSets()
	if !active {
		_ = FlushIPSets()
		ClearMangle()
	}
}

func (r *Runtime) SetCustom(custom []CustomEntry) {
	if custom == nil {
		custom = []CustomEntry{}
	}
	r.mu.Lock()
	r.custom = append([]CustomEntry(nil), custom...)
	active := r.activeLocked()
	r.mu.Unlock()
	_ = EnsureIPSets()
	if !active {
		_ = FlushIPSets()
		ClearMangle()
	}
}

// SetEnabled is an alias for SetSystemEnabled (builtin pack toggle).
func (r *Runtime) SetEnabled(on bool) { r.SetSystemEnabled(on) }

// Enabled reports builtin system-ignore pack state.
func (r *Runtime) Enabled() bool { return r.SystemEnabled() }

// Matches reports whether qname matches the active system pack and/or custom rules.
func (r *Runtime) Matches(qname string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	system := r.system
	custom := r.custom
	r.mu.RUnlock()
	return MatchAny(qname, system, custom)
}

// ObserveDNS adds A/AAAA answers to ipsets when qname matches an active ignore rule.
func (r *Runtime) ObserveDNS(qname string, answers []string) {
	if !r.Matches(qname) {
		return
	}
	_ = EnsureIPSets()
	for _, a := range answers {
		ip := net.ParseIP(strings.TrimSpace(a))
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			_ = exec.Command("ipset", "add", IPSetName, ip.String(), "-exist").Run()
		} else {
			_ = exec.Command("ipset", "add", IPSetName6, ip.String(), "-exist").Run()
		}
	}
}

func EnsureIPSets() error {
	_ = exec.Command("ipset", "create", IPSetName, "hash:ip", "family", "inet", "-exist").Run()
	_ = exec.Command("ipset", "create", IPSetName6, "hash:ip", "family", "inet6", "-exist").Run()
	return nil
}

func FlushIPSets() error {
	_ = exec.Command("ipset", "flush", IPSetName).Run()
	_ = exec.Command("ipset", "flush", IPSetName6).Run()
	return nil
}

// SetupMangle marks packets to/from ignored IPs on the WG iface for tc fw filters.
func SetupMangle(wgIface string) {
	ClearMangle()
	if wgIface == "" {
		return
	}
	chain := "POTATO_IGNORE"
	_ = exec.Command("iptables", "-t", "mangle", "-N", chain).Run()
	_ = exec.Command("iptables", "-t", "mangle", "-F", chain).Run()
	// Egress on tun = client→internet: dst in ignore set
	_ = exec.Command("iptables", "-t", "mangle", "-A", chain, "-o", wgIface,
		"-m", "set", "--match-set", IPSetName, "dst", "-j", "MARK", "--set-mark", MarkIgnore).Run()
	_ = exec.Command("iptables", "-t", "mangle", "-A", chain, "-o", wgIface,
		"-m", "set", "--match-set", IPSetName6, "dst", "-j", "MARK", "--set-mark", MarkIgnore).Run()
	// Ingress on tun = internet→client: src in ignore set
	_ = exec.Command("iptables", "-t", "mangle", "-A", chain, "-i", wgIface,
		"-m", "set", "--match-set", IPSetName, "src", "-j", "MARK", "--set-mark", MarkIgnore).Run()
	_ = exec.Command("iptables", "-t", "mangle", "-A", chain, "-i", wgIface,
		"-m", "set", "--match-set", IPSetName6, "src", "-j", "MARK", "--set-mark", MarkIgnore).Run()
	_ = exec.Command("iptables", "-t", "mangle", "-A", "PREROUTING", "-j", chain).Run()
	_ = exec.Command("iptables", "-t", "mangle", "-A", "POSTROUTING", "-j", chain).Run()
	log.Printf("system ignore: mangle marks on %s", wgIface)
}

func ClearMangle() {
	chain := "POTATO_IGNORE"
	_ = exec.Command("iptables", "-t", "mangle", "-D", "PREROUTING", "-j", chain).Run()
	_ = exec.Command("iptables", "-t", "mangle", "-D", "POSTROUTING", "-j", chain).Run()
	_ = exec.Command("iptables", "-t", "mangle", "-F", chain).Run()
	_ = exec.Command("iptables", "-t", "mangle", "-X", chain).Run()
}
