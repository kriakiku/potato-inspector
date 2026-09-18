package wg

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"

	"github.com/potatoinspector/potato-inspector/internal/store"
)

type Manager struct {
	mu         sync.RWMutex
	iface      string
	subnet     *net.IPNet
	listenPort int
	uplink     string
	store      *store.Store
	device     *device.Device
	tunDev     tun.Device
	serverPriv [32]byte
	serverPub  [32]byte
	started    bool
}

type PeerStatus struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	PublicKey     string     `json:"publicKey"`
	AllowedIP     string     `json:"allowedIP"`
	CreatedAt     string     `json:"createdAt"`
	LastHandshake *time.Time `json:"lastHandshake,omitempty"`
	RxBytes       int64      `json:"rxBytes"`
	TxBytes       int64      `json:"txBytes"`
	Endpoint      string     `json:"endpoint,omitempty"`
}

func NewManager(iface, subnetCIDR string, listenPort int, uplink string, st *store.Store) (*Manager, error) {
	_, ipnet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return nil, fmt.Errorf("parse subnet: %w", err)
	}
	return &Manager{
		iface:      iface,
		subnet:     ipnet,
		listenPort: listenPort,
		uplink:     uplink,
		store:      st,
	}, nil
}

func (m *Manager) Iface() string { return m.iface }
func (m *Manager) Subnet() string {
	return m.subnet.String()
}
func (m *Manager) ListenPort() int { return m.listenPort }
func (m *Manager) ServerPublicKey() string {
	return base64.StdEncoding.EncodeToString(m.serverPub[:])
}

func GenerateKeypair() (priv, pub [32]byte, err error) {
	if _, err = rand.Read(priv[:]); err != nil {
		return
	}
	// clamp
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	curve25519.ScalarBaseMult(&pub, &priv)
	return
}

func keyToBase64(k [32]byte) string {
	return base64.StdEncoding.EncodeToString(k[:])
}

func parseKey(s string) ([32]byte, error) {
	var out [32]byte
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return out, err
	}
	if len(b) != 32 {
		return out, fmt.Errorf("key length %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pf, err := m.store.LoadPeers()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if pf.ServerPrivateKey == "" {
		priv, pub, err := GenerateKeypair()
		if err != nil {
			return err
		}
		m.serverPriv = priv
		m.serverPub = pub
		pf.ServerPrivateKey = keyToBase64(priv)
		pf.ServerPublicKey = keyToBase64(pub)
		if pf.Peers == nil {
			pf.Peers = []store.Peer{}
		}
		if err := m.store.SavePeers(pf); err != nil {
			return err
		}
	} else {
		priv, err := parseKey(pf.ServerPrivateKey)
		if err != nil {
			return err
		}
		pub, err := parseKey(pf.ServerPublicKey)
		if err != nil {
			return err
		}
		m.serverPriv = priv
		m.serverPub = pub
	}

	tunDev, err := tun.CreateTUN(m.iface, device.DefaultMTU)
	if err != nil {
		return fmt.Errorf("create tun %s: %w (need /dev/net/tun + CAP_NET_ADMIN)", m.iface, err)
	}
	m.tunDev = tunDev

	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", m.iface))
	dev := device.NewDevice(tunDev, NewStdNetBind(), logger)
	m.device = dev

	cfg := fmt.Sprintf("private_key=%x\nlisten_port=%d\n", m.serverPriv, m.listenPort)
	for _, p := range pf.Peers {
		pk, err := parseKey(p.PublicKey)
		if err != nil {
			continue
		}
		cfg += fmt.Sprintf("public_key=%x\nallowed_ip=%s\n", pk, stripHostBits(p.AllowedIP))
	}
	if err := dev.IpcSet(cfg); err != nil {
		return fmt.Errorf("ipc set: %w", err)
	}
	dev.Up()

	serverIP := m.gatewayIP()
	if err := exec.Command("ip", "addr", "add", serverIP.String()+"/"+subnetPrefix(m.subnet), "dev", m.iface).Run(); err != nil {
		// may already exist
		_ = exec.Command("ip", "addr", "replace", serverIP.String()+"/"+subnetPrefix(m.subnet), "dev", m.iface).Run()
	}
	_ = exec.Command("ip", "link", "set", "up", "dev", m.iface).Run()

	if err := EnableForwarding(); err != nil {
		return err
	}
	if err := SetupNAT(m.subnet.String(), m.uplink); err != nil {
		return err
	}

	m.started = true
	return nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ClearNAT(m.subnet.String(), m.uplink)
	if m.device != nil {
		m.device.Close()
	}
	m.started = false
}

func (m *Manager) GatewayIP() net.IP {
	return m.gatewayIP()
}

func (m *Manager) gatewayIP() net.IP {
	ip := m.subnet.IP.To4()
	if ip == nil {
		return m.subnet.IP
	}
	out := make(net.IP, 4)
	copy(out, ip)
	out[3] = 1
	return out
}

func subnetPrefix(n *net.IPNet) string {
	ones, _ := n.Mask.Size()
	return strconv.Itoa(ones)
}

func stripHostBits(cidr string) string {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return cidr
	}
	return ip.Mask(ipnet.Mask).String() + "/" + subnetPrefix(ipnet)
}

func (m *Manager) nextIP(peers []store.Peer) (string, error) {
	used := map[string]bool{}
	for _, p := range peers {
		ip, _, err := net.ParseCIDR(p.AllowedIP)
		if err != nil {
			continue
		}
		used[ip.String()] = true
	}
	base := m.subnet.IP.To4()
	if base == nil {
		return "", fmt.Errorf("ipv4 only")
	}
	ones, bits := m.subnet.Mask.Size()
	maxHosts := 1 << (bits - ones)
	for i := 2; i < maxHosts-1; i++ {
		ip := make(net.IP, 4)
		copy(ip, base)
		ip[3] = byte(i) // simplistic for /24
		if ones < 24 {
			n := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
			n += uint32(i)
			ip[0] = byte(n >> 24)
			ip[1] = byte(n >> 16)
			ip[2] = byte(n >> 8)
			ip[3] = byte(n)
		}
		if !m.subnet.Contains(ip) {
			continue
		}
		if used[ip.String()] {
			continue
		}
		return ip.String() + "/32", nil
	}
	return "", fmt.Errorf("no free IPs in %s", m.subnet)
}

func (m *Manager) AddPeer(name string) (store.Peer, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pf, err := m.store.LoadPeers()
	if err != nil {
		return store.Peer{}, "", err
	}
	ip, err := m.nextIP(pf.Peers)
	if err != nil {
		return store.Peer{}, "", err
	}
	priv, pub, err := GenerateKeypair()
	if err != nil {
		return store.Peer{}, "", err
	}
	peer := store.Peer{
		ID:         fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:       name,
		PublicKey:  keyToBase64(pub),
		PrivateKey: keyToBase64(priv),
		AllowedIP:  ip,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	pf.Peers = append(pf.Peers, peer)
	if err := m.store.SavePeers(pf); err != nil {
		return store.Peer{}, "", err
	}
	if m.device != nil {
		cfg := fmt.Sprintf("public_key=%x\nallowed_ip=%s\n", pub, strings.Split(ip, "/")[0]+"/32")
		_ = m.device.IpcSet(cfg)
	}
	return peer, keyToBase64(priv), nil
}

func (m *Manager) RevokePeer(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pf, err := m.store.LoadPeers()
	if err != nil {
		return err
	}
	var removed *store.Peer
	kept := make([]store.Peer, 0, len(pf.Peers))
	for i := range pf.Peers {
		if pf.Peers[i].ID == id {
			p := pf.Peers[i]
			removed = &p
			continue
		}
		kept = append(kept, pf.Peers[i])
	}
	if removed == nil {
		return fmt.Errorf("peer not found")
	}
	pf.Peers = kept
	if err := m.store.SavePeers(pf); err != nil {
		return err
	}
	if m.device != nil {
		pk, err := parseKey(removed.PublicKey)
		if err == nil {
			_ = m.device.IpcSet(fmt.Sprintf("public_key=%x\nremove=true\n", pk))
		}
	}
	return nil
}

func (m *Manager) ClientConfig(peer store.Peer, endpoint, dns string) string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	b.WriteString(fmt.Sprintf("PrivateKey = %s\n", peer.PrivateKey))
	b.WriteString(fmt.Sprintf("Address = %s\n", peer.AllowedIP))
	if dns != "" {
		b.WriteString(fmt.Sprintf("DNS = %s\n", dns))
	}
	b.WriteString("\n[Peer]\n")
	b.WriteString(fmt.Sprintf("PublicKey = %s\n", keyToBase64(m.serverPub)))
	b.WriteString("AllowedIPs = 0.0.0.0/0, ::/0\n")
	if endpoint != "" {
		b.WriteString(fmt.Sprintf("Endpoint = %s\n", endpoint))
	}
	b.WriteString("PersistentKeepalive = 25\n")
	return b.String()
}

func (m *Manager) ListPeerStatus() ([]PeerStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pf, err := m.store.LoadPeers()
	if err != nil {
		return nil, err
	}
	handshakeByPub := map[string]peerRuntime{}
	if m.device != nil {
		dump, err := m.device.IpcGet()
		if err == nil {
			handshakeByPub = parseWGDump(dump)
		}
	}
	out := make([]PeerStatus, 0, len(pf.Peers))
	for _, p := range pf.Peers {
		st := PeerStatus{
			ID:        p.ID,
			Name:      p.Name,
			PublicKey: p.PublicKey,
			AllowedIP: p.AllowedIP,
			CreatedAt: p.CreatedAt,
		}
		if rt, ok := handshakeByPub[p.PublicKey]; ok {
			if !rt.handshake.IsZero() {
				hs := rt.handshake
				st.LastHandshake = &hs
			}
			st.RxBytes = rt.rx
			st.TxBytes = rt.tx
			st.Endpoint = rt.endpoint
		}
		out = append(out, st)
	}
	return out, nil
}

type peerRuntime struct {
	handshake time.Time
	rx, tx    int64
	endpoint  string
}

func parseWGDump(dump string) map[string]peerRuntime {
	out := map[string]peerRuntime{}
	var cur string
	var rt peerRuntime
	flush := func() {
		if cur != "" {
			out[cur] = rt
		}
	}
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k, v := parts[0], parts[1]
		switch k {
		case "public_key":
			flush()
			b, err := hexToBase64(v)
			if err != nil {
				cur = ""
				rt = peerRuntime{}
				continue
			}
			cur = b
			rt = peerRuntime{}
		case "last_handshake_time_sec":
			sec, _ := strconv.ParseInt(v, 10, 64)
			if sec > 0 {
				rt.handshake = time.Unix(sec, 0)
			}
		case "rx_bytes":
			rt.rx, _ = strconv.ParseInt(v, 10, 64)
		case "tx_bytes":
			rt.tx, _ = strconv.ParseInt(v, 10, 64)
		case "endpoint":
			rt.endpoint = v
		}
	}
	flush()
	return out
}

func hexToBase64(hexKey string) (string, error) {
	if len(hexKey) != 64 {
		return "", fmt.Errorf("bad hex key")
	}
	b := make([]byte, 32)
	for i := 0; i < 32; i++ {
		var v byte
		fmt.Sscanf(hexKey[i*2:i*2+2], "%02x", &v)
		b[i] = v
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func EnableForwarding() error {
	const path = "/proc/sys/net/ipv4/ip_forward"
	if b, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(b)) == "1" {
		return nil
	}
	if err := os.WriteFile(path, []byte("1\n"), 0o644); err != nil {
		// Docker often mounts this read-only after setting it via compose sysctls.
		if b, rerr := os.ReadFile(path); rerr == nil && strings.TrimSpace(string(b)) == "1" {
			return nil
		}
		return fmt.Errorf("enable ip_forward: %w (set net.ipv4.ip_forward=1 via compose sysctls or host)", err)
	}
	return nil
}

func SetupNAT(subnet, uplink string) error {
	uplink = strings.TrimSpace(uplink)
	if uplink == "" {
		return fmt.Errorf("MASQUERADE: empty uplink")
	}
	if !IfaceExists(uplink) {
		return fmt.Errorf("MASQUERADE: uplink %q not found", uplink)
	}
	if err := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", subnet, "-o", uplink, "-j", "MASQUERADE").Run(); err != nil {
		if err := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", subnet, "-o", uplink, "-j", "MASQUERADE").Run(); err != nil {
			return fmt.Errorf("MASQUERADE -o %s: %w", uplink, err)
		}
	}
	// Confirm the rule is actually present (iptables can accept -o for a missing iface at add time on some setups).
	if err := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", subnet, "-o", uplink, "-j", "MASQUERADE").Run(); err != nil {
		return fmt.Errorf("MASQUERADE rule missing after setup (-s %s -o %s): %w", subnet, uplink, err)
	}
	iface := os.Getenv("POTATOINSPECTOR_WG_IFACE")
	if iface == "" {
		iface = "wg0"
	}
	if exec.Command("iptables", "-C", "FORWARD", "-i", iface, "-o", uplink, "-j", "ACCEPT").Run() != nil {
		_ = exec.Command("iptables", "-A", "FORWARD", "-i", iface, "-o", uplink, "-j", "ACCEPT").Run()
	}
	if exec.Command("iptables", "-C", "FORWARD", "-i", uplink, "-o", iface, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT").Run() != nil {
		_ = exec.Command("iptables", "-A", "FORWARD", "-i", uplink, "-o", iface, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT").Run()
	}
	log.Printf("NAT: MASQUERADE -s %s -o %s (FORWARD %s <-> %s)", subnet, uplink, iface, uplink)
	return nil
}

func ClearNAT(subnet, uplink string) {
	uplink = strings.TrimSpace(uplink)
	if uplink == "" {
		return
	}
	_ = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", subnet, "-o", uplink, "-j", "MASQUERADE").Run()
	iface := os.Getenv("POTATOINSPECTOR_WG_IFACE")
	if iface == "" {
		iface = "wg0"
	}
	_ = exec.Command("iptables", "-D", "FORWARD", "-i", iface, "-o", uplink, "-j", "ACCEPT").Run()
	_ = exec.Command("iptables", "-D", "FORWARD", "-i", uplink, "-o", iface, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT").Run()
}

// ApplyUplink replaces the NAT uplink, clearing previous iptables rules when already started.
func (m *Manager) ApplyUplink(uplink string) error {
	uplink = strings.TrimSpace(uplink)
	if uplink == "" {
		return fmt.Errorf("empty uplink")
	}
	if !IfaceExists(uplink) {
		return fmt.Errorf("uplink %q not found", uplink)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.uplink == uplink {
		return nil
	}
	prev := m.uplink
	if m.started && prev != "" {
		ClearNAT(m.subnet.String(), prev)
	}
	m.uplink = uplink
	if m.started {
		if err := SetupNAT(m.subnet.String(), uplink); err != nil {
			m.uplink = prev
			if prev != "" {
				_ = SetupNAT(m.subnet.String(), prev)
			}
			return err
		}
	}
	return nil
}

func (m *Manager) Uplink() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.uplink
}