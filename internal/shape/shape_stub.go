//go:build !linux

package shape

import (
	"fmt"
	"sync"

	"github.com/potatoinspector/potato-inspector/internal/profiles"
)

// Manager is a no-op shaper outside Linux (dev builds).
type Manager struct {
	mu          sync.RWMutex
	iface       string
	active      string
	status      string
	lastProfile profiles.Profile
}

func New(iface string, apiPort int, dnsUpstream string) *Manager {
	return &Manager{
		iface:       iface,
		active:      "passthrough",
		status:      "noop (non-linux)",
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
	m.active = "passthrough"
	m.status = "noop (non-linux)"
	m.lastProfile = profiles.PassthroughProfile()
	return nil
}
func (m *Manager) Apply(p profiles.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastProfile = p
	m.active = p.ID
	m.status = "noop (non-linux)"
	return nil
}

func ResolveUplink(preferred string) (string, error) {
	if preferred != "" {
		return preferred, nil
	}
	return "", fmt.Errorf("uplink resolution requires linux")
}
