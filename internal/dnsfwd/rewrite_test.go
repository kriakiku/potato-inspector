package dnsfwd

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/potatoinspector/potato-inspector/internal/store"
)

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		name, pattern string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"EXAMPLE.COM", "example.com", true},
		{"example.com.", "example.com", true},
		{"www.example.com", "example.com", false},
		{"foo.local", "*.local", true},
		{"a.b.local", "*.local", true},
		{"local", "*.local", false},
		{"bar.domain.com", "*.domain.com", true},
		{"domain.com", "*.domain.com", false},
		{"evil.domain.com.evil", "*.domain.com", false},
		{"x.y.domain.com", "*.domain.com", true},
	}
	for _, c := range cases {
		got := matchPattern(normalizeName(c.name), c.pattern)
		if got != c.want {
			t.Errorf("matchPattern(%q, %q)=%v want %v", c.name, c.pattern, got, c.want)
		}
	}
}

func TestMatchRewriteFirstWins(t *testing.T) {
	rules := []store.DNSRewriteRule{
		{Pattern: "*.local", IP: "10.0.0.1", Enabled: true},
		{Pattern: "foo.local", IP: "10.0.0.2", Enabled: true},
	}
	r := MatchRewrite("foo.local", rules)
	if r == nil || r.IP != "10.0.0.1" {
		t.Fatalf("expected first rule, got %+v", r)
	}
	rules[0].Enabled = false
	r = MatchRewrite("foo.local", rules)
	if r == nil || r.IP != "10.0.0.2" {
		t.Fatalf("expected second rule, got %+v", r)
	}
}

func buildQuery(name string, qtype uint16) []byte {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:2], 0x1234) // ID
	msg[2] = 0x01                                // RD
	binary.BigEndian.PutUint16(msg[4:6], 1)      // QDCOUNT
	for _, part := range splitName(name) {
		msg = append(msg, byte(len(part)))
		msg = append(msg, []byte(part)...)
	}
	msg = append(msg, 0)
	var tq [4]byte
	binary.BigEndian.PutUint16(tq[0:2], qtype)
	binary.BigEndian.PutUint16(tq[2:4], 1) // IN
	msg = append(msg, tq[:]...)
	return msg
}

func splitName(name string) []string {
	name = normalizeName(name)
	if name == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			parts = append(parts, name[start:i])
			start = i + 1
		}
	}
	parts = append(parts, name[start:])
	return parts
}

func TestBuildRewriteResponseA(t *testing.T) {
	q := buildQuery("foo.local", 1)
	resp, err := buildRewriteResponse(q, net.ParseIP("10.8.0.1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	answers := parseAnswers(resp)
	if len(answers) != 1 || answers[0] != "10.8.0.1" {
		t.Fatalf("answers=%v", answers)
	}
	// TTL at answer RR
	_, off := decodeName(resp, 12)
	off += 4 // skip question
	_, off2 := decodeName(resp, off)
	ttl := binary.BigEndian.Uint32(resp[off2+4 : off2+8])
	if ttl != 0 {
		t.Fatalf("ttl=%d want 0", ttl)
	}
}

func TestBuildEmptyAnswerAAAA(t *testing.T) {
	q := buildQuery("example.com", 28)
	resp, err := buildEmptyAnswer(q)
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint16(resp[6:8]) != 0 {
		t.Fatalf("ancount=%d want 0", binary.BigEndian.Uint16(resp[6:8]))
	}
	if ans := parseAnswers(resp); len(ans) != 0 {
		t.Fatalf("answers=%v want empty", ans)
	}
	// QR set
	if resp[2]&0x80 == 0 {
		t.Fatal("QR not set")
	}
}


func TestClampTTLs(t *testing.T) {
	q := buildQuery("example.com", 1)
	resp, err := buildRewriteResponse(q, net.ParseIP("1.2.3.4"), 300)
	if err != nil {
		t.Fatal(err)
	}
	_, off := decodeName(resp, 12)
	off += 4
	_, off2 := decodeName(resp, off)
	if binary.BigEndian.Uint32(resp[off2+4:off2+8]) != 300 {
		t.Fatal("setup ttl")
	}
	clampTTLs(resp, 0)
	if binary.BigEndian.Uint32(resp[off2+4:off2+8]) != 0 {
		t.Fatalf("ttl not clamped")
	}
}

func TestNameserverIP(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"1.1.1.1", "1.1.1.1"},
		{"1.1.1.1:53", "1.1.1.1"},
		{" 8.8.8.8 ", "8.8.8.8"},
	}
	for _, c := range cases {
		got, err := nameserverIP(c.in)
		if err != nil || got != c.want {
			t.Errorf("nameserverIP(%q)=%q,%v want %q", c.in, got, err, c.want)
		}
	}
	if _, err := nameserverIP("not-an-ip"); err == nil {
		t.Fatal("expected error")
	}
}

