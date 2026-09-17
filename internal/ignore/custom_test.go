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
	}
	if !MatchCustom("api.github.com", custom) {
		t.Fatal("expected match api.github.com")
	}
	if !MatchCustom("github.com", custom) {
		t.Fatal("expected match github.com")
	}
	if MatchCustom("www.example.com", custom) {
		t.Fatal("disabled entry should not match")
	}
	if MatchAny("api.github.com", false, custom) != true {
		t.Fatal("MatchAny custom")
	}
	if MatchAny("google.com", false, custom) {
		t.Fatal("should not match builtin when system off")
	}
	if !MatchAny("google.com", true, custom) {
		t.Fatal("should match builtin when system on")
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
	if len(got) != 2 {
		t.Fatalf("len=%d %#v", len(got), got)
	}
	if got[0].Domain != "github.com" || got[0].Enabled || !strings.Contains(got[0].Comment, "GitHub") {
		t.Fatalf("github entry: %#v", got[0])
	}
	if got[1].Domain != "example.org" || !got[1].Enabled || got[1].Comment != "live" {
		t.Fatalf("example entry: %#v", got[1])
	}
	round, err := ParseCustomHosts(FormatCustomHosts(got))
	if err != nil {
		t.Fatal(err)
	}
	if len(round) != 2 || round[0].Domain != "github.com" || round[0].Enabled {
		t.Fatalf("roundtrip: %#v", round)
	}
}
