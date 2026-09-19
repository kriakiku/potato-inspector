package rules

import (
	"testing"
)

func TestClampDelayMs(t *testing.T) {
	if got := ClampDelayMs(-5, 60_000); got != 0 {
		t.Fatalf("neg: %d", got)
	}
	if got := ClampDelayMs(100, 60_000); got != 100 {
		t.Fatalf("ok: %d", got)
	}
	if got := ClampDelayMs(90_000, 60_000); got != 60_000 {
		t.Fatalf("cap: %d", got)
	}
	if got := ClampDelayMs(200, 0); got != 200 {
		t.Fatalf("default max still allows 200: %d", got)
	}
	if got := ClampDelayMs(70_000, 0); got != DefaultMaxDelayMs {
		t.Fatalf("default max: %d", got)
	}
}

func TestApplyJitter(t *testing.T) {
	if got := applyJitter(100, 0); got != 100 {
		t.Fatalf("%d", got)
	}
	seen := map[int]bool{}
	for i := 0; i < 400; i++ {
		got := applyJitter(100, 20)
		if got < 80 || got > 120 {
			t.Fatalf("out of range %d", got)
		}
		seen[got] = true
	}
	if len(seen) < 5 {
		t.Fatalf("spread %d", len(seen))
	}
	for i := 0; i < 100; i++ {
		if got := applyJitter(5, 20); got < 0 {
			t.Fatalf("negative %d", got)
		}
	}
}

func TestParseResult(t *testing.T) {
	r := parseResult(map[string]any{"delay_ms": 12})
	if r.DelayMs != 12 {
		t.Fatalf("%+v", r)
	}
	r = parseResult(map[string]any{"delay_ms": float64(33)})
	if r.DelayMs != 33 {
		t.Fatalf("%+v", r)
	}
}
