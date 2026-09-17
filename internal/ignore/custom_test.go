package ignore

import (
	"strings"
	"testing"
)

func TestNormalizeDomain(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"github.com", "github.com", true},
		{"*.github.com", "github.com", true},
		{"  *.GitHub.COM  ", "github.com", true},
		{"github.com/foo", "github.com", true},
		{"", "", false},
		{"not a domain", "", false},
		{"-bad.com", "", false},
	}
	for _, c := range cases {
		got, err := NormalizeDomain(c.in)
		if c.ok {
			if err != nil || got != c.want {
				t.Fatalf("%q: got %q %v, want %q", c.in, got, err, c.want)
			}
		} else if err == nil {
			t.Fatalf("%q: expected error, got %q", c.in, got)
		}
	}
}

func TestMatchCustom(t *testing.T) {
	custom := []CustomEntry{
		{Domain: "github.com", Enabled: true},
		{Domain: "example.com", Enabled: false},
		{Comment: "lol"},
	}
	if !MatchCustom("api.github.com", custom) {
		t.Fatal("expected match api.github.com")
	}
	if MatchCustom("www.example.com", custom) {
		t.Fatal("disabled entry should not match")
	}
}

func TestParseKeepsStandaloneComment(t *testing.T) {
	got, err := ParseCustomHosts("# lol\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Domain != "" || got[0].Comment != "lol" {
		t.Fatalf("got %#v", got)
	}
	round := FormatCustomHosts(got)
	if round != "# lol\n" {
		t.Fatalf("format %q", round)
	}
}

func TestParseCustomHosts(t *testing.T) {
	text := `
# Example note only
# github.com # Example: GitHub apex
*.example.org # live
example.org
`
	got, err := ParseCustomHosts(text)
	if err != nil {
		t.Fatal(err)
	}
	// comment-only + disabled github + enabled example.org (duplicate example.org skipped)
	if len(got) != 3 {
		t.Fatalf("len=%d %#v", len(got), got)
	}
	if got[0].Domain != "" || got[0].Comment != "Example note only" {
		t.Fatalf("comment entry: %#v", got[0])
	}
	if got[1].Domain != "github.com" || got[1].Enabled || !strings.Contains(got[1].Comment, "GitHub") {
		t.Fatalf("github entry: %#v", got[1])
	}
	if got[2].Domain != "example.org" || !got[2].Enabled || got[2].Comment != "live" {
		t.Fatalf("example entry: %#v", got[2])
	}
}

func TestFormatDefaultCustom(t *testing.T) {
	text := FormatCustomHosts(DefaultCustom())
	if !strings.Contains(text, "# github.com\n") {
		t.Fatalf("expected disabled domain on its own line, got %q", text)
	}
	if !strings.Contains(text, "# Example:") {
		t.Fatalf("expected comment line, got %q", text)
	}
	got, err := ParseCustomHosts(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("roundtrip len=%d %#v", len(got), got)
	}
	if got[0].Domain != "" || !strings.Contains(got[0].Comment, "Example:") {
		t.Fatalf("comment: %#v", got[0])
	}
	if got[1].Domain != "github.com" || got[1].Enabled {
		t.Fatalf("domain: %#v", got[1])
	}
}

func TestFormatEnabledAndDisabled(t *testing.T) {
	text := FormatCustomHosts([]CustomEntry{
		{Domain: "a.com", Comment: "off", Enabled: false},
		{Domain: "b.com", Comment: "on", Enabled: true},
	})
	want := "# off\n# a.com\n\nb.com # on\n"
	if text != want {
		t.Fatalf("got %q want %q", text, want)
	}
	round, err := ParseCustomHosts(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(round) != 3 {
		t.Fatalf("len=%d %#v", len(round), round)
	}
	if round[0].Comment != "off" || round[0].Domain != "" {
		t.Fatalf("round[0]=%#v", round[0])
	}
	if round[1].Domain != "a.com" || round[1].Enabled {
		t.Fatalf("round[1]=%#v", round[1])
	}
	if round[2].Domain != "b.com" || !round[2].Enabled || round[2].Comment != "on" {
		t.Fatalf("round[2]=%#v", round[2])
	}
}
