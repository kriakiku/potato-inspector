package runtime

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/potatoinspector/potato-inspector/internal/baseline"
	"github.com/potatoinspector/potato-inspector/internal/catalog"
	"github.com/potatoinspector/potato-inspector/internal/profiles"
	"github.com/potatoinspector/potato-inspector/internal/shape"
)

// State holds ephemeral profile + baseline (baseline also persisted under dataDir).
type State struct {
	mu       sync.RWMutex
	dataDir  string
	catalog  *catalog.Manager
	shape    *shape.Manager
	profile  profiles.Profile
	hostRtt  map[string]int
	probedAt time.Time
}

func New(dataDir string, cat *catalog.Manager, sh *shape.Manager) *State {
	s := &State{
		dataDir: dataDir,
		catalog: cat,
		shape:   sh,
		profile: profiles.PassthroughProfile(),
		hostRtt: map[string]int{},
	}
	if f, err := baseline.Load(dataDir); err != nil {
		log.Printf("baseline load: %v", err)
	} else if !f.Empty() {
		s.hostRtt = copyMap(f.HostRtt)
		s.probedAt = f.ProbedAt
		cat.SetHostRtt(f.HostRtt)
		log.Printf("baseline loaded probedAt=%s (%d dests)", f.ProbedAt.Format(time.RFC3339), len(f.HostRtt))
	}
	return s
}

func (s *State) Profile() profiles.Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profile
}

func (s *State) HostRtt() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int, len(s.hostRtt))
	for k, v := range s.hostRtt {
		out[k] = v
	}
	return out
}

func (s *State) ProbedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.probedAt
}

func (s *State) BaselineEmpty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.hostRtt) == 0 || s.probedAt.IsZero()
}

func (s *State) BaselineAge() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.probedAt.IsZero() {
		return time.Duration(1<<63 - 1)
	}
	return time.Since(s.probedAt)
}

func (s *State) SetHostRtt(rtt map[string]int) {
	s.mu.Lock()
	s.hostRtt = copyMap(rtt)
	s.probedAt = time.Now().UTC()
	probedAt := s.probedAt
	host := copyMap(s.hostRtt)
	s.mu.Unlock()
	s.catalog.SetHostRtt(host)
	s.persist(host, probedAt)
	p := s.Profile()
	if !p.Passthrough && p.Country != "" {
		_, _ = s.ApplyCountryTier(p.Country, p.Tier)
	}
}

func (s *State) ProbeBaseline() map[string]int {
	rtt := s.catalog.ProbeHostRtt()
	s.mu.Lock()
	s.hostRtt = copyMap(rtt)
	s.probedAt = time.Now().UTC()
	probedAt := s.probedAt
	s.mu.Unlock()
	s.persist(rtt, probedAt)
	p := s.Profile()
	if !p.Passthrough && p.Country != "" {
		_, _ = s.ApplyCountryTier(p.Country, p.Tier)
	}
	return rtt
}

func (s *State) persist(rtt map[string]int, probedAt time.Time) {
	if s.dataDir == "" {
		return
	}
	if err := baseline.Save(s.dataDir, rtt, probedAt); err != nil {
		log.Printf("baseline save: %v", err)
	}
}

func (s *State) ClearPassthrough() error {
	if err := s.shape.Clear(); err != nil {
		return err
	}
	s.mu.Lock()
	s.profile = profiles.PassthroughProfile()
	s.mu.Unlock()
	return nil
}

func (s *State) ApplyCountryTier(country, tier string) (profiles.Profile, error) {
	host := s.HostRtt()
	if len(host) == 0 {
		host = s.catalog.HostRtt()
	}
	p, err := s.catalog.ProfileFor(country, tier, host)
	if err != nil {
		return profiles.Profile{}, err
	}
	p.Country = country
	p.Tier = tier
	if err := s.shape.Apply(p); err != nil {
		return profiles.Profile{}, err
	}
	s.mu.Lock()
	s.profile = p
	s.mu.Unlock()
	return p, nil
}

func (s *State) PathExtraDelayMs(dest string) int {
	p := s.Profile()
	if p.Passthrough || p.Country == "" {
		return 0
	}
	return s.catalog.PathExtraDelayMs(p.Country, p.Tier, dest, s.HostRtt())
}

func (s *State) HandshakeDelayMs() int {
	p := s.Profile()
	if p.Passthrough {
		return 0
	}
	// Approximate one-way last-mile so client-visible TLS is not instant.
	return p.DelayMs
}

func (s *State) StatusSummary() string {
	p := s.Profile()
	if p.Passthrough {
		return "passthrough"
	}
	return fmt.Sprintf("%s/%s delay=%dms", p.Country, p.Tier, p.DelayMs)
}

func copyMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
