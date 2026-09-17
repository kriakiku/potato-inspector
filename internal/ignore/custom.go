package ignore

import (
	"fmt"
	"regexp"
	"strings"
)

// CustomEntry is a user-managed ignore domain (suffix match like builtin).
// Domain == "" with Comment set is a free-standing comment line (# lol).
type CustomEntry struct {
	Domain  string `json:"domain"`
	Comment string `json:"comment"`
	Enabled bool   `json:"enabled"`
}

// DefaultCustom returns the seeded example list (github.com, off by default).
func DefaultCustom() []CustomEntry {
	return []CustomEntry{
		{
			Domain:  "github.com",
			Comment: "Example: GitHub apex + all subdomains (api, gist, raw, ...). Uncomment the next line to enable.",
			Enabled: false,
		},
	}
}

// DefaultCustomText is the seeded hosts textarea (exact text users see).
func DefaultCustomText() string {
	return `# Example: GitHub apex + all subdomains (api, gist, raw, ...). Uncomment the next line to enable.
# github.com
`
}

// DomainEntries returns only entries that have a domain (for matching / MITM).
func DomainEntries(entries []CustomEntry) []CustomEntry {
	out := make([]CustomEntry, 0, len(entries))
	for _, e := range entries {
		if strings.TrimSpace(e.Domain) == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

var domainRe = regexp.MustCompile(`^(?i)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$|^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizeDomain strips *. prefix, lowercases, validates. Returns apex form (e.g. github.com).
func NormalizeDomain(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "*.")
	s = strings.TrimPrefix(s, ".")
	s = strings.ToLower(strings.TrimSuffix(s, "."))
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "", fmt.Errorf("empty domain")
	}
	if len(s) > 253 {
		return "", fmt.Errorf("domain too long")
	}
	if strings.ContainsAny(s, " \t\n\r") {
		return "", fmt.Errorf("domain must not contain spaces")
	}
	if !domainRe.MatchString(s) {
		return "", fmt.Errorf("invalid domain %q (use github.com or *.github.com)", raw)
	}
	return s, nil
}

func trimComment(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 500 {
		return s[:500]
	}
	return s
}

// FormatCustomHosts renders entries as a hosts-like text block.
//
//	# free comment
//	# comment for disabled domain
//	# domain
//	domain # inline comment
func FormatCustomHosts(entries []CustomEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	first := true
	writeGap := func() {
		if !first {
			b.WriteByte('\n')
		}
		first = false
	}
	for _, e := range entries {
		d := strings.TrimSpace(e.Domain)
		comment := trimComment(e.Comment)
		if d == "" {
			if comment == "" {
				continue
			}
			writeGap()
			b.WriteString("# ")
			b.WriteString(comment)
			b.WriteByte('\n')
			continue
		}
		writeGap()
		if !e.Enabled {
			if comment != "" {
				b.WriteString("# ")
				b.WriteString(comment)
				b.WriteByte('\n')
			}
			b.WriteString("# ")
			b.WriteString(d)
			b.WriteByte('\n')
			continue
		}
		b.WriteString(d)
		if comment != "" {
			b.WriteString(" # ")
			b.WriteString(comment)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// ParseCustomHosts parses a hosts-like textarea.
// Lines:
//   domain [# comment]
//   *.domain [# comment]
//   # domain [# comment]   → disabled domain
//   # note                 → kept as a comment-only line
// Blank lines ignored. Duplicate domains: first wins.
func ParseCustomHosts(text string) ([]CustomEntry, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]CustomEntry, 0, len(lines))
	seen := map[string]bool{}

	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if rest == "" {
				continue
			}
			domainTok, inlineComment, tokOK := splitDomainComment(rest)
			if tokOK && looksLikeIgnoreDomain(domainTok) {
				if d, err := NormalizeDomain(domainTok); err == nil {
					if seen[d] {
						continue
					}
					seen[d] = true
					comment := trimComment(inlineComment)
					out = append(out, CustomEntry{Domain: d, Comment: comment, Enabled: false})
					continue
				}
			}
			// Free-standing comment, e.g. "# lol"
			out = append(out, CustomEntry{Comment: trimComment(rest)})
			continue
		}

		domainTok, inlineComment, ok := splitDomainComment(line)
		if !ok {
			return nil, fmt.Errorf("line %d: expected domain [# comment]", i+1)
		}
		d, err := NormalizeDomain(domainTok)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, CustomEntry{
			Domain:  d,
			Comment: trimComment(inlineComment),
			Enabled: true,
		})
	}
	return out, nil
}

// looksLikeIgnoreDomain is true for hosts we treat as domain lines when prefixed with #.
// Single-label words ("lol", "off") stay documentary comments; real names need a dot.
func looksLikeIgnoreDomain(raw string) bool {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "*.")
	s = strings.TrimPrefix(s, ".")
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	s = strings.ToLower(strings.TrimSuffix(s, "."))
	if s == "localhost" {
		return true
	}
	return strings.Contains(s, ".")
}

// splitDomainComment splits "domain # comment" or "domain".
func splitDomainComment(line string) (domain, comment string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}
	if i := strings.IndexByte(line, '#'); i >= 0 {
		domain = strings.TrimSpace(line[:i])
		comment = strings.TrimSpace(line[i+1:])
	} else {
		fields := strings.Fields(line)
		if len(fields) != 1 {
			return "", "", false
		}
		domain = fields[0]
	}
	if domain == "" {
		return "", "", false
	}
	return domain, comment, true
}

// ValidateCustom normalizes and dedupes domain entries; keeps comment-only lines.
func ValidateCustom(in []CustomEntry) ([]CustomEntry, error) {
	if in == nil {
		return []CustomEntry{}, nil
	}
	out := make([]CustomEntry, 0, len(in))
	seen := map[string]bool{}
	for i, e := range in {
		d := strings.TrimSpace(e.Domain)
		comment := trimComment(e.Comment)
		if d == "" {
			if comment == "" {
				continue
			}
			out = append(out, CustomEntry{Comment: comment})
			continue
		}
		norm, err := NormalizeDomain(d)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i+1, err)
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		out = append(out, CustomEntry{
			Domain:  norm,
			Comment: comment,
			Enabled: e.Enabled,
		})
	}
	return out, nil
}

// MatchCustom reports whether host matches an enabled custom suffix.
func MatchCustom(host string, custom []CustomEntry) bool {
	h := normalizeHost(host)
	if h == "" {
		return false
	}
	for _, e := range custom {
		if !e.Enabled || e.Domain == "" {
			continue
		}
		d := normalizeHost(e.Domain)
		if d == "" {
			continue
		}
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

// EnabledCustomDomains returns normalized domains that are enabled.
func EnabledCustomDomains(custom []CustomEntry) []string {
	var out []string
	for _, e := range custom {
		if !e.Enabled || e.Domain == "" {
			continue
		}
		d := normalizeHost(e.Domain)
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}
