package flows

import (
	"fmt"
	"sync"
	"time"
)

const (
	MaxBodyPreview = 64 * 1024 // 64 KiB
	RingSize       = 2000      // max events kept in RAM
)

type EventType string

const (
	TypeHTTP EventType = "http"
	TypeTLS  EventType = "tls"
	TypeDNS  EventType = "dns"
)

type Event struct {
	ID      string         `json:"id"`
	Type    EventType      `json:"type"`
	Ts      time.Time      `json:"ts"`
	Summary string         `json:"summary"`
	Detail  map[string]any `json:"detail"`
}

// Writer keeps a fixed-size in-memory ring of inspector events (no disk).
type Writer struct {
	mu      sync.Mutex
	enabled bool
	ring    []Event
	ringIdx int
	ringLen int
	subs    map[chan Event]struct{}
}

func New() *Writer {
	return &Writer{
		ring: make([]Event, RingSize),
		subs: make(map[chan Event]struct{}),
	}
}

func (w *Writer) SetEnabled(v bool) {
	w.mu.Lock()
	w.enabled = v
	w.mu.Unlock()
}

func (w *Writer) Enabled() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.enabled
}

func (w *Writer) Capacity() int { return RingSize }

func (w *Writer) Subscribe() chan Event {
	ch := make(chan Event, 64)
	w.mu.Lock()
	w.subs[ch] = struct{}{}
	w.mu.Unlock()
	return ch
}

func (w *Writer) Unsubscribe(ch chan Event) {
	w.mu.Lock()
	delete(w.subs, ch)
	w.mu.Unlock()
	close(ch)
}

func (w *Writer) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ring = make([]Event, RingSize)
	w.ringIdx = 0
	w.ringLen = 0
}

func (w *Writer) Recent(limit int, typ EventType) []Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	if limit <= 0 || limit > w.ringLen {
		limit = w.ringLen
	}
	out := make([]Event, 0, limit)
	for i := 0; i < w.ringLen && len(out) < limit; i++ {
		idx := (w.ringIdx - 1 - i + RingSize) % RingSize
		ev := w.ring[idx]
		if typ != "" && ev.Type != typ {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func (w *Writer) Emit(ev Event) {
	if ev.ID == "" {
		ev.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if ev.Ts.IsZero() {
		ev.Ts = time.Now().UTC()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.enabled {
		// still allow live subscribers when capture is off? Plan: capture gates recording.
		// DNS/MITM should respect capture — skip ring if disabled.
		return
	}

	w.ring[w.ringIdx] = ev
	w.ringIdx = (w.ringIdx + 1) % RingSize
	if w.ringLen < RingSize {
		w.ringLen++
	}

	for ch := range w.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (w *Writer) Close() error { return nil }

// TruncateBody caps body preview size and stubs binary.
func TruncateBody(s string, contentType string) (preview string, stubbed bool) {
	if isBinaryCT(contentType) {
		return fmt.Sprintf("[binary omitted: %s]", contentType), true
	}
	if len(s) > MaxBodyPreview {
		return s[:MaxBodyPreview] + "\n…[truncated]", false
	}
	return s, false
}

func isBinaryCT(ct string) bool {
	if ct == "" {
		return false
	}
	binaryPrefixes := []string{"image/", "audio/", "video/", "application/octet-stream", "application/pdf", "application/zip"}
	for _, p := range binaryPrefixes {
		if len(ct) >= len(p) && ct[:len(p)] == p {
			return true
		}
	}
	return false
}
