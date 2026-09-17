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
)

type Server struct {
	mu       sync.Mutex
	flows    *flows.Writer
	upstream string
	udpConn  *net.UDPConn
	tcpLn    net.Listener
	wgIface  string
	enabled  bool
	stopCh   chan struct{}
}

func New(fw *flows.Writer, upstream, wgIface string) *Server {
	if upstream == "" {
		upstream = "1.1.1.1:53"
	}
	if !strings.Contains(upstream, ":") {
		upstream = upstream + ":53"
	}
	return &Server{flows: fw, upstream: upstream, wgIface: wgIface}
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

func (s *Server) handleUDP(query []byte, addr *net.UDPAddr) {
	start := time.Now()
	qname, qtype := parseQuestion(query)
	resp, err := forwardUDP(s.upstream, query)
	rtt := time.Since(start)
	rcode := -1
	answers := []string{}
	if err == nil && len(resp) > 3 {
		rcode = int(resp[3] & 0x0f)
		answers = parseAnswers(resp)
	}
	summary := fmt.Sprintf("DNS %s %s → %v (%s)", qtype, qname, answers, rtt.Round(time.Millisecond))
	s.flows.Emit(flows.Event{
		Type:    flows.TypeDNS,
		Summary: summary,
		Detail: map[string]any{
			"qname":   qname,
			"qtype":   qtype,
			"rcode":   rcode,
			"answers": answers,
			"rttMs":   rtt.Milliseconds(),
			"error":   errString(err),
			"proto":   "udp",
		},
	})
	if err == nil {
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
	qname, qtype := parseQuestion(query)
	resp, err := forwardTCP(s.upstream, query)
	rtt := time.Since(start)
	rcode := -1
	answers := []string{}
	if err == nil && len(resp) > 3 {
		rcode = int(resp[3] & 0x0f)
		answers = parseAnswers(resp)
	}
	s.flows.Emit(flows.Event{
		Type:    flows.TypeDNS,
		Summary: fmt.Sprintf("DNS %s %s → %v (%s)", qtype, qname, answers, rtt.Round(time.Millisecond)),
		Detail: map[string]any{
			"qname": qname, "qtype": qtype, "rcode": rcode,
			"answers": answers, "rttMs": rtt.Milliseconds(),
			"error": errString(err), "proto": "tcp",
		},
	})
	if err != nil {
		return
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
