package dnsfwd

import (
	"encoding/binary"
	"net"
	"strings"

	"github.com/potatoinspector/potato-inspector/internal/store"
)

// MatchRewrite returns the first enabled rule whose pattern matches qname.
func MatchRewrite(qname string, rules []store.DNSRewriteRule) *store.DNSRewriteRule {
	name := normalizeName(qname)
	if name == "" {
		return nil
	}
	for i := range rules {
		r := &rules[i]
		if !r.Enabled {
			continue
		}
		if matchPattern(name, r.Pattern) {
			return r
		}
	}
	return nil
}

func normalizeName(qname string) string {
	n := strings.TrimSpace(qname)
	n = strings.TrimSuffix(n, ".")
	return strings.ToLower(n)
}

// matchPattern: exact "example.com" or wildcard suffix "*.domain.com" / "*.local".
func matchPattern(name, pattern string) bool {
	p := normalizeName(pattern)
	if p == "" || name == "" {
		return false
	}
	if strings.HasPrefix(p, "*.") {
		suffix := p[1:] // ".domain.com"
		if suffix == "." || suffix == "" {
			return false
		}
		return strings.HasSuffix(name, suffix) && name != strings.TrimPrefix(suffix, ".")
	}
	return name == p
}

func questionType(msg []byte) uint16 {
	if len(msg) < 12 {
		return 0
	}
	_, off := decodeName(msg, 12)
	if off+2 > len(msg) {
		return 0
	}
	return binary.BigEndian.Uint16(msg[off : off+2])
}

// buildRewriteResponse synthesizes an answer for a matched rule.
// A queries get an A RR with ttl; AAAA gets NOERROR with empty answers.
func buildRewriteResponse(query []byte, ip net.IP, ttl uint32) ([]byte, error) {
	if len(query) < 12 {
		return nil, errBadQuery
	}
	qtype := questionType(query)
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, errBadIP
	}

	// Copy header + question from query.
	_, qEnd := decodeName(query, 12)
	qEnd += 4 // qtype + qclass
	if qEnd > len(query) {
		return nil, errBadQuery
	}

	resp := make([]byte, 0, qEnd+16)
	resp = append(resp, query[:qEnd]...)

	// Flags: QR=1, AA=1, RA=1, RCODE=0; copy RD from query.
	resp[2] = 0x84 // QR | AA | (RD cleared then set below)
	if query[2]&0x01 != 0 {
		resp[2] |= 0x01 // RD
	}
	resp[3] = 0x80 // RA

	binary.BigEndian.PutUint16(resp[4:6], 1) // QDCOUNT
	ancount := uint16(0)
	if qtype == 1 { // A
		ancount = 1
	}
	binary.BigEndian.PutUint16(resp[6:8], ancount)
	binary.BigEndian.PutUint16(resp[8:10], 0)  // NSCOUNT
	binary.BigEndian.PutUint16(resp[10:12], 0) // ARCOUNT

	if ancount == 1 {
		// Name pointer to offset 12 (question name).
		resp = append(resp, 0xc0, 0x0c)
		resp = append(resp, 0x00, 0x01) // TYPE A
		resp = append(resp, 0x00, 0x01) // CLASS IN
		var ttlBuf [4]byte
		binary.BigEndian.PutUint32(ttlBuf[:], ttl)
		resp = append(resp, ttlBuf[:]...)
		resp = append(resp, 0x00, 0x04) // RDLENGTH
		resp = append(resp, ip4...)
	}
	return resp, nil
}

// clampTTLs sets every RR TTL in answer/authority/additional to 0.
func clampTTLs(msg []byte) {
	if len(msg) < 12 {
		return
	}
	qd := int(binary.BigEndian.Uint16(msg[4:6]))
	an := int(binary.BigEndian.Uint16(msg[6:8]))
	ns := int(binary.BigEndian.Uint16(msg[8:10]))
	ar := int(binary.BigEndian.Uint16(msg[10:12]))
	off := 12
	for i := 0; i < qd; i++ {
		_, off = decodeName(msg, off)
		off += 4
		if off > len(msg) {
			return
		}
	}
	total := an + ns + ar
	for i := 0; i < total; i++ {
		_, off2 := decodeName(msg, off)
		if off2+10 > len(msg) {
			return
		}
		// TTL at off2+4 .. off2+8
		msg[off2+4] = 0
		msg[off2+5] = 0
		msg[off2+6] = 0
		msg[off2+7] = 0
		rdlen := int(binary.BigEndian.Uint16(msg[off2+8 : off2+10]))
		off = off2 + 10 + rdlen
		if off > len(msg) {
			return
		}
	}
}

type rewriteError string

func (e rewriteError) Error() string { return string(e) }

const (
	errBadQuery rewriteError = "bad dns query"
	errBadIP    rewriteError = "bad ipv4"
)
