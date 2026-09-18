package profiles_test

import (
	"testing"

	"github.com/potatoinspector/potato-inspector/internal/profiles"
	"github.com/potatoinspector/potato-inspector/internal/store"
)

func TestBuiltinPassthrough(t *testing.T) {
	st := store.New(t.TempDir())
	_ = st.EnsureDirs()
	reg, err := profiles.NewRegistry(st)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := reg.Get("passthrough")
	if !ok {
		t.Fatal("missing builtin passthrough")
	}
	if !p.Builtin || !p.Passthrough {
		t.Fatalf("passthrough should be builtin+passthrough: %+v", p)
	}
	if len(reg.List()) != 1 {
		t.Fatalf("expected only passthrough builtin, got %d", len(reg.List()))
	}
}
