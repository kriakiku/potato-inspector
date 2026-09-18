package share

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/potatoinspector/potato-inspector/internal/store"
)

const MaxTextBytes = 256 * 1024

type Snapshot struct {
	Text      string `json:"text"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

type Pad struct {
	mu   sync.Mutex
	path string
	snap Snapshot
	subs map[chan Snapshot]struct{}
}

func New(st *store.Store) *Pad {
	p := &Pad{
		path: st.SharePath(),
		subs: make(map[chan Snapshot]struct{}),
		snap: Snapshot{Text: "", Version: 0, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}
	p.load()
	return p
}

func (p *Pad) load() {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return
	}
	var s Snapshot
	if json.Unmarshal(data, &s) != nil {
		return
	}
	p.snap = s
}

func (p *Pad) persistLocked() {
	_ = store.AtomicWriteJSON(p.path, p.snap)
}

func (p *Pad) Get() Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snap
}

func (p *Pad) Set(text string) (Snapshot, error) {
	if len(text) > MaxTextBytes {
		return Snapshot{}, fmt.Errorf("text exceeds %d bytes", MaxTextBytes)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snap.Text = text
	p.snap.Version++
	p.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	p.persistLocked()
	snap := p.snap
	for ch := range p.subs {
		select {
		case ch <- snap:
		default:
		}
	}
	return snap, nil
}

func (p *Pad) Subscribe() chan Snapshot {
	ch := make(chan Snapshot, 4)
	p.mu.Lock()
	p.subs[ch] = struct{}{}
	snap := p.snap
	p.mu.Unlock()
	select {
	case ch <- snap:
	default:
	}
	return ch
}

func (p *Pad) Unsubscribe(ch chan Snapshot) {
	p.mu.Lock()
	delete(p.subs, ch)
	p.mu.Unlock()
	close(ch)
}
