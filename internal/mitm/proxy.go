package mitm

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kriakiku/potato-network/internal/ca"
	"github.com/kriakiku/potato-network/internal/rules"
	pnruntime "github.com/kriakiku/potato-network/internal/runtime"
)

// Proxy is a transparent HTTP(S) MITM (REDIRECT + SO_ORIGINAL_DST).
type Proxy struct {
	port   int
	ca     *ca.Bundle
	rules  *rules.Engine
	state  *pnruntime.State
	ln     net.Listener
	certMu sync.Mutex
	certs  map[string]*tls.Certificate
}

func New(port int, bundle *ca.Bundle, eng *rules.Engine, st *pnruntime.State) *Proxy {
	return &Proxy{
		port:  port,
		ca:    bundle,
		rules: eng,
		state: st,
		certs: make(map[string]*tls.Certificate),
	}
}

func (p *Proxy) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p.port))
	if err != nil {
		return err
	}
	p.ln = ln
	if err := SetupRedirect(p.port); err != nil {
		_ = ln.Close()
		return err
	}
	go p.acceptLoop()
	log.Printf("MITM transparent listening :%d", p.port)
	return nil
}

func (p *Proxy) Stop() {
	ClearRedirect()
	if p.ln != nil {
		_ = p.ln.Close()
	}
}

func (p *Proxy) acceptLoop() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		go p.handle(c)
	}
}

func (p *Proxy) handle(client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Minute))

	origIP, origPort, err := originalDst(client)
	if err != nil {
		log.Printf("mitm original dst: %v", err)
		return
	}
	br := bufio.NewReader(client)
	b, err := br.Peek(1)
	if err != nil {
		return
	}
	if b[0] == 0x16 {
		p.handleTLS(br, client, origIP, origPort)
		return
	}
	p.handleHTTP(br, client, origIP, origPort)
}

func (p *Proxy) handleHTTP(br *bufio.Reader, client net.Conn, origIP string, origPort int) {
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	host := req.Host
	if host == "" {
		host = net.JoinHostPort(origIP, strconv.Itoa(origPort))
	}
	target := net.JoinHostPort(origIP, strconv.Itoa(origPort))
	up, err := dialMarked(target)
	if err != nil {
		return
	}
	defer up.Close()
	if err := req.Write(up); err != nil {
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(up), req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	p.applyPathDelay("response", host, req.URL.Path, req.Header, resp.Header)
	_ = resp.Write(client)
}

func (p *Proxy) handleTLS(br *bufio.Reader, client net.Conn, origIP string, origPort int) {
	if d := p.state.HandshakeDelayMs(); d > 0 {
		time.Sleep(time.Duration(d) * time.Millisecond)
	}

	host := origIP
	if h, err := peekSNI(br); err == nil && h != "" {
		host = h
	}

	tlsCert, err := p.leafCert(host)
	if err != nil {
		log.Printf("mitm leaf %s: %v", host, err)
		return
	}
	tlsClient := tls.Server(&peekConn{Reader: br, Conn: client}, &tls.Config{
		Certificates: []tls.Certificate{*tlsCert},
		MinVersion:   tls.VersionTLS12,
	})
	if err := tlsClient.Handshake(); err != nil {
		return
	}
	defer tlsClient.Close()

	target := net.JoinHostPort(origIP, strconv.Itoa(origPort))
	rawUp, err := dialMarked(target)
	if err != nil {
		return
	}
	defer rawUp.Close()
	tlsUp := tls.Client(rawUp, &tls.Config{ServerName: host, InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err := tlsUp.Handshake(); err != nil {
		return
	}
	defer tlsUp.Close()

	cbr := bufio.NewReader(tlsClient)
	ubr := bufio.NewReader(tlsUp)
	for {
		_ = tlsClient.SetDeadline(time.Now().Add(2 * time.Minute))
		_ = tlsUp.SetDeadline(time.Now().Add(2 * time.Minute))
		req, err := http.ReadRequest(cbr)
		if err != nil {
			return
		}
		path := "/"
		if req.URL != nil {
			path = req.URL.RequestURI()
		}
		if err := req.Write(tlsUp); err != nil {
			return
		}
		resp, err := http.ReadResponse(ubr, req)
		if err != nil {
			return
		}
		p.applyPathDelay("response", host, path, req.Header, resp.Header)
		err = resp.Write(tlsClient)
		resp.Body.Close()
		if err != nil {
			return
		}
		if req.Close || resp.Close {
			return
		}
	}
}

func (p *Proxy) applyPathDelay(phase, host, path string, reqH, respH http.Header) {
	if p.state.Profile().Passthrough {
		return
	}
	res, err := p.rules.Eval(phase, host, path, headerMap(reqH), headerMap(respH))
	if err != nil {
		log.Printf("rules eval: %v", err)
		return
	}
	if res.DelayMs > 0 {
		time.Sleep(time.Duration(res.DelayMs) * time.Millisecond)
	}
}

func headerMap(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

func (p *Proxy) leafCert(host string) (*tls.Certificate, error) {
	p.certMu.Lock()
	defer p.certMu.Unlock()
	if c, ok := p.certs[host]; ok {
		return c, nil
	}
	key, leaf, err := p.ca.TLSCertificate(host)
	if err != nil {
		return nil, err
	}
	cert := &tls.Certificate{
		Certificate: [][]byte{leaf.Raw},
		PrivateKey:  key,
		Leaf:        leaf,
	}
	p.certs[host] = cert
	return cert, nil
}

type peekConn struct {
	io.Reader
	net.Conn
}

func (c *peekConn) Read(p []byte) (int, error) { return c.Reader.Read(p) }

func peekSNI(br *bufio.Reader) (string, error) {
	data, err := br.Peek(1024)
	if err != nil && len(data) < 43 {
		return "", fmt.Errorf("short")
	}
	if len(data) < 43 || data[0] != 0x16 {
		return "", fmt.Errorf("not handshake")
	}
	return parseSNI(data)
}

func parseSNI(data []byte) (string, error) {
	if len(data) < 9 {
		return "", fmt.Errorf("short")
	}
	p := 5
	if data[p] != 0x01 {
		return "", fmt.Errorf("not clienthello")
	}
	p++
	p += 3 // hs len
	p += 2 // version
	p += 32
	if p >= len(data) {
		return "", fmt.Errorf("short")
	}
	sidLen := int(data[p])
	p++
	p += sidLen
	if p+2 > len(data) {
		return "", fmt.Errorf("short")
	}
	csLen := int(data[p])<<8 | int(data[p+1])
	p += 2 + csLen
	if p >= len(data) {
		return "", fmt.Errorf("short")
	}
	compLen := int(data[p])
	p++
	p += compLen
	if p+2 > len(data) {
		return "", fmt.Errorf("no extensions")
	}
	extLen := int(data[p])<<8 | int(data[p+1])
	p += 2
	end := p + extLen
	if end > len(data) {
		end = len(data)
	}
	for p+4 <= end {
		typ := int(data[p])<<8 | int(data[p+1])
		l := int(data[p+2])<<8 | int(data[p+3])
		p += 4
		if p+l > end {
			break
		}
		if typ == 0 {
			q := p + 2
			if q+3 > p+l {
				break
			}
			if data[q] != 0 {
				p += l
				continue
			}
			q++
			nl := int(data[q])<<8 | int(data[q+1])
			q += 2
			if q+nl > p+l {
				break
			}
			return string(data[q : q+nl]), nil
		}
		p += l
	}
	return "", fmt.Errorf("no sni")
}
