package cronbaseline

import (
	"testing"
	"time"
)

func TestNextRunEvery3h(t *testing.T) {
	s := Schedule{EveryH: 3, Minute: 17, HourMod: 1} // hours 1,4,7,10,...
	now := time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)
	got := nextRun(now, s)
	want := time.Date(2026, 9, 18, 4, 17, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestParseFalse(t *testing.T) {
	if !Parse("false").Disabled {
		t.Fatal("expected disabled")
	}
}

func TestParseStarSlash(t *testing.T) {
	s := Parse("42 */6 * * *")
	if s.Disabled || s.EveryH != 6 || s.Minute != 42 {
		t.Fatalf("unexpected %#v", s)
	}
}
