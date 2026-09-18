package catalog_test

import (
	"testing"

	"github.com/kriakiku/potato-network/internal/catalog"
)

func TestProfileForDelayVsBaseline(t *testing.T) {
	m, err := catalog.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, ok := m.Country("BD")
	if !ok {
		t.Fatal("BD missing from embedded catalog")
	}
	tier := c.Tiers["typical"]
	target := tier.RttToDest["cf"]
	if target <= 0 {
		t.Fatalf("BD typical cf rtt=%d", target)
	}

	host := 10
	p, err := m.ProfileFor("BD", "typical", map[string]int{"cf": host})
	if err != nil {
		t.Fatal(err)
	}
	want := (target - host) / 2
	if want < 0 {
		want = 0
	}
	if p.DelayMs != want {
		t.Fatalf("DelayMs=%d want %d (target=%d host=%d)", p.DelayMs, want, target, host)
	}
	if p.Passthrough || p.DownloadMbps != tier.DownloadMbps || p.LossPercent != tier.LossPercent {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestProfileForClampsWhenHostSlower(t *testing.T) {
	m, err := catalog.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := m.ProfileFor("BD", "typical", map[string]int{"cf": 10_000})
	if err != nil {
		t.Fatal(err)
	}
	if p.DelayMs != 0 {
		t.Fatalf("DelayMs=%d want 0 when host CF RTT exceeds country", p.DelayMs)
	}
	if !p.EmulationLimited || p.Warning == "" {
		t.Fatalf("expected EmulationLimited+Warning, got limited=%v warning=%q", p.EmulationLimited, p.Warning)
	}
}

func TestProfileForNoLimitWhenHostFaster(t *testing.T) {
	m, err := catalog.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := m.ProfileFor("BD", "typical", map[string]int{"cf": 10})
	if err != nil {
		t.Fatal(err)
	}
	if p.EmulationLimited || p.Warning != "" {
		t.Fatalf("unexpected limit: %+v", p)
	}
}
