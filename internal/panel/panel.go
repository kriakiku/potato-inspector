package panel

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
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
	"github.com/potatoinspector/potato-inspector/internal/ignore"
	"github.com/potatoinspector/potato-inspector/internal/mitm"
	"github.com/potatoinspector/potato-inspector/internal/profiles"
	"github.com/potatoinspector/potato-inspector/internal/shape"
	"github.com/potatoinspector/potato-inspector/internal/share"
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
	Ignore          *ignore.Runtime
	Flows           *flows.Writer
	Share           *share.Pad
	Static          fs.FS
	PanelPort       int
	RadarCatalogURL string
}

func New(st *store.Store, wgm *wg.Manager, sh *shape.Manager, pr *profiles.Registry, cat *catalog.Manager, mm *mitm.Manager, dns *dnsfwd.Server, ign *ignore.Runtime, fw *flows.Writer, pad *share.Pad, static fs.FS, panelPort int, radarURL string) *Server {
	if pad == nil {
		pad = share.New(st)
	}
	return &Server{
		Store:           st,
		WG:              wgm,
		Shape:           sh,
		Profiles:        pr,
		Catalog:         cat,
		MITM:            mm,
		DNS:             dns,
		Ignore:          ign,
		Flows:           fw,
		Share:           pad,
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
	mux.HandleFunc("/api/ignore", s.handleIgnore)
	mux.HandleFunc("/api/mitm", s.handleMITM)
	mux.HandleFunc("/api/mitm/ca.crt", s.handleCA)
	mux.HandleFunc("/api/mitm/ca/regenerate", s.handleCARegenerate)
	mux.HandleFunc("/api/mitm/test-regex", s.handleTestRegex)
	mux.HandleFunc("/api/inspector", s.handleInspector)
	mux.HandleFunc("/api/inspector/stream", s.handleInspectorStream)
	mux.HandleFunc("/api/inspector/clear", s.handleInspectorClear)
	mux.HandleFunc("/api/inspector/pause", s.handleInspectorPause)
	mux.HandleFunc("/api/inspector/har", s.handleHAR)
	mux.HandleFunc("/api/share", s.handleShare)
	mux.HandleFunc("/api/share/stream", s.handleShareStream)

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
		"mitmEnabled":      s.MITM.Enabled(),
		"forceDisableCache": st.ForceDisableCache,
		"disablePacketLoss": st.DisablePacketLoss,
		"dnsShortTtl":      st.DNSShortTTL,
		"dnsTtl":           st.DNSTTL,
		"systemIgnoreEnabled": st.SystemIgnoreEnabled,
		"captureEnabled":   true,
		"capturePaused":    s.Flows != nil && !s.Flows.Enabled(),
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
		"hostRtt":          hostRtt,
		"hostRttPinned":    st.HostRttPinned,
		"favoriteCountries": st.FavoriteCountries,
		"appliedDelayMs":   prof.DelayMs,
		"inspectorNote": func() string {
			if !s.MITM.Enabled() {
				return "MITM proxy not running yet (needs WireGuard/TUN). Capture will show DNS only until MITM starts."
			}
			return "MITM on: HTTP, TLS, and DNS events stream into the Inspector (Pause stops recording)."
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
			"captureEnabled":     true,
			"capturePaused":      s.Flows != nil && !s.Flows.Enabled(),
			"dnsIntercept":       true,
			"clientDns":          st.ClientDNS,
			"dnsShortTtl":        st.DNSShortTTL,
			"dnsTtl":             st.DNSTTL,
			"dnsRewriteRules":    st.DNSRewriteRules,
			"dnsBuiltinRules":    s.dnsBuiltinRules(),
			"favoriteCountries":  st.FavoriteCountries,
			"disablePacketLoss":  st.DisablePacketLoss,
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
		if v, ok := body["dnsShortTtl"].(bool); ok {
			st.DNSShortTTL = v
		}
		if v, ok := body["dnsTtl"].(float64); ok {
			st.DNSTTL = store.NormalizeDNSTTL(int(v))
		}
		if v, ok := body["disablePacketLoss"].(bool); ok {
			st.DisablePacketLoss = v
		}
		if _, ok := body["captureEnabled"]; ok {
			// Capture is always on; Pause in Inspector gates recording at runtime.
			st.CaptureEnabled = true
		}
		// DNS intercept is always on when WireGuard is up.
		st.DNSIntercept = true
		if s.DNS != nil && !s.DNS.Enabled() {
			_ = s.DNS.Start()
		}
		if raw, ok := body["dnsRewriteRules"]; ok {
			rules, err := parseDNSRewriteRules(raw)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			st.DNSRewriteRules = rules
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
		if s.Shape != nil {
			s.Shape.SetDisablePacketLoss(st.DisablePacketLoss)
		}
		if s.DNS != nil {
			if err := s.DNS.ApplyConfig(st.ClientDNS, st.DNSRewriteRules, st.DNSShortTTL, st.DNSTTL); err != nil {
				// Forwarder still updated; resolv.conf may be RO (e.g. macOS host run).
				writeJSON(w, 200, map[string]any{
					"ok":                true,
					"favoriteCountries": st.FavoriteCountries,
					"disablePacketLoss": st.DisablePacketLoss,
					"systemDnsWarning":  err.Error(),
				})
				_ = s.MITM.ReloadConfig()
				return
			}
		}
		_ = s.MITM.ReloadConfig()
		writeJSON(w, 200, map[string]any{
			"ok":                true,
			"favoriteCountries": st.FavoriteCountries,
			"disablePacketLoss": st.DisablePacketLoss,
		})
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

func parseDNSRewriteRules(raw any) ([]store.DNSRewriteRule, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("dnsRewriteRules: %w", err)
	}
	var rules []store.DNSRewriteRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("dnsRewriteRules must be array of {id,pattern,ip,enabled}")
	}
	out := make([]store.DNSRewriteRule, 0, len(rules))
	for _, r := range rules {
		r.Pattern = strings.TrimSpace(r.Pattern)
		r.IP = strings.TrimSpace(r.IP)
		if r.Pattern == "" && r.IP == "" {
			continue
		}
		if r.Pattern == "" {
			return nil, fmt.Errorf("dns rewrite rule missing pattern")
		}
		// Built-in portal names — always handled by dnsfwd, not user rules.
		if dnsfwd.IsBuiltinHost(r.Pattern) {
			continue
		}
		ip := net.ParseIP(r.IP)
		if ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("dns rewrite rule %q: need IPv4", r.Pattern)
		}
		r.IP = ip.To4().String()
		if r.ID == "" {
			r.ID = fmt.Sprintf("dns-%d-%d", time.Now().UnixNano(), len(out))
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *Server) dnsBuiltinRules() []map[string]any {
	ip := ""
	if s.WG != nil {
		if g := s.WG.GatewayIP(); g != nil {
			ip = g.String()
		}
	}
	return []map[string]any{
		{
			"id":      "builtin-potato-local",
			"pattern": dnsfwd.PortalHost,
			"ip":      ip,
			"enabled": true,
			"builtin": true,
			"note":    "CA install portal (http://" + dnsfwd.PortalHost + ")",
		},
		{
			"id":      "builtin-potato-share-local",
			"pattern": dnsfwd.ShareHost,
			"ip":      ip,
			"enabled": true,
			"builtin": true,
			"note":    "HTTPS Share API (https://" + dnsfwd.ShareHost + ")",
		},
	}
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
	dns := s.WG.GatewayIP().String()
	switch {
	case action == "conf" && r.Method == http.MethodGet:
		cfg := s.WG.ClientConfig(*peer, endpoint, dns)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.conf", peer.Name))
		_, _ = w.Write([]byte(cfg))
	case action == "qr" && r.Method == http.MethodGet:
		cfg := s.WG.ClientConfig(*peer, endpoint, dns)
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
	if body.ID == "passthrough" {
		prof, err := s.applyDirect()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "profile": prof, "status": s.Shape.Status()})
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
	_ = s.MITM.ReloadConfig()
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
		Direct  bool   `json:"direct"`
		Country string `json:"country"`
		Tier    string `json:"tier"`
		Probe   bool   `json:"probe"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad json"})
		return
	}
	if body.Direct || body.Country == "direct" {
		prof, err := s.applyDirect()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		hostRtt := map[string]int{}
		if s.Catalog != nil {
			hostRtt = s.Catalog.HostRtt()
		}
		writeJSON(w, 200, map[string]any{
			"ok":      true,
			"profile": prof,
			"hostRtt": hostRtt,
			"status":  s.Shape.Status(),
			"direct":  true,
		})
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

func (s *Server) applyDirect() (profiles.Profile, error) {
	p, ok := s.Profiles.Get("passthrough")
	if !ok {
		p = profiles.Profile{
			ID:          "passthrough",
			Name:        "Direct",
			Description: "No last-mile shaping; no MITM path delay.",
			Passthrough: true,
			Builtin:     true,
		}
	}
	if err := s.Shape.Apply(p); err != nil {
		return p, err
	}
	st, _ := s.Store.LoadSettings()
	st.ActiveProfileID = "passthrough"
	st.ActiveCountry = "direct"
	st.ActiveTier = ""
	if err := s.Store.SaveSettings(st); err != nil {
		return p, err
	}
	_ = s.MITM.ReloadConfig()
	return p, nil
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

func (s *Server) handleIgnore(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.LoadSettings()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, s.ignorePayload(st))
	case http.MethodPut:
		var body struct {
			Enabled    *bool                 `json:"enabled"`
			Custom     *[]ignore.CustomEntry `json:"custom"`
			CustomText *string               `json:"customText"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad json"})
			return
		}
		if body.Enabled == nil && body.Custom == nil && body.CustomText == nil {
			writeJSON(w, 400, map[string]string{"error": "enabled, custom, or customText required"})
			return
		}
		if body.Enabled != nil {
			st.SystemIgnoreEnabled = *body.Enabled
		}
		if body.CustomText != nil {
			parsed, err := ignore.ParseCustomHosts(*body.CustomText)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			// Keep exact user text; structured list is only for matching.
			st.CustomIgnoreText = *body.CustomText
			st.CustomIgnore = ignore.DomainEntries(parsed)
		} else if body.Custom != nil {
			validated, err := ignore.ValidateCustom(*body.Custom)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			st.CustomIgnore = ignore.DomainEntries(validated)
			st.CustomIgnoreText = ignore.FormatCustomHosts(validated)
		}
		if err := s.Store.SaveSettings(st); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if s.Ignore != nil {
			s.Ignore.SetSystemEnabled(st.SystemIgnoreEnabled)
			s.Ignore.SetCustom(st.CustomIgnore)
		}
		if s.Shape != nil {
			active := st.SystemIgnoreEnabled
			if !active {
				for _, e := range st.CustomIgnore {
					if e.Enabled {
						active = true
						break
					}
				}
			}
			s.Shape.SetIgnoreExempt(active)
		}
		_ = s.MITM.ReloadConfig()
		writeJSON(w, 200, s.ignorePayload(st))
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) ignorePayload(st store.Settings) map[string]any {
	text := st.CustomIgnoreText
	custom := st.CustomIgnore
	if custom == nil {
		custom = []ignore.CustomEntry{}
	}
	if text == "" && len(custom) > 0 {
		text = ignore.FormatCustomHosts(custom)
	}
	return map[string]any{
		"ok":         true,
		"enabled":    st.SystemIgnoreEnabled,
		"domains":    ignore.Domains(),
		"custom":     ignore.DomainEntries(custom),
		"customText": text,
	}
}

func (s *Server) handleMITM(w http.ResponseWriter, r *http.Request) {
	st, _ := s.Store.LoadSettings()
	rules, _ := s.Store.LoadMITMRules()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{
			"enabled":           true,
			"forceDisableCache": st.ForceDisableCache,
			"rules":             rules.Rules,
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
			ForceDisableCache *bool            `json:"forceDisableCache"`
			Rules             []store.MITMRule `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad json"})
			return
		}
		if body.ForceDisableCache != nil {
			st.ForceDisableCache = *body.ForceDisableCache
		}
		if body.Rules != nil {
			rules.Rules = body.Rules
		}
		st.MITMEnabled = true
		if !s.MITM.Enabled() {
			if err := s.MITM.Start(); err != nil {
				writeJSON(w, 500, map[string]string{"error": err.Error()})
				return
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

func (s *Server) handleCARegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	path, err := s.MITM.RegenerateCA()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	st, _ := s.Store.LoadSettings()
	st.MITMEnabled = true
	_ = s.Store.SaveSettings(st)
	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"running": s.MITM.Enabled(),
		"caPath":  path,
		"note":    "Re-install the new CA on clients; old CA is invalid.",
	})
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

func (s *Server) handleInspectorPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Paused *bool `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Paused == nil {
		writeJSON(w, 400, map[string]string{"error": "paused bool required"})
		return
	}
	s.Flows.SetEnabled(!*body.Paused)
	_ = s.MITM.ReloadConfig()
	writeJSON(w, 200, map[string]any{
		"ok":            true,
		"paused":        *body.Paused,
		"capturePaused": *body.Paused,
	})
}

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if s.Share == nil {
		writeJSON(w, 500, map[string]string{"error": "share unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, s.Share.Get())
	case http.MethodPut:
		var body struct {
			Text *string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == nil {
			writeJSON(w, 400, map[string]string{"error": "text string required"})
			return
		}
		snap, err := s.Share.Set(*body.Text)
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
	if s.Share == nil {
		http.Error(w, "share unavailable", 500)
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
	ch := s.Share.Subscribe()
	defer s.Share.Unsubscribe(ch)
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
