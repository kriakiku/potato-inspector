package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kriakiku/potato-network/internal/profiles"
)

//go:embed data/catalog.json
var embeddedCatalog []byte

type Destination struct {
	Label     string `json:"label"`
	Calibrate string `json:"calibrate"`
	Target    string `json:"target"`
	AwsRegion string `json:"awsRegion,omitempty"`
}

type Tier struct {
	DownloadMbps float64        `json:"downloadMbps"`
	UploadMbps   float64        `json:"uploadMbps"`
	LossPercent  float64        `json:"lossPercent"`
	RttToDest    map[string]int `json:"rttToDest"`
}

type Country struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Flag       string          `json:"flag"`
	NearestAws string          `json:"nearestAws,omitempty"`
	RttSource  string          `json:"rttSource,omitempty"`
	Tiers      map[string]Tier `json:"tiers"`
}

type Catalog struct {
	GeneratedAt  string                 `json:"generatedAt"`
	Source       string                 `json:"source"`
	Destinations map[string]Destination `json:"destinations"`
	Countries    []Country              `json:"countries"`
}

type Manager struct {
	mu        sync.RWMutex
	dataDir   string
	cat       Catalog
	hostRtt   map[string]int
	lastProbe time.Time
}

func NewManager(dataDir string) (*Manager, error) {
	m := &Manager{
		dataDir: dataDir,
		hostRtt: make(map[string]int),
	}
	if err := m.Load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) catalogPath() string {
	return filepath.Join(m.dataDir, "catalog.json")
}

func (m *Manager) Load() error {
	path := m.catalogPath()
	var data []byte
	if b, err := os.ReadFile(path); err == nil {
		data = b
	} else {
		data = embeddedCatalog
		_ = os.MkdirAll(m.dataDir, 0o755)
		_ = os.WriteFile(path, embeddedCatalog, 0o644)
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return fmt.Errorf("parse catalog: %w", err)
	}
	m.mu.Lock()
	m.cat = cat
	m.mu.Unlock()
	return nil
}

func (m *Manager) Get() Catalog {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cat
}

func (m *Manager) Country(id string) (Country, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.cat.Countries {
		if c.ID == id {
			return c, true
		}
	}
	return Country{}, false
}

func (m *Manager) Destinations() map[string]Destination {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]Destination, len(m.cat.Destinations))
	for k, v := range m.cat.Destinations {
		out[k] = v
	}
	return out
}

func (m *Manager) RefreshFromURL(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("catalog HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return fmt.Errorf("parse catalog: %w", err)
	}
	if err := os.MkdirAll(m.dataDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(m.catalogPath(), data, 0o644); err != nil {
		return err
	}
	m.mu.Lock()
	m.cat = cat
	m.mu.Unlock()
	return nil
}

func (m *Manager) ProfileFor(countryID, tier string, hostRtt map[string]int) (profiles.Profile, error) {
	c, ok := m.Country(countryID)
	if !ok {
		return profiles.Profile{}, fmt.Errorf("unknown country %s", countryID)
	}
	t, ok := c.Tiers[tier]
	if !ok {
		return profiles.Profile{}, fmt.Errorf("unknown tier %s", tier)
	}
	targetRtt := t.RttToDest["cf"]
	hostCf := 0
	if hostRtt != nil {
		hostCf = hostRtt["cf"]
	}
	effectiveRtt := targetRtt - hostCf
	if effectiveRtt < 0 {
		effectiveRtt = 0
	}
	delay := effectiveRtt / 2
	return profiles.Profile{
		ID:           fmt.Sprintf("catalog:%s:%s", countryID, tier),
		Name:         fmt.Sprintf("%s %s (%s)", c.Flag, c.Name, tier),
		Description:  fmt.Sprintf("Last-mile %s/%s; base delay vs CF (target RTT %dms, host CF %dms)", countryID, tier, targetRtt, hostCf),
		Country:      countryID,
		Tier:         tier,
		DelayMs:      delay,
		DownloadMbps: t.DownloadMbps,
		UploadMbps:   t.UploadMbps,
		LossPercent:  t.LossPercent,
		Passthrough:  false,
	}, nil
}

func (m *Manager) PathExtraDelayMs(countryID, tier, dest string, hostRtt map[string]int) int {
	if dest == "" || dest == "cf" {
		return 0
	}
	c, ok := m.Country(countryID)
	if !ok {
		return 0
	}
	t, ok := c.Tiers[tier]
	if !ok {
		return 0
	}
	rttDest := t.RttToDest[dest]
	rttCf := t.RttToDest["cf"]
	pathDelta := rttDest - rttCf
	if pathDelta < 0 {
		pathDelta = 0
	}
	hostDelta := 0
	if hostRtt != nil {
		hd := hostRtt[dest] - hostRtt["cf"]
		if hd > 0 {
			hostDelta = hd
		}
	}
	extraRtt := pathDelta - hostDelta
	if extraRtt < 0 {
		extraRtt = 0
	}
	return extraRtt / 2
}

func (m *Manager) HostRtt() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int, len(m.hostRtt))
	for k, v := range m.hostRtt {
		out[k] = v
	}
	return out
}

func (m *Manager) SetHostRtt(rtt map[string]int) {
	m.mu.Lock()
	m.hostRtt = copyIntMap(rtt)
	m.lastProbe = time.Now()
	m.mu.Unlock()
}

func copyIntMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (m *Manager) LastProbe() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastProbe
}
