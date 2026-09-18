package ignore

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		host string
		ok   bool
	}{
		{"google.com", true},
		{"www.google.com", true},
		{"clients3.google.com", true},
		{"connectivitycheck.gstatic.com", true},
		{"captive.apple.com", true},
		{"foo.bar.icloud.com", true},
		{"example.com", false},
		{"notgoogle.com", false},
		{"GOOGLE.COM", true},
		{"switchbot.net", true},
		{"api.switchbot.net", true},
		{"", false},
	}
	for _, c := range cases {
		if got := Match(c.host); got != c.ok {
			t.Errorf("Match(%q)=%v want %v", c.host, got, c.ok)
		}
	}
}
