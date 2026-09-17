package panel

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"

	"github.com/potatoinspector/potato-inspector/internal/dnsfwd"
	"github.com/potatoinspector/potato-inspector/internal/flows"
	"github.com/potatoinspector/potato-inspector/internal/mitm"
	"github.com/potatoinspector/potato-inspector/internal/profiles"
	"github.com/potatoinspector/potato-inspector/internal/shape"
	"github.com/potatoinspector/potato-inspector/internal/store"
	"github.com/potatoinspector/potato-inspector/internal/wg"
)

type Server struct {
	Store     *store.Store
	WG        *wg.Manager
	Shape     *shape.Manager
	Profiles  *profiles.Registry
	MITM      *mitm.Manager
	DNS       *dnsfwd.Server
	Flows     *flows.Writer
	Static    fs.FS
	PanelPort int

	mu       sync.Mutex
	sessions map[string]time.Time
}

func New(st *store.Store, wgm *wg.Manager, sh *shape.Manager, pr *profiles.Registry, mm *mitm.Manager, dns *dnsfwd.Server, fw *flows.Writer, static fs.FS, panelPort int) *Server {
	return &Server{
		Store:     st,
		WG:        wgm,
		Shape:     sh,
		Profiles:  pr,
		MITM:      mm,
		DNS:       dns,
		Flows:     fw,
		Static:    static,
		PanelPort: panelPort,
		sessions:  map[string]time.Time{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.auth(s.handleLogout))
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/settings", s.auth(s.handleSettings))
	mux.HandleFunc("/api/peers", s.auth(s.handlePeers))
	mux.HandleFunc("/api/peers/", s.auth(s.handlePeerSub))
	mux.HandleFunc("/api/profiles", s.auth(s.handleProfiles))
	mux.HandleFunc("/api/profiles/apply", s.auth(s.handleApplyProfile))
	mux.HandleFunc("/api/mitm", s.auth(s.handleMITM))
	mux.HandleFunc("/api/mitm/ca.crt", s.auth(s.handleCA))
	mux.HandleFunc("/api/inspector", s.auth(s.handleInspector))
	mux.HandleFunc("/api/inspector/stream", s.auth(s.handleInspectorStream))
	mux.HandleFunc("/api/inspector/clear", s.auth(s.handleInspectorClear))
	mux.HandleFunc("/api/inspector/har", s.auth(s.handleHAR))
	mux.HandleFunc("/api/password", s.auth(s.handlePassword))

	if s.Static != nil {
		fileServer := http.FileServer(http.FS(s.Static))
		mux.Handle("/", spaHandler(s.Static, fileServer))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("PotatoInspector panel — build web/ assets"))
		})
	}
	return mux
}

func spaHandler(static fs.FS, files http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(static, path); err == nil && !strings.HasSuffix(path, "/") {
			files.ServeHTTP(w, r)
			return
		}
		data, err := fs.ReadFile(static, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("pi_session")
		if err != nil || !s.validSession(c.Value) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) validSession(tok string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[tok]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.sessions, tok)
		return false
	}
	return true
}

func (s *Server) newSession() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	s.mu.Lock()
	s.sessions[tok] = time.Now().Add(24 * time.Hour)
	s.mu.Unlock()
	return tok
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad json"})
		return
	}
	st, err := s.Store.LoadSettings()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(st.PasswordHash), []byte(body.Password)) != nil {
		writeJSON(w, 401, map[string]string{"error": "invalid password"})
		return
	}
	tok := s.newSession()
	http.SetCookie(w, &http.Cookie{
		Name:     "pi_session",
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("pi_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "pi_session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st, _ := s.Store.LoadSettings()
	peers, _ := s.WG.ListPeerStatus()
	activeHS := 0
	var lastHS time.Time
	for _, p := range peers {
		if !p.LastHandshake.IsZero() {
			activeHS++
			if p.LastHandshake.After(lastHS) {
				lastHS = p.LastHandshake
			}
		}
	}
	prof, _ := s.Profiles.Get(st.ActiveProfileID)
	writeJSON(w, 200, map[string]any{
		"product":         "PotatoInspector",
		"activeProfileId": st.ActiveProfileID,
		"profile":         prof,
		"mitmEnabled":     st.MITMEnabled && s.MITM.Enabled(),
		"captureEnabled":  st.CaptureEnabled,
		"captureCapacity": s.Flows.Capacity(),
		"dnsIntercept":    st.DNSIntercept && s.DNS.Enabled(),
		"peerCount":       len(peers),
		"peersHandshaking": activeHS,
		"lastHandshake":   lastHS,
		"qdisc":           s.Shape.Status(),
		"qdiscDump":       s.Shape.QdiscDump(),
		"wgPublicKey":     s.WG.ServerPublicKey(),
		"wgPort":          st.WGPort,
		"wgEndpoint":      st.WGEndpoint,
		"extraDelayMs":    st.ExtraDelayMs,
		"noteMitmOff":     !st.MITMEnabled,
		"inspectorNote": func() string {
			if !st.MITMEnabled {
				if st.DNSIntercept {
					if s.DNS.Enabled() {
						return "MITM off: HTTP/TLS bodies unavailable. DNS logging is active (port 53 intercept)."
					}
					return "MITM off: HTTP/TLS bodies unavailable. DNS intercept is configured but not running (needs WG/TUN)."
				}
				return "MITM off: HTTP/TLS inspector idle. DNS intercept is off."
			}
			return "MITM on: HTTP, TLS, and DNS events available when capture is on."
		}(),
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.LoadSettings()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{
			"wgEndpoint":      st.WGEndpoint,
			"wgSubnet":        st.WGSubnet,
			"wgPort":          st.WGPort,
			"uplink":          st.Uplink,
			"activeProfileId": st.ActiveProfileID,
			"mitmEnabled":     st.MITMEnabled,
			"extraDelayMs":    st.ExtraDelayMs,
			"captureEnabled":  st.CaptureEnabled,
			"dnsIntercept":    st.DNSIntercept,
			"clientDns":       st.ClientDNS,
		})
	case http.MethodPut:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad json"})
			return
		}
		if v, ok := body["wgEndpoint"].(string); ok {
			st.WGEndpoint = v
		}
		if v, ok := body["clientDns"].(string); ok {
			st.ClientDNS = v
		}
		if v, ok := body["extraDelayMs"].(float64); ok {
			st.ExtraDelayMs = int(v)
		}
		if v, ok := body["captureEnabled"].(bool); ok {
			st.CaptureEnabled = v
			s.Flows.SetEnabled(v)
		}
		if v, ok := body["dnsIntercept"].(bool); ok {
			st.DNSIntercept = v
			if v {
				_ = s.DNS.Start()
			} else {
				s.DNS.Stop()
			}
		}
		if err := s.Store.SaveSettings(st); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		_ = s.MITM.ReloadConfig()
		writeJSON(w, 200, map[string]string{"ok": "true"})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad json"})
		return
	}
	st, _ := s.Store.LoadSettings()
	if bcrypt.CompareHashAndPassword([]byte(st.PasswordHash), []byte(body.Current)) != nil {
		writeJSON(w, 401, map[string]string{"error": "invalid password"})
		return
	}
	if len(body.Next) < 4 {
		writeJSON(w, 400, map[string]string{"error": "password too short"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Next), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	st.PasswordHash = string(hash)
	if err := s.Store.SaveSettings(st); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		peers, err := s.WG.ListPeerStatus()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, peers)
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Name == "" {
			body.Name = "peer"
		}
		peer, _, err := s.WG.AddPeer(body.Name)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, peer)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePeerSub(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/peers/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	pf, err := s.Store.LoadPeers()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var peer *store.Peer
	for i := range pf.Peers {
		if pf.Peers[i].ID == id {
			peer = &pf.Peers[i]
			break
		}
	}
	if peer == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	st, _ := s.Store.LoadSettings()
	endpoint := st.WGEndpoint
	if endpoint != "" && !strings.Contains(endpoint, ":") {
		endpoint = fmt.Sprintf("%s:%d", endpoint, st.WGPort)
	}
	switch {
	case action == "conf" && r.Method == http.MethodGet:
		cfg := s.WG.ClientConfig(*peer, endpoint, st.ClientDNS)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.conf", peer.Name))
		_, _ = w.Write([]byte(cfg))
	case action == "qr" && r.Method == http.MethodGet:
		cfg := s.WG.ClientConfig(*peer, endpoint, st.ClientDNS)
		png, err := qrcode.Encode(cfg, qrcode.Medium, 256)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	case action == "" && r.Method == http.MethodDelete:
		if err := s.WG.RevokePeer(id); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "true"})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, s.Profiles.List())
	case http.MethodPost:
		var p profiles.Profile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad json"})
			return
		}
		if err := s.Profiles.SaveCustom(p); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, p)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if err := s.Profiles.DeleteCustom(id); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "true"})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleApplyProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad json"})
		return
	}
	p, ok := s.Profiles.Get(body.ID)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "unknown profile"})
		return
	}
	if err := s.Shape.Apply(p); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	st, _ := s.Store.LoadSettings()
	st.ActiveProfileID = body.ID
	_ = s.Store.SaveSettings(st)
	writeJSON(w, 200, map[string]any{"ok": true, "status": s.Shape.Status()})
}

func (s *Server) handleMITM(w http.ResponseWriter, r *http.Request) {
	st, _ := s.Store.LoadSettings()
	rules, _ := s.Store.LoadMITMRules()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{
			"enabled":      st.MITMEnabled,
			"extraDelayMs": st.ExtraDelayMs,
			"rules":        rules.Rules,
			"bypassSni":    rules.BypassSNI,
			"running":      s.MITM.Enabled(),
		})
	case http.MethodPut:
		var body struct {
			Enabled      *bool             `json:"enabled"`
			ExtraDelayMs *int              `json:"extraDelayMs"`
			Rules        []store.MITMRule  `json:"rules"`
			BypassSNI    []string          `json:"bypassSni"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad json"})
			return
		}
		if body.ExtraDelayMs != nil {
			st.ExtraDelayMs = *body.ExtraDelayMs
		}
		if body.Rules != nil {
			rules.Rules = body.Rules
		}
		if body.BypassSNI != nil {
			rules.BypassSNI = body.BypassSNI
		}
		if body.Enabled != nil {
			st.MITMEnabled = *body.Enabled
			if *body.Enabled {
				if err := s.MITM.Start(); err != nil {
					writeJSON(w, 500, map[string]string{"error": err.Error()})
					return
				}
			} else {
				_ = s.MITM.Stop()
			}
		}
		_ = s.Store.SaveSettings(st)
		_ = s.Store.SaveMITMRules(rules)
		_ = s.MITM.ReloadConfig()
		writeJSON(w, 200, map[string]string{"ok": "true"})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCA(w http.ResponseWriter, r *http.Request) {
	_, _, err := s.MITM.EnsureCA()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	data, err := os.ReadFile(s.MITM.CACertPath())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", "attachment; filename=potatoinspector-ca.crt")
	_, _ = w.Write(data)
}

func (s *Server) handleInspector(w http.ResponseWriter, r *http.Request) {
	typ := flows.EventType(r.URL.Query().Get("type"))
	evs := s.Flows.Recent(200, typ)
	writeJSON(w, 200, evs)
}

func (s *Server) handleInspectorClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	s.Flows.Clear()
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

func (s *Server) handleInspectorStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := s.Flows.Subscribe()
	defer s.Flows.Unsubscribe(ch)
	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleHAR(w http.ResponseWriter, r *http.Request) {
	evs := s.Flows.Recent(500, flows.TypeHTTP)
	entries := []map[string]any{}
	for _, ev := range evs {
		d := ev.Detail
		entries = append(entries, map[string]any{
			"startedDateTime": ev.Ts.Format(time.RFC3339Nano),
			"request": map[string]any{
				"method": d["method"],
				"url":    fmt.Sprintf("https://%v%v", d["host"], d["path"]),
				"headers": headersToHAR(d["requestHeaders"]),
			},
			"response": map[string]any{
				"status":  d["status"],
				"headers": headersToHAR(d["responseHeaders"]),
				"content": map[string]any{
					"text": d["bodyPreview"],
				},
			},
		})
	}
	har := map[string]any{
		"log": map[string]any{
			"version": "1.2",
			"creator": map[string]string{"name": "PotatoInspector", "version": "1.0"},
			"entries": entries,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=potatoinspector.har")
	_ = json.NewEncoder(w).Encode(har)
}

func headersToHAR(v any) []map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make([]map[string]string, 0, len(m))
	for k, val := range m {
		out = append(out, map[string]string{"name": k, "value": fmt.Sprint(val)})
	}
	return out
}

// SecureCompare is available if needed
var _ = subtle.ConstantTimeCompare
