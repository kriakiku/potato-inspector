package share

import (
	"path/filepath"
	"testing"

	"github.com/potatoinspector/potato-inspector/internal/store"
)

func TestPadSetGetBroadcast(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir)
	p := New(st)
	ch := p.Subscribe()
	defer p.Unsubscribe(ch)

	// drain initial snapshot
	<-ch

	snap, err := p.Set("hello")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != 1 || snap.Text != "hello" {
		t.Fatalf("snap=%+v", snap)
	}
	got := <-ch
	if got.Text != "hello" || got.Version != 1 {
		t.Fatalf("broadcast=%+v", got)
	}
	if p.Get().Text != "hello" {
		t.Fatal("get mismatch")
	}
	if !store.Exists(filepath.Join(dir, "share.json")) {
		t.Fatal("expected persist")
	}
}

func TestPadMaxSize(t *testing.T) {
	p := New(store.New(t.TempDir()))
	big := make([]byte, MaxTextBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	if _, err := p.Set(string(big)); err == nil {
		t.Fatal("expected size error")
	}
}
