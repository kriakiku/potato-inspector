package caportal

import "testing"

func TestDetectOS(t *testing.T) {
	cases := []struct {
		ua, want string
	}{
		{"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36", "android"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)", "ios"},
		{"Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)", "ios"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0)", "macos"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "windows"},
		{"curl/8.0", "unknown"},
	}
	for _, c := range cases {
		if got := detectOS(c.ua); got != c.want {
			t.Errorf("detectOS(%q)=%q want %q", c.ua, got, c.want)
		}
	}
}
