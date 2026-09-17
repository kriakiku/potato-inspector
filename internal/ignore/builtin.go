package ignore

import (
	"strings"
)

// Builtin system / OEM domains. Match includes the apex and all subdomains.
var builtin = []string{
	// Google / Android
	"google.com",
	"googleapis.com",
	"gstatic.com",
	"googleusercontent.com",
	"ggpht.com",
	"android.com",
	"youtube.com",
	"ytimg.com",
	"googlezip.net",
	// Apple
	"apple.com",
	"icloud.com",
	"mzstatic.com",
	"appleiphonecell.com",
	"cdn-apple.com",
	"icloud-content.com",
	// Microsoft / Windows
	"microsoft.com",
	"windows.com",
	"msftncsi.com",
	"live.com",
	"office.com",
	"office.net",
	"msn.com",
}

// Domains returns a copy of the builtin suffix list.
func Domains() []string {
	out := make([]string, len(builtin))
	copy(out, builtin)
	return out
}

// Match reports whether host equals or is a subdomain of any builtin domain.
func Match(host string) bool {
	return MatchBuiltin(host)
}

// MatchBuiltin reports whether host matches the embedded OS/vendor pack.
func MatchBuiltin(host string) bool {
	h := normalizeHost(host)
	if h == "" {
		return false
	}
	for _, d := range builtin {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

// MatchAny is true if builtin (when systemOn) or an enabled custom entry matches.
func MatchAny(host string, systemOn bool, custom []CustomEntry) bool {
	if systemOn && MatchBuiltin(host) {
		return true
	}
	return MatchCustom(host, custom)
}

func normalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimSuffix(h, ".")
	if i := strings.IndexByte(h, ':'); i >= 0 {
		// strip accidental port
		h = h[:i]
	}
	return h
}
