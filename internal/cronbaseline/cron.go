package cronbaseline

import (
	"log"
	"math/rand"
	"strings"
	"time"

	pnruntime "github.com/kriakiku/potato-network/internal/runtime"
)

// Schedule describes how often to re-probe host baseline.
type Schedule struct {
	Disabled bool
	EveryH   int // hours between runs (default 3)
	Minute   int // UTC minute 0–59
	HourMod  int // run when hour%EveryH == HourMod
}

// Parse reads POTATONETWORK_BASELINE_CRON.
// Empty/unset → every 3h at a random UTC minute (and random slot in the 3h cycle).
// "false" → disabled (no boot auto-probe, no loop).
// "M */N * * *" → every N hours at minute M (HourMod random in 0..N-1 if not specified).
func Parse(spec string) Schedule {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "false" {
		return Schedule{Disabled: true}
	}
	s := Schedule{EveryH: 3, Minute: rand.Intn(60), HourMod: rand.Intn(3)}
	if spec == "" {
		log.Printf("baseline cron: every %dh at :%02d UTC (slot hour%%%d==%d)", s.EveryH, s.Minute, s.EveryH, s.HourMod)
		return s
	}
	parts := strings.Fields(spec)
	if len(parts) >= 2 {
		if m, ok := parseIntVal(parts[0]); ok {
			s.Minute = m % 60
		}
		hourField := parts[1]
		if strings.HasPrefix(hourField, "*/") {
			if n, ok := parseIntVal(strings.TrimPrefix(hourField, "*/")); ok && n > 0 {
				s.EveryH = n
				s.HourMod = rand.Intn(n)
			}
		} else if h, ok := parseIntVal(hourField); ok {
			// "M H * * *" → treat as once daily at H:M; interval 24h, fixed mod
			s.EveryH = 24
			s.HourMod = h % 24
		}
		log.Printf("baseline cron: every %dh at :%02d UTC (slot hour%%%d==%d) from %q", s.EveryH, s.Minute, s.EveryH, s.HourMod, spec)
		return s
	}
	log.Printf("baseline cron: unparsed %q — fallback every %dh :%02d", spec, s.EveryH, s.Minute)
	return s
}

func (s Schedule) Interval() time.Duration {
	if s.EveryH <= 0 {
		return 3 * time.Hour
	}
	return time.Duration(s.EveryH) * time.Hour
}

// Start loads already done by State; probes if empty/stale, then loops (unless disabled).
func Start(spec string, st *pnruntime.State) {
	s := Parse(spec)
	if st.BaselineEmpty() {
		log.Printf("baseline: empty — probing now")
		rtt := st.ProbeBaseline()
		log.Printf("baseline: probed %d dests", len(rtt))
	} else if !s.Disabled && st.BaselineAge() > s.Interval() {
		log.Printf("baseline: stale (age=%s > %s) — probing now", st.BaselineAge().Round(time.Second), s.Interval())
		rtt := st.ProbeBaseline()
		log.Printf("baseline: probed %d dests", len(rtt))
	} else if !st.BaselineEmpty() {
		log.Printf("baseline: fresh (age=%s)", st.BaselineAge().Round(time.Second))
	}
	if s.Disabled {
		log.Printf("baseline cron: disabled")
		return
	}
	go loop(st, s)
}

func loop(st *pnruntime.State, s Schedule) {
	for {
		now := time.Now().UTC()
		next := nextRun(now, s)
		time.Sleep(time.Until(next))
		rtt := st.ProbeBaseline()
		log.Printf("baseline cron: probed %d dests at %s", len(rtt), time.Now().UTC().Format(time.RFC3339))
		time.Sleep(time.Minute)
	}
}

func nextRun(now time.Time, s Schedule) time.Time {
	every := s.EveryH
	if every <= 0 {
		every = 3
	}
	mod := s.HourMod % every
	t := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), s.Minute, 0, 0, time.UTC)
	if !t.After(now) {
		t = t.Add(time.Hour)
	}
	for i := 0; i < every*48; i++ {
		if t.Hour()%every == mod {
			return t
		}
		t = t.Add(time.Hour)
	}
	return now.Add(time.Duration(every) * time.Hour)
}

func parseIntVal(s string) (int, bool) {
	n := 0
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
