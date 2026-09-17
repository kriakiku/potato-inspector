package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/potatoinspector/potato-inspector/internal/store"
)

func TestAtomicWriteJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	v := store.Settings{ActiveProfileID: "bangladesh-dhaka-4g-europe", ExtraDelayMs: 180}
	if err := store.AtomicWriteJSON(path, v); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should be gone, err=%v", err)
	}
	var got store.Settings
	if err := store.ReadJSON(path, &got); err != nil {
		t.Fatal(err)
	}
	if got.ActiveProfileID != v.ActiveProfileID || got.ExtraDelayMs != 180 {
		t.Fatalf("got %+v", got)
	}
}
