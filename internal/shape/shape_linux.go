//go:build linux

package shape

import (
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"sync"

	"github.com/kriakiku/potato-network/internal/config"
	"github.com/kriakiku/potato-network/internal/datapath"
	"github.com/kriakiku/potato-network/internal/profiles"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

const ifbName = "ifb0"

// Manager applies last-mile netem on the uplink via netlink.
type Manager struct {
	mu          sync.RWMutex
	iface       string
	apiPort     int
	dnsUpstream string
	exclude     []net.IPNet
	active      string
	status      string
	lastProfile profiles.Profile
}

func New(iface string, apiPort int, dnsUpstream string, exclude []net.IPNet) *Manager {
	return &Manager{
		iface:       iface,
		apiPort:     apiPort,
		dnsUpstream: dnsUpstream,
		exclude:     append([]net.IPNet(nil), exclude...),
		active:      "passthrough",
		status:      "no qdisc",
		lastProfile: profiles.PassthroughProfile(),
	}
}

func (m *Manager) Iface() string { return m.iface }

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

func (m *Manager) LastProfile() profiles.Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastProfile
}

func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clearLocked()
	if err := m.installExemptLocked(); err != nil {
		return err
	}
	m.active = "passthrough"
	m.status = "no qdisc"
	m.lastProfile = profiles.PassthroughProfile()
	return nil
}

func (m *Manager) clearLocked() {
	_ = deleteRootQdisc(m.iface)
	_ = deleteAllQdiscs(ifbName)
	if link, err := netlink.LinkByName(ifbName); err == nil {
		_ = netlink.LinkSetDown(link)
	}
	datapath.ClearExempt()
}

// EnsureExempt reinstalls API/DNS/shape-exclude marks (call after MITM InstallBase).
func (m *Manager) EnsureExempt() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.installExemptLocked()
}

func (m *Manager) installExemptLocked() error {
	if err := datapath.SetExempt(m.apiPort, m.dnsUpstream, m.exclude); err != nil {
		return fmt.Errorf("nftables exempt: %w", err)
	}
	return nil
}

func (m *Manager) Apply(p profiles.Profile) error {
	if p.Passthrough || (p.DelayMs == 0 && p.DownloadMbps == 0 && p.UploadMbps == 0 && p.LossPercent == 0) {
		return m.Clear()
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clearLocked()
	if err := m.installExemptLocked(); err != nil {
		return err
	}

	jitter := p.DelayMs / 10
	if err := applyHTBNetem(m.iface, p.DelayMs, jitter, p.LossPercent, p.UploadMbps); err != nil {
		return fmt.Errorf("egress: %w", err)
	}
	if err := setupIFB(m.iface, m.exclude); err != nil {
		return fmt.Errorf("ifb: %w", err)
	}
	if err := applyHTBNetem(ifbName, p.DelayMs, jitter, p.LossPercent, p.DownloadMbps); err != nil {
		return fmt.Errorf("ingress: %w", err)
	}

	m.active = p.ID
	m.lastProfile = p
	m.status = fmt.Sprintf("delay=%dms(one-way≈%dms RTT) loss=%.2f%% down=%.1fMbps up=%.1fMbps iface=%s",
		p.DelayMs, p.DelayMs*2, p.LossPercent, p.DownloadMbps, p.UploadMbps, m.iface)
	return nil
}

func deleteAllQdiscs(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	qdiscs, err := netlink.QdiscList(link)
	if err != nil {
		return err
	}
	for _, q := range qdiscs {
		_ = netlink.QdiscDel(q)
	}
	return nil
}

func deleteRootQdisc(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	qdiscs, err := netlink.QdiscList(link)
	if err != nil {
		return err
	}
	for _, q := range qdiscs {
		parent := q.Attrs().Parent
		if parent == netlink.HANDLE_ROOT || parent == 0 {
			_ = netlink.QdiscDel(q)
		}
		if parent == netlink.HANDLE_INGRESS || q.Attrs().Handle == netlink.MakeHandle(0xffff, 0) {
			_ = netlink.QdiscDel(q)
		}
	}
	return nil
}

func applyHTBNetem(iface string, delayMs, jitterMs int, loss, rateMbps float64) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}
	_ = deleteAllQdiscs(iface)
	idx := link.Attrs().Index

	htb := netlink.NewHtb(netlink.QdiscAttrs{
		LinkIndex: idx,
		Handle:    netlink.MakeHandle(1, 0),
		Parent:    netlink.HANDLE_ROOT,
	})
	htb.Defcls = 20
	if err := netlink.QdiscAdd(htb); err != nil {
		return fmt.Errorf("htb root: %w", err)
	}

	addClass := func(handle, parent uint32, rate uint64) error {
		cl := netlink.NewHtbClass(netlink.ClassAttrs{
			LinkIndex: idx,
			Handle:    handle,
			Parent:    parent,
		}, netlink.HtbClassAttrs{Rate: rate, Ceil: rate})
		return netlink.ClassAdd(cl)
	}
	const tenG = uint64(1_250_000_000)
	if err := addClass(netlink.MakeHandle(1, 1), netlink.MakeHandle(1, 0), tenG); err != nil {
		return fmt.Errorf("htb 1:1: %w", err)
	}
	if err := addClass(netlink.MakeHandle(1, 10), netlink.MakeHandle(1, 1), tenG); err != nil {
		return fmt.Errorf("htb 1:10: %w", err)
	}
	rate := tenG
	if rateMbps > 0 {
		rate = uint64(rateMbps * 1000 * 1000 / 8)
	}
	if err := addClass(netlink.MakeHandle(1, 20), netlink.MakeHandle(1, 1), rate); err != nil {
		return fmt.Errorf("htb 1:20: %w", err)
	}

	// optional leaf on exempt class omitted (kernel default)

	if delayMs > 0 || loss > 0 {
		attrs := netlink.NetemQdiscAttrs{}
		if delayMs > 0 {
			attrs.Latency = uint32(delayMs) * 1000 // netem expects microseconds
			if jitterMs > 0 {
				attrs.Jitter = uint32(jitterMs) * 1000
			}
		}
		if loss > 0 {
			attrs.Loss = float32(loss)
		}
		netem := netlink.NewNetem(netlink.QdiscAttrs{
			LinkIndex: idx,
			Handle:    netlink.MakeHandle(20, 0),
			Parent:    netlink.MakeHandle(1, 20),
		}, attrs)
		if err := netlink.QdiscAdd(netem); err != nil {
			return fmt.Errorf("netem: %w", err)
		}
	}

	fw := &netlink.FwFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: idx,
			Parent:    netlink.MakeHandle(1, 0),
			Handle:    netlink.MakeHandle(0, uint16(config.MarkExemptShape)),
			Priority:  1,
			Protocol:  unix.ETH_P_IP,
		},
		ClassId: netlink.MakeHandle(1, 10),
		Mask:    uint32(config.MarkExemptShape),
	}
	if err := netlink.FilterAdd(fw); err != nil {
		return fmt.Errorf("fw filter: %w", err)
	}
	return nil
}

func setupIFB(uplink string, exclude []net.IPNet) error {
	_ = exec.Command("modprobe", "ifb").Run()

	if _, err := netlink.LinkByName(ifbName); err != nil {
		if err := netlink.LinkAdd(&netlink.Ifb{LinkAttrs: netlink.LinkAttrs{Name: ifbName}}); err != nil {
			return fmt.Errorf("ifb add: %w", err)
		}
	}
	ifbLink, err := netlink.LinkByName(ifbName)
	if err != nil {
		return err
	}
	if err := netlink.LinkSetUp(ifbLink); err != nil {
		return fmt.Errorf("ifb up: %w", err)
	}

	up, err := netlink.LinkByName(uplink)
	if err != nil {
		return err
	}
	// Remove only ingress qdisc; keep egress HTB root.
	qdiscs, _ := netlink.QdiscList(up)
	for _, q := range qdiscs {
		if q.Attrs().Parent == netlink.HANDLE_INGRESS || q.Attrs().Handle == netlink.MakeHandle(0xffff, 0) {
			_ = netlink.QdiscDel(q)
		}
	}

	ingress := &netlink.Ingress{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: up.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_INGRESS,
		},
	}
	if err := netlink.QdiscAdd(ingress); err != nil {
		return fmt.Errorf("ingress qdisc: %w", err)
	}

	// Higher priority: skip mirred for packets from shape-exclude sources (no ingress netem).
	prio := uint16(1)
	for _, n := range exclude {
		ip4 := n.IP.To4()
		mask := net.IP(n.Mask).To4()
		if ip4 == nil || mask == nil {
			continue
		}
		val := binary.BigEndian.Uint32(ip4) & binary.BigEndian.Uint32(mask)
		msk := binary.BigEndian.Uint32(mask)
		skip := &netlink.U32{
			FilterAttrs: netlink.FilterAttrs{
				LinkIndex: up.Attrs().Index,
				Parent:    netlink.MakeHandle(0xffff, 0),
				Priority:  prio,
				Protocol:  unix.ETH_P_IP,
			},
			Sel: &netlink.TcU32Sel{
				Flags: netlink.TC_U32_TERMINAL,
				Nkeys: 1,
				// ip.src at offset 12 within IP header; +nexthdr offset handled by kernel for ETH_P_IP
				Keys: []netlink.TcU32Key{{
					Mask: msk,
					Val:  val,
					Off:  12,
				}},
			},
			Actions: []netlink.Action{
				&netlink.GenericAction{
					ActionAttrs: netlink.ActionAttrs{Action: netlink.TC_ACT_OK},
				},
			},
		}
		if err := netlink.FilterAdd(skip); err != nil {
			return fmt.Errorf("ingress skip exclude %s: %w", n.String(), err)
		}
		prio++
	}

	filter := &netlink.U32{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: up.Attrs().Index,
			Parent:    netlink.MakeHandle(0xffff, 0),
			Priority:  prio,
			Protocol:  unix.ETH_P_IP,
		},
		Sel: &netlink.TcU32Sel{
			Flags: netlink.TC_U32_TERMINAL,
			Nkeys: 1,
			Keys:  []netlink.TcU32Key{{Mask: 0, Val: 0, Off: 0}},
		},
		Actions: []netlink.Action{
			&netlink.MirredAction{
				ActionAttrs:  netlink.ActionAttrs{Action: netlink.TC_ACT_STOLEN},
				MirredAction: netlink.TCA_EGRESS_REDIR,
				Ifindex:      ifbLink.Attrs().Index,
			},
		},
	}
	if err := netlink.FilterAdd(filter); err != nil {
		return fmt.Errorf("mirred ifb: %w", err)
	}
	return nil
}

func ResolveUplink(preferred string) (string, error) {
	if preferred != "" {
		if _, err := netlink.LinkByName(preferred); err == nil {
			return preferred, nil
		}
	}
	routes, err := netlink.RouteList(nil, unix.AF_INET)
	if err != nil {
		return "", fmt.Errorf("route list: %w", err)
	}
	for _, r := range routes {
		if r.Dst == nil && r.LinkIndex > 0 {
			link, err := netlink.LinkByIndex(r.LinkIndex)
			if err != nil {
				continue
			}
			return link.Attrs().Name, nil
		}
	}
	// No default route yet (some Docker / early boot) — first non-loopback iface.
	links, err := netlink.LinkList()
	if err != nil {
		return "", fmt.Errorf("no default route device")
	}
	for _, link := range links {
		attrs := link.Attrs()
		if attrs.Name == "lo" || attrs.Flags&net.FlagLoopback != 0 {
			continue
		}
		if attrs.Flags&net.FlagUp == 0 {
			continue
		}
		return attrs.Name, nil
	}
	return "", fmt.Errorf("no default route device")
}
