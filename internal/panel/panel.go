package panel

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/potatoinspector/potato-inspector/internal/catalog"
	"github.com/potatoinspector/potato-inspector/internal/dnsfwd"
	"github.com/potatoinspector/potato-inspector/internal/flows"
	"github.com/potatoinspector/potato-inspector/internal/mitm"
	"github.com/potatoinspector/potato-inspector/internal/profiles"
	"github.com/potatoinspector/potato-inspector/internal/shape"
	"github.com/potatoinspector/potato-inspector/internal/store"
	"github.com/potatoinspector/potato-inspector/internal/wg"
)

type Server struct {
	Store           *store.Store
	WG              *wg.Manager
	Shape           *shape.Manager
	Profiles        *profiles.Registry
	Catalog         *catalog.Manager
	MITM            *mitm.Manager
	DNS             *dnsfwd.Server
	Flows           *flows.Writer
	Static          fs.FS
	PanelPort       int
	RadarCatalogURL string
}

func New(st *store.Store, wgm *wg.Manager, sh *shape.Manager, pr *profiles.Registry, cat *catalog.Manager, mm *mitm.Manager, dns *dnsfwd.Server, fw *flows.Writer, static fs.FS, panelPort int, radarURL string) *Server {
	return &Server{
		Store:           st,
		WG:              wgm,
		Shape:           sh,
		Profiles:        pr,
		Catalog:         cat,
		MITM:            mm,
		DNS:             dns,
		Flows:           fw,
		Static:          static,
		PanelPort:       panelPort,
		RadarCatalogURL: radarURL,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/peers", s.handlePeers)
	mux.HandleFunc("/api/peers/", s.handlePeerSub)
	mux.HandleFunc("/api/profiles", s.handleProfiles)
	mux.HandleFunc("/api/profiles/apply", s.handleApplyProfile)
	mux.HandleFunc("/api/catalog", s.handleCatalog)
	mux.HandleFunc("/api/catalog/apply", s.handleCatalogApply)
	mux.HandleFunc("/api/catalog/refresh", s.handleCatalogRefresh)
	mux.HandleFunc("/api/catalog/probe", s.handleCatalogProbe)
	mux.HandleFunc("/api/catalog/baseline", s.handleCatalogBaseline)
	mux.HandleFunc("/api/mitm", s.handleMITM)
	mux.HandleFunc("/api/mitm/ca.crt", s.handleCA)
	mux.HandleFunc("/api/mitm/test-regex", s.handleTestRegex)
	mux.HandleFunc("/api/inspector", s.handleInspector)
	mux.HandleFunc("/api/inspector/stream", s.handleInspectorStream)
	mux.HandleFunc("/api/inspector/clear", s.handleInspectorClear)
	mux.HandleFunc("/api/inspector/har", s.handleHAR)

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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
	hostRtt := map[string]int{}
	if s.Catalog != nil {
		hostRtt = s.Catalog.HostRtt()
	}
	writeJSON(w, 200, map[string]any{
		"product":          "PotatoInspector",
		"activeProfileId":  st.ActiveProfileID,
		"activeCountry":    st.ActiveCountry,
		"activeTier":       st.ActiveTier,
		"profile":          prof,
		"mitmEnabled":      st.MITMEnabled && s.MITM.Enabled(),
		"captureEnabled":   st.CaptureEnabled,
		"captureCapacity":  s.Flows.Capacity(),
		"dnsIntercept":     st.DNSIntercept && s.DNS.Enabled(),
		"peerCount":        len(peers),
		"peersHandshaking": activeHS,
		"lastHandshake":    lastHS,
		"qdisc":            s.Shape.Status(),
		"qdiscDump":        s.Shape.QdiscDump(),
		"wgPublicKey":      s.WG.ServerPublicKey(),
		"wgPort":           st.WGPort,
		"wgEndpoint":       st.WGEndpoint,
		"extraDelayMs":     st.ExtraDelayMs,
		"hostRtt":          hostRtt,
		"hostRttPinned":    st.HostRttPinned,
		"favoriteCountries": st.FavoriteCountries,
		"appliedDelayMs":   prof.DelayMs,
		"noteMitmOff":      !st.MITMEnabled,
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
			"wgEndpoint":         st.WGEndpoint,
			"wgSubnet":           st.WGSubnet,
			"wgPort":             st.WGPort,
			"uplink":             st.Uplink,
			"activeProfileId":    st.ActiveProfileID,
			"mitmEnabled":        st.MITMEnabled,
			"extraDelayMs":       st.ExtraDelayMs,
			"captureEnabled":     st.CaptureEnabled,
			"dnsIntercept":       st.DNSIntercept,
			"clientDns":          st.ClientDNS,
			"favoriteCountries":  st.FavoriteCountries,
			"hostRttPinned":      st.HostRttPinned,
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
		if raw, ok := body["favoriteCountries"]; ok {
			favs, err := parseStringSlice(raw)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": "favoriteCountries must be string array"})
				return
			}
			st.FavoriteCountries = favs
		}
		if err := s.Store.SaveSettings(st); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		_ = s.MITM.ReloadConfig()
		writeJSON(w, 200, map[string]any{"ok": true, "favoriteCountries": st.FavoriteCountries})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func parseStringSlice(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("not array")
	}
	out := make([]string, 0, len(arr))
	seen := map[string]bool{}
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("not string")
		}
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
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

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if s.Catalog == nil {
		writeJSON(w, 500, map[string]string{"error": "catalog unavailable"})
		return
	}
	writeJSON(w, 200, s.Catalog.Get())
}

func (s *Server) handleCatalogApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Country string `json:"country"`
		Tier    string `json:"tier"`
		Probe   bool   `json:"probe"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad json"})
		return
	}
	if body.Country == "" || body.Tier == "" {
		writeJSON(w, 400, map[string]string{"error": "country and tier required"})
		return
	}
	prof, hostRtt, err := s.applyCatalog(body.Country, body.Tier, body.Probe)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"profile": prof,
		"hostRtt": hostRtt,
		"status":  s.Shape.Status(),
	})
}

func (s *Server) handleCatalogRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if s.Catalog == nil {
		writeJSON(w, 500, map[string]string{"error": "catalog unavailable"})
		return
	}
	url := s.RadarCatalogURL
	var body struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.URL != "" {
		url = body.URL
	}
	if err := s.Catalog.Pull(url); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	_ = s.MITM.ReloadConfig()
	writeJSON(w, 200, map[string]any{"ok": true, "catalog": s.Catalog.Get()})
}

func (s *Server) handleCatalogProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if s.Catalog == nil {
		writeJSON(w, 500, map[string]string{"error": "catalog unavailable"})
		return
	}
	rtt := s.Catalog.ProbeHostRtt()
	st, _ := s.Store.LoadSettings()
	st.HostRtt = rtt
	st.HostRttProbedAt = s.Catalog.LastProbe().UTC().Format(time.RFC3339)
	_ = s.Store.SaveSettings(st)
	_ = s.MITM.ReloadConfig()
	writeJSON(w, 200, s.baselinePayload(st))
}

func (s *Server) handleCatalogBaseline(w http.ResponseWriter, r *http.Request) {
	if s.Catalog == nil {
		writeJSON(w, 500, map[string]string{"error": "catalog unavailable"})
		return
	}
	st, _ := s.Store.LoadSettings()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, s.baselinePayload(st))
	case http.MethodPut:
		var body struct {
			HostRtt map[string]int `json:"hostRtt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad json"})
			return
		}
		if body.HostRtt == nil {
			writeJSON(w, 400, map[string]string{"error": "hostRtt required"})
			return
		}
		known := s.Catalog.Destinations()
		cleaned := make(map[string]int, len(body.HostRtt))
		for id, ms := range body.HostRtt {
			if _, ok := known[id]; !ok {
				writeJSON(w, 400, map[string]string{"error": fmt.Sprintf("unknown dest %s", id)})
				return
			}
			if ms < 0 {
				writeJSON(w, 400, map[string]string{"error": fmt.Sprintf("negative rtt for %s", id)})
				return
			}
			cleaned[id] = ms
		}
		s.Catalog.SetHostRtt(cleaned)
		st.HostRtt = cleaned
		st.HostRttProbedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.Store.SaveSettings(st); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		_ = s.MITM.ReloadConfig()
		writeJSON(w, 200, s.baselinePayload(st))
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) baselinePayload(st store.Settings) map[string]any {
	dests := s.Catalog.Destinations()
	ids := s.Catalog.DestinationIDs()
	list := make([]map[string]any, 0, len(ids))
	host := s.Catalog.HostRtt()
	if len(host) == 0 && len(st.HostRtt) > 0 {
		host = st.HostRtt
	}
	for _, id := range ids {
		d := dests[id]
		list = append(list, map[string]any{
			"id":     id,
			"label":  d.Label,
			"target": d.Target,
			"rttMs":  host[id],
		})
	}
	last := st.HostRttProbedAt
	if last == "" {
		if t := s.Catalog.LastProbe(); !t.IsZero() {
			last = t.UTC().Format(time.RFC3339)
		}
	}
	return map[string]any{
		"hostRtt":      host,
		"lastProbeAt":  last,
		"destinations": list,
	}
}

func (s *Server) applyCatalog(country, tier string, probe bool) (profiles.Profile, map[string]int, error) {
	if s.Catalog == nil {
		return profiles.Profile{}, nil, fmt.Errorf("catalog unavailable")
	}
	st, _ := s.Store.LoadSettings()
	hostRtt := s.Catalog.HostRtt()
	if len(hostRtt) == 0 && len(st.HostRtt) > 0 {
		s.Catalog.SetHostRtt(st.HostRtt)
		hostRtt = st.HostRtt
	}
	// Baseline is host location, not country profile — only probe when empty.
	if len(hostRtt) == 0 {
		hostRtt = s.Catalog.ProbeHostRtt()
		st.HostRtt = hostRtt
		st.HostRttProbedAt = s.Catalog.LastProbe().UTC().Format(time.RFC3339)
	}
	_ = probe
	prof, err := s.Catalog.ProfileFor(country, tier, hostRtt)
	if err != nil {
		return profiles.Profile{}, hostRtt, err
	}
	if err := s.Shape.Apply(prof); err != nil {
		return prof, hostRtt, err
	}
	st.ActiveCountry = country
	st.ActiveTier = tier
	st.ActiveProfileID = prof.ID
	_ = s.Store.SaveSettings(st)
	_ = s.MITM.ReloadConfig()
	return prof, hostRtt, nil
}

func (s *Server) handleMITM(w http.ResponseWriter, r *http.Request) {
	st, _ := s.Store.LoadSettings()
	rules, _ := s.Store.LoadMITMRules()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{
			"enabled":           st.MITMEnabled,
			"extraDelayMs":      st.ExtraDelayMs,
			"forceDisableCache": st.ForceDisableCache,
			"rules":             rules.Rules,
			"bypassSni":         rules.BypassSNI,
			"running":           s.MITM.Enabled(),
			"destinations": func() map[string]any {
				if s.Catalog == nil {
					return map[string]any{}
				}
				dests := s.Catalog.Destinations()
				out := map[string]any{}
				for _, id := range s.Catalog.DestinationIDs() {
					out[id] = dests[id]
				}
				return out
			}(),
		})
	case http.MethodPut:
		var body struct {
			Enabled           *bool            `json:"enabled"`
			ExtraDelayMs      *int             `json:"extraDelayMs"`
			ForceDisableCache *bool            `json:"forceDisableCache"`
			Rules             []store.MITMRule `json:"rules"`
			BypassSNI         []string         `json:"bypassSni"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad json"})
			return
		}
		if body.ExtraDelayMs != nil {
			st.ExtraDelayMs = *body.ExtraDelayMs
		}
		if body.ForceDisableCache != nil {
			st.ForceDisableCache = *body.ForceDisableCache
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

func (s *Server) handleTestRegex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Pattern    string `json:"pattern"`
		Text       string `json:"text"`
		IgnoreCase bool   `json:"ignoreCase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad json"})
		return
	}
	addon := os.Getenv("POTATOINSPECTOR_ADDON")
	if addon == "" {
		addon = "mitmaddon"
	}
	script := filepath.Join(addon, "test_regex.py")
	if _, err := os.Stat(script); err != nil {
		// fallback relative to cwd
		alt := filepath.Join("mitmaddon", "test_regex.py")
		if _, err2 := os.Stat(alt); err2 == nil {
			script = alt
		}
	}
	payload, _ := json.Marshal(body)
	cmd := exec.Command("python3", script)
	cmd.Stdin = strings.NewReader(string(payload))
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, 500, map[string]any{
			"ok":      false,
			"matched": false,
			"error":   fmt.Sprintf("python: %v (%s)", err, strings.TrimSpace(string(out))),
		})
		return
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		writeJSON(w, 500, map[string]any{
			"ok":      false,
			"matched": false,
			"error":   fmt.Sprintf("parse python output: %v (%s)", err, strings.TrimSpace(string(out))),
		})
		return
	}
	writeJSON(w, 200, result)
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
