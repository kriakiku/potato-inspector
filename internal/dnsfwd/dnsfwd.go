package dnsfwd

import (
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/potatoinspector/potato-inspector/internal/flows"
	"github.com/potatoinspector/potato-inspector/internal/ignore"
	"github.com/potatoinspector/potato-inspector/internal/store"
)

const (
	// PortalHost is the built-in name for the on-tunnel CA install portal.
	PortalHost = "potato.local"
)

type Server struct {
	mu        sync.Mutex
	flows     *flows.Writer
	upstream  string
	rules     []store.DNSRewriteRule
	shortTTL  bool
	ttlSec    uint32
	gatewayIP net.IP
	udpConn   *net.UDPConn
	tcpLn     net.Listener
	wgIface   string
	enabled   bool
	stopCh    chan struct{}
	ignore    *ignore.Runtime
}

func New(fw *flows.Writer, upstream, wgIface string, ign *ignore.Runtime) *Server {
	s := &Server{flows: fw, wgIface: wgIface, ignore: ign}
	s.SetConfig(upstream, nil, false, 30)
	return s
}

func (s *Server) SetGateway(ip net.IP) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ip == nil {
		s.gatewayIP = nil
		return
	}
	s.gatewayIP = append(net.IP(nil), ip.To4()...)
	if s.gatewayIP == nil {
		s.gatewayIP = append(net.IP(nil), ip...)
	}
}

func (s *Server) SetConfig(upstream string, rules []store.DNSRewriteRule, shortTTL bool, ttlSec int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if upstream == "" {
		upstream = "1.1.1.1:53"
	}
	if !strings.Contains(upstream, ":") {
		upstream = upstream + ":53"
	}
	s.upstream = upstream
	s.shortTTL = shortTTL
	s.ttlSec = uint32(store.NormalizeDNSTTL(ttlSec))
	if rules == nil {
		s.rules = nil
	} else {
		s.rules = append([]store.DNSRewriteRule{}, rules...)
	}
}

// ApplyConfig updates forwarder settings and writes Upstream DNS to /etc/resolv.conf.
func (s *Server) ApplyConfig(upstream string, rules []store.DNSRewriteRule, shortTTL bool, ttlSec int) error {
	s.SetConfig(upstream, rules, shortTTL, ttlSec)
	return ApplySystemResolver(upstream)
}

func (s *Server) configSnapshot() (upstream string, rules []store.DNSRewriteRule, shortTTL bool, ttlSec uint32, gateway net.IP) {
	s.mu.Lock()
	defer s.mu.Unlock()
	upstream = s.upstream
	shortTTL = s.shortTTL
	ttlSec = s.ttlSec
	if s.gatewayIP != nil {
		gateway = append(net.IP(nil), s.gatewayIP...)
	}
	if s.rules != nil {
		rules = append([]store.DNSRewriteRule{}, s.rules...)
	}
	return
}

func (s *Server) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled {
		return nil
	}
	addr, err := net.ResolveUDPAddr("udp", "0.0.0.0:5353")
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("dns udp listen: %w", err)
	}
	s.udpConn = conn
	tcpLn, err := net.Listen("tcp", "0.0.0.0:5353")
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("dns tcp listen: %w", err)
	}
	s.tcpLn = tcpLn
	s.stopCh = make(chan struct{})
	s.enabled = true
	SetupRedirect(s.wgIface)
	go s.serveUDP()
	go s.serveTCP()
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		return
	}
	close(s.stopCh)
	ClearRedirect(s.wgIface)
	if s.udpConn != nil {
		_ = s.udpConn.Close()
	}
	if s.tcpLn != nil {
		_ = s.tcpLn.Close()
	}
	s.enabled = false
}

func SetupRedirect(wgIface string) {
	_ = exec.Command("iptables", "-t", "nat", "-N", "POTATO_DNS").Run()
	_ = exec.Command("iptables", "-t", "nat", "-F", "POTATO_DNS").Run()
	_ = exec.Command("iptables", "-t", "nat", "-A", "POTATO_DNS", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-ports", "5353").Run()
	_ = exec.Command("iptables", "-t", "nat", "-A", "POTATO_DNS", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-ports", "5353").Run()
	_ = exec.Command("iptables", "-t", "nat", "-A", "PREROUTING", "-i", wgIface, "-j", "POTATO_DNS").Run()
}

func ClearRedirect(wgIface string) {
	_ = exec.Command("iptables", "-t", "nat", "-D", "PREROUTING", "-i", wgIface, "-j", "POTATO_DNS").Run()
	_ = exec.Command("iptables", "-t", "nat", "-F", "POTATO_DNS").Run()
	_ = exec.Command("iptables", "-t", "nat", "-X", "POTATO_DNS").Run()
}

func (s *Server) serveUDP() {
	buf := make([]byte, 4096)
	for {
		n, addr, err := s.udpConn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				return
			}
		}
		go s.handleUDP(append([]byte{}, buf[:n]...), addr)
	}
}

func (s *Server) resolve(query []byte) (resp []byte, qname, qtype string, rewritten bool, pattern string, err error) {
	upstream, rules, shortTTL, ttlSec, gateway := s.configSnapshot()
	qname, qtype = parseQuestion(query)
	if gateway != nil && gateway.To4() != nil && normalizeName(qname) == PortalHost {
		resp, err = buildRewriteResponse(query, gateway, ttlSec)
		return resp, qname, qtype, true, PortalHost, err
	}
	if rule := MatchRewrite(qname, rules); rule != nil {
		ip := net.ParseIP(strings.TrimSpace(rule.IP))
		if ip == nil || ip.To4() == nil {
			return nil, qname, qtype, false, "", fmt.Errorf("rewrite rule %q: bad ip %q", rule.Pattern, rule.IP)
		}
		resp, err = buildRewriteResponse(query, ip, ttlSec)
		return resp, qname, qtype, true, rule.Pattern, err
	}
	resp, err = forwardUDP(upstream, query)
	if err == nil && shortTTL && len(resp) > 0 {
		clampTTLs(resp, ttlSec)
	}
	return resp, qname, qtype, false, "", err
}

func (s *Server) resolveTCP(query []byte) (resp []byte, qname, qtype string, rewritten bool, pattern string, err error) {
	upstream, rules, shortTTL, ttlSec, gateway := s.configSnapshot()
	qname, qtype = parseQuestion(query)
	if gateway != nil && gateway.To4() != nil && normalizeName(qname) == PortalHost {
		resp, err = buildRewriteResponse(query, gateway, ttlSec)
		return resp, qname, qtype, true, PortalHost, err
	}
	if rule := MatchRewrite(qname, rules); rule != nil {
		ip := net.ParseIP(strings.TrimSpace(rule.IP))
		if ip == nil || ip.To4() == nil {
			return nil, qname, qtype, false, "", fmt.Errorf("rewrite rule %q: bad ip %q", rule.Pattern, rule.IP)
		}
		resp, err = buildRewriteResponse(query, ip, ttlSec)
		return resp, qname, qtype, true, rule.Pattern, err
	}
	resp, err = forwardTCP(upstream, query)
	if err == nil && shortTTL && len(resp) > 0 {
		clampTTLs(resp, ttlSec)
	}
	return resp, qname, qtype, false, "", err
}

func (s *Server) handleUDP(query []byte, addr *net.UDPAddr) {
	start := time.Now()
	resp, qname, qtype, rewritten, pattern, err := s.resolve(query)
	rtt := time.Since(start)
	rcode := -1
	answers := []string{}
	if err == nil && len(resp) > 3 {
		rcode = int(resp[3] & 0x0f)
		answers = parseAnswers(resp)
	}
	summary := fmt.Sprintf("DNS %s %s → %v (%s)", qtype, qname, answers, rtt.Round(time.Millisecond))
	if rewritten {
		summary = fmt.Sprintf("DNS %s %s → %v rewrite:%s (%s)", qtype, qname, answers, pattern, rtt.Round(time.Millisecond))
	}
	detail := map[string]any{
		"qname":   qname,
		"qtype":   qtype,
		"rcode":   rcode,
		"answers": answers,
		"rttMs":   rtt.Milliseconds(),
		"error":   errString(err),
		"proto":   "udp",
	}
	if rewritten {
		detail["rewritten"] = true
		detail["pattern"] = pattern
	}
	s.flows.Emit(flows.Event{
		Type:    flows.TypeDNS,
		Summary: summary,
		Detail:  detail,
	})
	if err == nil {
		if s.ignore != nil {
			s.ignore.ObserveDNS(qname, answers)
		}
		_, _ = s.udpConn.WriteToUDP(resp, addr)
	}
}

func (s *Server) serveTCP() {
	for {
		conn, err := s.tcpLn.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				return
			}
		}
		go s.handleTCP(conn)
	}
}

func (s *Server) handleTCP(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	var lenBuf [2]byte
	if _, err := conn.Read(lenBuf[:]); err != nil {
		return
	}
	n := binary.BigEndian.Uint16(lenBuf[:])
	query := make([]byte, n)
	if _, err := conn.Read(query); err != nil {
		return
	}
	start := time.Now()
	resp, qname, qtype, rewritten, pattern, err := s.resolveTCP(query)
	rtt := time.Since(start)
	rcode := -1
	answers := []string{}
	if err == nil && len(resp) > 3 {
		rcode = int(resp[3] & 0x0f)
		answers = parseAnswers(resp)
	}
	detail := map[string]any{
		"qname": qname, "qtype": qtype, "rcode": rcode,
		"answers": answers, "rttMs": rtt.Milliseconds(),
		"error": errString(err), "proto": "tcp",
	}
	if rewritten {
		detail["rewritten"] = true
		detail["pattern"] = pattern
	}
	summary := fmt.Sprintf("DNS %s %s → %v (%s)", qtype, qname, answers, rtt.Round(time.Millisecond))
	if rewritten {
		summary = fmt.Sprintf("DNS %s %s → %v rewrite:%s (%s)", qtype, qname, answers, pattern, rtt.Round(time.Millisecond))
	}
	s.flows.Emit(flows.Event{
		Type:    flows.TypeDNS,
		Summary: summary,
		Detail:  detail,
	})
	if err != nil {
		return
	}
	if s.ignore != nil {
		s.ignore.ObserveDNS(qname, answers)
	}
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(resp)))
	_, _ = conn.Write(lenBuf[:])
	_, _ = conn.Write(resp)
}

func forwardUDP(upstream string, query []byte) ([]byte, error) {
	c, err := net.DialTimeout("udp", upstream, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write(query); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	return buf[:n], err
}

func forwardTCP(upstream string, query []byte) ([]byte, error) {
	c, err := net.DialTimeout("tcp", upstream, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(query)))
	if _, err := c.Write(hdr[:]); err != nil {
		return nil, err
	}
	if _, err := c.Write(query); err != nil {
		return nil, err
	}
	if _, err := c.Read(hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(hdr[:])
	buf := make([]byte, n)
	_, err = c.Read(buf)
	return buf, err
}

func parseQuestion(msg []byte) (name, qtype string) {
	if len(msg) < 12 {
		return "?", "?"
	}
	name, off := decodeName(msg, 12)
	if off+4 > len(msg) {
		return name, "?"
	}
	t := binary.BigEndian.Uint16(msg[off : off+2])
	return name, typeName(t)
}

func parseAnswers(msg []byte) []string {
	if len(msg) < 12 {
		return nil
	}
	ancount := int(binary.BigEndian.Uint16(msg[6:8]))
	_, off := decodeName(msg, 12)
	off += 4 // qtype qclass
	var out []string
	for i := 0; i < ancount && off < len(msg); i++ {
		_, off2 := decodeName(msg, off)
		if off2+10 > len(msg) {
			break
		}
		typ := binary.BigEndian.Uint16(msg[off2 : off2+2])
		rdlen := int(binary.BigEndian.Uint16(msg[off2+8 : off2+10]))
		rdata := msg[off2+10 : off2+10+rdlen]
		off = off2 + 10 + rdlen
		switch typ {
		case 1: // A
			if len(rdata) == 4 {
				out = append(out, net.IP(rdata).String())
			}
		case 28: // AAAA
			if len(rdata) == 16 {
				out = append(out, net.IP(rdata).String())
			}
		case 5: // CNAME
			n, _ := decodeName(msg, off2+10)
			out = append(out, n)
		default:
			out = append(out, typeName(typ))
		}
	}
	return out
}

func decodeName(msg []byte, off int) (string, int) {
	var parts []string
	visited := 0
	start := off
	for off < len(msg) && visited < 20 {
		visited++
		l := int(msg[off])
		if l == 0 {
			off++
			break
		}
		if l&0xC0 == 0xC0 {
			if off+1 >= len(msg) {
				break
			}
			ptr := int(binary.BigEndian.Uint16(msg[off:off+2]) & 0x3FFF)
			n, _ := decodeName(msg, ptr)
			parts = append(parts, n)
			off += 2
			return strings.Join(parts, "."), off
		}
		off++
		if off+l > len(msg) {
			break
		}
		parts = append(parts, string(msg[off:off+l]))
		off += l
	}
	if off == start {
		off++
	}
	return strings.Join(parts, "."), off
}

func typeName(t uint16) string {
	switch t {
	case 1:
		return "A"
	case 2:
		return "NS"
	case 5:
		return "CNAME"
	case 15:
		return "MX"
	case 16:
		return "TXT"
	case 28:
		return "AAAA"
	default:
		return fmt.Sprintf("TYPE%d", t)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
