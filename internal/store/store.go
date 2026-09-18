package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/potatoinspector/potato-inspector/internal/ignore"
)

// AtomicWriteJSON marshals v and writes via temp + rename.
func AtomicWriteJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ReadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Settings persisted in /data/settings.json
type Settings struct {
	WGEndpoint        string `json:"wgEndpoint"`
	WGSubnet          string `json:"wgSubnet"`
	WGPort            int    `json:"wgPort"`
	Uplink            string `json:"uplink"`
	ActiveProfileID   string `json:"activeProfileId"`
	ActiveCountry     string `json:"activeCountry"`
	ActiveTier        string `json:"activeTier"`
	MITMEnabled       bool   `json:"mitmEnabled"`
	ForceDisableCache bool   `json:"forceDisableCache"`
	CaptureEnabled    bool   `json:"captureEnabled"` // always on; kept for older settings files
	DNSIntercept      bool             `json:"dnsIntercept"`
	ClientDNS         string           `json:"clientDns"`
	DNSShortTTL       bool             `json:"dnsShortTtl"` // clamp forwarded answer TTLs to DNSTTL
	DNSTTL            int              `json:"dnsTtl"`      // seconds: 0, 30, 60, 300
	DNSRewriteRules   []DNSRewriteRule `json:"dnsRewriteRules,omitempty"`
	HostRtt           map[string]int   `json:"hostRtt,omitempty"`
	HostRttPinned     bool           `json:"hostRttPinned"` // deprecated: kept for older settings files
	HostRttProbedAt   string         `json:"hostRttProbedAt,omitempty"`
	FavoriteCountries   []string             `json:"favoriteCountries,omitempty"`
	SystemIgnoreEnabled bool                 `json:"systemIgnoreEnabled"`
	CustomIgnoreText    string               `json:"customIgnoreText,omitempty"`
	CustomIgnore        []ignore.CustomEntry `json:"customIgnore,omitempty"` // derived from text for matching
}

func DefaultSettings(subnet string, port int, uplink string) Settings {
	return Settings{
		WGSubnet:          subnet,
		WGPort:            port,
		Uplink:            uplink,
		ActiveProfileID:   "passthrough",
		ActiveCountry:     "BD",
		ActiveTier:        "typical",
		MITMEnabled:       true,
		ForceDisableCache: false,
		CaptureEnabled:    true,
		DNSIntercept:        true,
		ClientDNS:           "1.1.1.1",
		DNSShortTTL:         false,
		DNSTTL:              30,
		DNSRewriteRules:     nil,
		SystemIgnoreEnabled: true,
		CustomIgnoreText:    ignore.DefaultCustomText(),
		CustomIgnore:        ignore.DomainEntries(ignore.DefaultCustom()),
	}
}

// DNSRewriteRule maps a host pattern to an IPv4 answer (dnsfwd).
type DNSRewriteRule struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"` // example.com or *.domain.com
	IP      string `json:"ip"`
	Enabled bool   `json:"enabled"`
}

// Peer persisted in /data/peers.json
type Peer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
	AllowedIP  string `json:"allowedIP"` // e.g. 10.8.0.2/32
	CreatedAt  string `json:"createdAt"`
}

type PeersFile struct {
	ServerPublicKey  string `json:"serverPublicKey"`
	ServerPrivateKey string `json:"serverPrivateKey"`
	Peers            []Peer `json:"peers"`
}

// MITMRules persisted in /data/mitm-rules.json
type MITMRule struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	HostRegex    string `json:"hostRegex"`
	PathRegex    string `json:"pathRegex"`
	Dest         string `json:"dest"`         // cf | aws-eu-central-1 | aws-us-east-1 | …
	ExtraDelayMs int    `json:"extraDelayMs"` // 0 = path-delta only; >0 = absolute override
	Enabled      bool   `json:"enabled"`
}

type MITMRulesFile struct {
	Rules []MITMRule `json:"rules"`
}

func DefaultMITMRules() MITMRulesFile {
	return MITMRulesFile{
		Rules: []MITMRule{
			{
				ID:           "api-paths",
				Name:         "API paths",
				HostRegex:    ".*",
				PathRegex:    `^/api(/|$)`,
				Dest:         "aws-eu-central-1",
				ExtraDelayMs: 0,
				Enabled:      true,
			},
		},
	}
}

// Store is the synchronized JSON-backed store.
type Store struct {
	mu      sync.RWMutex
	dataDir string
}

func New(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

func (s *Store) DataDir() string { return s.dataDir }

func (s *Store) SettingsPath() string   { return filepath.Join(s.dataDir, "settings.json") }
func (s *Store) PeersPath() string      { return filepath.Join(s.dataDir, "peers.json") }
func (s *Store) MITMRulesPath() string  { return filepath.Join(s.dataDir, "mitm-rules.json") }
func (s *Store) CustomProfilesPath() string {
	return filepath.Join(s.dataDir, "profiles", "custom.json")
}
func (s *Store) CADir() string { return filepath.Join(s.dataDir, "ca") }
func (s *Store) SharePath() string { return filepath.Join(s.dataDir, "share.json") }

func (s *Store) EnsureDirs() error {
	for _, d := range []string{
		s.dataDir,
		filepath.Join(s.dataDir, "profiles"),
		s.CADir(),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func (s *Store) LoadSettings() (Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		return Settings{}, err
	}
	var st Settings
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	st.DNSIntercept = true // always on
	migrateDNSSettings(&st, data)
	return st, nil
}

func (s *Store) SaveSettings(st Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st.DNSIntercept = true
	st.DNSTTL = NormalizeDNSTTL(st.DNSTTL)
	return AtomicWriteJSON(s.SettingsPath(), st)
}

// NormalizeDNSTTL returns an allowed short-TTL preset (0, 30, 60, 300); default 30.
func NormalizeDNSTTL(sec int) int {
	switch sec {
	case 0, 30, 60, 300:
		return sec
	default:
		return 30
	}
}

func migrateDNSSettings(st *Settings, raw []byte) {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["dnsTtl"]; !ok {
		st.DNSTTL = 30
		// Legacy force-zero toggle → short TTL on with 0s
		if v, ok := m["dnsZeroTtl"]; ok {
			var z bool
			if json.Unmarshal(v, &z) == nil && z {
				st.DNSShortTTL = true
				st.DNSTTL = 0
			}
		}
	} else {
		st.DNSTTL = NormalizeDNSTTL(st.DNSTTL)
	}
	if _, ok := m["dnsShortTtl"]; !ok {
		if v, ok := m["dnsZeroTtl"]; ok {
			var z bool
			if json.Unmarshal(v, &z) == nil {
				st.DNSShortTTL = z
			}
		}
	}
}

func (s *Store) LoadPeers() (PeersFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var pf PeersFile
	err := ReadJSON(s.PeersPath(), &pf)
	if err != nil && os.IsNotExist(err) {
		return PeersFile{Peers: []Peer{}}, nil
	}
	return pf, err
}

func (s *Store) SavePeers(pf PeersFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return AtomicWriteJSON(s.PeersPath(), pf)
}

func (s *Store) LoadMITMRules() (MITMRulesFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var r MITMRulesFile
	err := ReadJSON(s.MITMRulesPath(), &r)
	if err != nil && os.IsNotExist(err) {
		return DefaultMITMRules(), nil
	}
	return r, err
}

func (s *Store) SaveMITMRules(r MITMRulesFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return AtomicWriteJSON(s.MITMRulesPath(), r)
}

func (s *Store) LoadCustomProfiles() ([]map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path := s.CustomProfilesPath()
	if !Exists(path) {
		return []map[string]any{}, nil
	}
	var list []map[string]any
	if err := ReadJSON(path, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) SaveCustomProfiles(list []map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return AtomicWriteJSON(s.CustomProfilesPath(), list)
}
