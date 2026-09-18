package shareportal

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/potatoinspector/potato-inspector/internal/dnsfwd"
	"github.com/potatoinspector/potato-inspector/internal/mitm"
	"github.com/potatoinspector/potato-inspector/internal/share"
)

type Server struct {
	mu      sync.Mutex
	mitm    *mitm.Manager
	pad     *share.Pad
	httpSrv *http.Server
	enabled bool
}

func New(mm *mitm.Manager, pad *share.Pad) *Server {
	return &Server{mitm: mm, pad: pad}
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
	if s.pad == nil {
		return fmt.Errorf("share pad required")
	}
	if _, _, err := s.mitm.EnsureShareLeaf(); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/share", s.handleShare)
	mux.HandleFunc("/api/share/stream", s.handleShareStream)
	mux.HandleFunc("/favicon.svg", s.handleFavicon)

	tlsCfg := &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: s.getCertificate,
	}
	ln, err := tls.Listen("tcp", ":443", tlsCfg)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.httpSrv = srv
	s.enabled = true
	go func() {
		_ = srv.Serve(ln)
	}()
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled || s.httpSrv == nil {
		return
	}
	_ = s.httpSrv.Close()
	s.enabled = false
}

func (s *Server) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	certPath, keyPath, err := s.mitm.EnsureShareLeaf()
	if err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

func portalOrigin() string {
	return "http://" + dnsfwd.PortalHost
}

// applyCORS sets CORS headers for potato.local. Returns false if Origin is present but not allowed.
func applyCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && !strings.EqualFold(origin, portalOrigin()) {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", portalOrigin())
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Vary", "Origin")
	return true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !applyCORS(w, r) {
		http.Error(w, "cors", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if !applyCORS(w, r) {
		http.Error(w, "cors", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, s.pad.Get())
	case http.MethodPut:
		var body struct {
			Text *string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == nil {
			writeJSON(w, 400, map[string]string{"error": "text string required"})
			return
		}
		snap, err := s.pad.Set(*body.Text)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, snap)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleShareStream(w http.ResponseWriter, r *http.Request) {
	if !applyCORS(w, r) {
		http.Error(w, "cors", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := s.pad.Subscribe()
	defer s.pad.Unsubscribe(ch)
	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case snap, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(snap)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(faviconSVG))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <ellipse cx="16" cy="17" rx="11" ry="9.5" fill="#c4a35a"/>
  <ellipse cx="16" cy="16.5" rx="10" ry="8.5" fill="#d4b86a"/>
  <ellipse cx="12" cy="13" rx="2.2" ry="1.6" fill="#b8954a" opacity=".55"/>
  <ellipse cx="20" cy="14.5" rx="1.6" ry="1.2" fill="#b8954a" opacity=".5"/>
  <ellipse cx="14.5" cy="20" rx="1.8" ry="1.3" fill="#b8954a" opacity=".45"/>
  <ellipse cx="21" cy="19" rx="1.3" ry="1" fill="#b8954a" opacity=".4"/>
  <circle cx="11.5" cy="12.5" r=".7" fill="#6b4a28"/>
  <circle cx="19.5" cy="13.8" r=".55" fill="#6b4a28"/>
  <circle cx="14" cy="19.2" r=".6" fill="#6b4a28"/>
  <circle cx="20.5" cy="18.5" r=".5" fill="#6b4a28"/>
  <path d="M14 7.2c.4-1.6 1.6-2.6 2.8-2.4.3 1.4-.4 2.6-1.4 3.1-.6.3-1.3.1-1.4-.7z" fill="#5a8f3a"/>
  <path d="M16.2 5.2c.9-.2 1.8.3 2.1 1.2-.7.5-1.5.5-2.1 0V5.2z" fill="#4a7a30"/>
</svg>
`
