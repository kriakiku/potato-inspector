package profiles_test

import (
	"testing"

	"github.com/potatoinspector/potato-inspector/internal/profiles"
	"github.com/potatoinspector/potato-inspector/internal/store"
)

func TestBuiltinIDsStable(t *testing.T) {
	st := store.New(t.TempDir())
	_ = st.EnsureDirs()
	reg, err := profiles.NewRegistry(st)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"passthrough",
		"bangladesh-dhaka-4g-europe-stable",
		"bangladesh-dhaka-4g-europe",
		"bangladesh-khulna-4g-cf-dac-stable",
		"bangladesh-khulna-4g-cf-dac",
		"bangladesh-mymensingh-teletalk-weak",
	}
	for _, id := range want {
		p, ok := reg.Get(id)
		if !ok {
			t.Fatalf("missing builtin %s", id)
		}
		if !p.Builtin {
			t.Fatalf("%s should be builtin", id)
		}
	}
	p, _ := reg.Get("bangladesh-dhaka-4g-europe")
	if p.DelayMs != 90 || p.DownloadMbps != 32 || p.UploadMbps != 12 || p.LossPercent != 0.5 {
		t.Fatalf("demo profile numbers changed: %+v", p)
	}
}
