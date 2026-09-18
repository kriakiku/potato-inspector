package croncatalog

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/kriakiku/potato-network/internal/catalog"
)

// Start schedules catalog refresh.
// Empty / unset → Tuesday at a random UTC hour:minute (picked at process start).
// "false" → disabled.
// Otherwise "M H * * D" (5-field cron-ish, D=0–6 Sunday=0).
func Start(spec string, cat *catalog.Manager, url string) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "false" {
		log.Printf("catalog cron: disabled")
		return
	}
	var minute, hour int
	var dow time.Weekday = time.Tuesday
	if spec == "" {
		minute = rand.Intn(60)
		hour = rand.Intn(24)
		log.Printf("catalog cron: Tuesday %02d:%02d UTC (random)", hour, minute)
	} else {
		// expect "M H * * D" where D=0-6 Sunday=0
		parts := strings.Fields(spec)
		if len(parts) >= 5 {
			_, _ = parseInt(parts[0], &minute)
			_, _ = parseInt(parts[1], &hour)
			if d, ok := parseIntVal(parts[4]); ok {
				dow = time.Weekday(d % 7)
			}
			log.Printf("catalog cron: weekday=%s %02d:%02d UTC", dow, hour, minute)
		} else {
			minute = rand.Intn(60)
			hour = rand.Intn(24)
			log.Printf("catalog cron: unparsed %q — fallback Tue %02d:%02d", spec, hour, minute)
		}
	}
	go loop(cat, url, minute, hour, dow)
}

func loop(cat *catalog.Manager, url string, minute, hour int, dow time.Weekday) {
	for {
		now := time.Now().UTC()
		next := nextRun(now, minute, hour, dow)
		time.Sleep(time.Until(next))
		if err := cat.RefreshFromURL(url); err != nil {
			log.Printf("catalog refresh: %v", err)
		} else {
			log.Printf("catalog refreshed at %s", time.Now().UTC().Format(time.RFC3339))
		}
		time.Sleep(time.Minute) // avoid double-fire
	}
}

func nextRun(now time.Time, minute, hour int, dow time.Weekday) time.Time {
	// find next matching weekday at hour:minute UTC
	for i := 0; i < 8; i++ {
		day := now.AddDate(0, 0, i)
		if day.Weekday() != dow {
			continue
		}
		cand := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.UTC)
		if cand.After(now) {
			return cand
		}
	}
	return now.Add(7 * 24 * time.Hour)
}

func parseInt(s string, dest *int) (int, bool) {
	n, ok := parseIntVal(s)
	if ok {
		*dest = n
	}
	return n, ok
}

func parseIntVal(s string) (int, bool) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
