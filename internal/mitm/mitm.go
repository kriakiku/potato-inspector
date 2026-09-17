package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/potatoinspector/potato-inspector/internal/flows"
	"github.com/potatoinspector/potato-inspector/internal/store"
)

const IngestAddr = "127.0.0.1:9477"

type Manager struct {
	mu         sync.Mutex
	store      *store.Store
	flows      *flows.Writer
	cmd        *os.Process
	enabled    bool
	addonDir   string
	wgIface    string
	ingest     *http.Server
	ingestOnce sync.Once
}

func New(st *store.Store, fw *flows.Writer, addonDir, wgIface string) *Manager {
	return &Manager{
		store:    st,
		flows:    fw,
		addonDir: addonDir,
		wgIface:  wgIface,
	}
}

func (m *Manager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled
}

func (m *Manager) EnsureCA() (certPath, keyPath string, err error) {
	dir := m.store.CADir()
	certPath = filepath.Join(dir, "ca.crt")
	keyPath = filepath.Join(dir, "ca.key")
	if store.Exists(certPath) && store.Exists(keyPath) {
		return certPath, keyPath, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "PotatoInspector CA",
			Organization: []string{"PotatoInspector"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	certOut, err := os.Create(certPath)
	if err != nil {
		return "", "", err
	}
	_ = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	_ = certOut.Close()
	keyOut, err := os.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", "", err
	}
	_ = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	_ = keyOut.Close()
	return certPath, keyPath, nil
}

func (m *Manager) CACertPath() string {
	return filepath.Join(m.store.CADir(), "ca.crt")
}

func (m *Manager) ensureIngest() {
	m.ingestOnce.Do(func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/event", m.handleIngest)
		m.ingest = &http.Server{Addr: IngestAddr, Handler: mux}
		go func() {
			ln, err := net.Listen("tcp", IngestAddr)
			if err != nil {
				return
			}
			_ = m.ingest.Serve(ln)
		}()
	})
}

func (m *Manager) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "read", 400)
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		http.Error(w, "json", 400)
		return
	}
	typ, _ := raw["type"].(string)
	summary, _ := raw["summary"].(string)
	detail, _ := raw["detail"].(map[string]any)
	id, _ := raw["id"].(string)
	ev := flows.Event{
		ID:      id,
		Type:    flows.EventType(typ),
		Ts:      time.Now().UTC(),
		Summary: summary,
		Detail:  detail,
	}
	if ts, ok := raw["ts"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			ev.Ts = t
		} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
			ev.Ts = t
		}
	}
	m.flows.Emit(ev)
	w.WriteHeader(http.StatusNoContent)
}

func (m *Manager) writeRuntimeConfig() (string, error) {
	rules, err := m.store.LoadMITMRules()
	if err != nil {
		return "", err
	}
	settings, err := m.store.LoadSettings()
	if err != nil {
		return "", err
	}
	cfg := map[string]any{
		"extraDelayMs": settings.ExtraDelayMs,
		"rules":        rules.Rules,
		"bypassSni":    rules.BypassSNI,
		"eventsURL":    "http://" + IngestAddr + "/event",
		"capture":      settings.CaptureEnabled,
	}
	path := filepath.Join(m.store.DataDir(), "mitm-runtime.json")
	data, _ := json.MarshalIndent(cfg, "", "  ")
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil {
		return nil
	}
	m.ensureIngest()
	cert, key, err := m.EnsureCA()
	if err != nil {
		return err
	}
	cfgPath, err := m.writeRuntimeConfig()
	if err != nil {
		return err
	}
	addon := filepath.Join(m.addonDir, "potato_addon.py")
	args := []string{
		"-s", addon,
		"--mode", "transparent",
		"--listen-host", "0.0.0.0",
		"--listen-port", "8080",
		"--set", "confdir=" + m.store.CADir(),
		"--set", "potato_config=" + cfgPath,
		"--ssl-insecure",
	}
	bin := "mitmdump"
	if _, err := exec.LookPath(bin); err != nil {
		bin = "mitmproxy"
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"POTATO_CA_CERT="+cert,
		"POTATO_CA_KEY="+key,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mitm: %w", err)
	}
	m.cmd = cmd.Process
	m.enabled = true
	if err := SetupTPROXY(m.wgIface); err != nil {
		_ = cmd.Process.Kill()
		m.cmd = nil
		m.enabled = false
		return err
	}
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		m.cmd = nil
		m.enabled = false
		m.mu.Unlock()
	}()
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ClearTPROXY(m.wgIface)
	if m.cmd != nil {
		_ = m.cmd.Kill()
		m.cmd = nil
	}
	m.enabled = false
	return nil
}

func (m *Manager) ReloadConfig() error {
	_, err := m.writeRuntimeConfig()
	return err
}

func SetupTPROXY(wgIface string) error {
	_ = exec.Command("iptables", "-C", "FORWARD", "-i", wgIface, "-p", "udp", "--dport", "443", "-j", "DROP").Run()
	_ = exec.Command("iptables", "-I", "FORWARD", "1", "-i", wgIface, "-p", "udp", "--dport", "443", "-j", "DROP").Run()
	_ = exec.Command("iptables", "-t", "mangle", "-N", "POTATO_MITM").Run()
	_ = exec.Command("iptables", "-t", "mangle", "-F", "POTATO_MITM").Run()
	rules := [][]string{
		{"-t", "mangle", "-A", "POTATO_MITM", "-p", "tcp", "--dport", "80", "-j", "TPROXY", "--on-port", "8080", "--on-ip", "127.0.0.1", "--tproxy-mark", "1"},
		{"-t", "mangle", "-A", "POTATO_MITM", "-p", "tcp", "--dport", "443", "-j", "TPROXY", "--on-port", "8080", "--on-ip", "127.0.0.1", "--tproxy-mark", "1"},
		{"-t", "mangle", "-A", "PREROUTING", "-i", wgIface, "-j", "POTATO_MITM"},
	}
	for _, r := range rules {
		_ = exec.Command("iptables", r...).Run()
	}
	_ = exec.Command("iptables", "-t", "nat", "-N", "POTATO_MITM_NAT").Run()
	_ = exec.Command("iptables", "-t", "nat", "-F", "POTATO_MITM_NAT").Run()
	_ = exec.Command("iptables", "-t", "nat", "-A", "POTATO_MITM_NAT", "-p", "tcp", "--dport", "80", "-j", "REDIRECT", "--to-ports", "8080").Run()
	_ = exec.Command("iptables", "-t", "nat", "-A", "POTATO_MITM_NAT", "-p", "tcp", "--dport", "443", "-j", "REDIRECT", "--to-ports", "8080").Run()
	_ = exec.Command("iptables", "-t", "nat", "-A", "PREROUTING", "-i", wgIface, "-j", "POTATO_MITM_NAT").Run()
	_ = exec.Command("ip", "rule", "add", "fwmark", "1", "lookup", "100").Run()
	_ = exec.Command("ip", "route", "add", "local", "0.0.0.0/0", "dev", "lo", "table", "100").Run()
	return nil
}

func ClearTPROXY(wgIface string) {
	_ = exec.Command("iptables", "-D", "FORWARD", "-i", wgIface, "-p", "udp", "--dport", "443", "-j", "DROP").Run()
	_ = exec.Command("iptables", "-t", "mangle", "-D", "PREROUTING", "-i", wgIface, "-j", "POTATO_MITM").Run()
	_ = exec.Command("iptables", "-t", "mangle", "-F", "POTATO_MITM").Run()
	_ = exec.Command("iptables", "-t", "mangle", "-X", "POTATO_MITM").Run()
	_ = exec.Command("iptables", "-t", "nat", "-D", "PREROUTING", "-i", wgIface, "-j", "POTATO_MITM_NAT").Run()
	_ = exec.Command("iptables", "-t", "nat", "-F", "POTATO_MITM_NAT").Run()
	_ = exec.Command("iptables", "-t", "nat", "-X", "POTATO_MITM_NAT").Run()
}

func LocalIP() string {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}