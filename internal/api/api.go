package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/potatoinspector/potato-inspector/internal/ca"
	"github.com/potatoinspector/potato-inspector/internal/catalog"
	"github.com/potatoinspector/potato-inspector/internal/config"
	"github.com/potatoinspector/potato-inspector/internal/rules"
	pnruntime "github.com/potatoinspector/potato-inspector/internal/runtime"
	"github.com/potatoinspector/potato-inspector/internal/shape"
)

type Server struct {
	cfg     config.Config
	state   *pnruntime.State
	catalog *catalog.Manager
	shape   *shape.Manager
	rules   *rules.Engine
	ca      *ca.Bundle
	mux     *http.ServeMux
}

func New(cfg config.Config, st *pnruntime.State, cat *catalog.Manager, sh *shape.Manager, eng *rules.Engine, bundle *ca.Bundle) *Server {
	s := &Server{cfg: cfg, state: st, catalog: cat, shape: sh, rules: eng, ca: bundle, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.auth(s.mux) }

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.HandleFunc("/v1/health", s.handleHealth)
	s.mux.HandleFunc("/v1/profile", s.handleProfile)
	s.mux.HandleFunc("/v1/baseline", s.handleBaseline)
	s.mux.HandleFunc("/v1/baseline/probe", s.handleBaselineProbe)
	s.mux.HandleFunc("/v1/catalog", s.handleCatalog)
	s.mux.HandleFunc("/v1/catalog/refresh", s.handleCatalogRefresh)
	s.mux.HandleFunc("/v1/ca.pem", s.handleCA)
	s.mux.HandleFunc("/v1/rules", s.handleRules)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Docs redirect + health are public.
		if r.URL.Path == "/" || r.URL.Path == "/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		tok := s.cfg.APIToken
		if tok == "" {
			next.ServeHTTP(w, r)
			return
		}
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")) != tok {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	url := strings.TrimSpace(s.cfg.DocsURL)
	if url == "" {
		writeJSON(w, 200, map[string]any{
			"product": "PotatoNetwork",
			"api":     "/v1/",
			"docs":    "set POTATONETWORK_DOCS_URL",
		})
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"product": "PotatoNetwork",
		"profile": s.state.StatusSummary(),
		"shape":   s.shape.Status(),
	})
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		p := s.state.Profile()
		writeJSON(w, 200, p)
	case http.MethodPut:
		var body struct {
			Country     string `json:"country"`
			Tier        string `json:"tier"`
			Passthrough bool   `json:"passthrough"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if body.Passthrough || (body.Country == "" && body.Tier == "") {
			if err := s.state.ClearPassthrough(); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, s.state.Profile())
			return
		}
		if body.Tier == "" {
			body.Tier = "typical"
		}
		p, err := s.state.ApplyCountryTier(body.Country, body.Tier)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, p)
	default:
		w.WriteHeader(405)
	}
}

func (s *Server) handleBaseline(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{
			"hostRtt":  s.state.HostRtt(),
			"probedAt": formatTime(s.state.ProbedAt()),
		})
	case http.MethodPut:
		var body struct {
			HostRtt map[string]int `json:"hostRtt"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if len(body.HostRtt) == 0 {
			writeErr(w, 400, "hostRtt required")
			return
		}
		s.state.SetHostRtt(body.HostRtt)
		writeJSON(w, 200, map[string]any{
			"hostRtt":  s.state.HostRtt(),
			"probedAt": formatTime(s.state.ProbedAt()),
		})
	default:
		w.WriteHeader(405)
	}
}

func (s *Server) handleBaselineProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	rtt := s.state.ProbeBaseline()
	writeJSON(w, 200, map[string]any{
		"hostRtt":  rtt,
		"probedAt": formatTime(s.state.ProbedAt()),
	})
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	cat := s.catalog.Get()
	writeJSON(w, 200, cat)
}

func (s *Server) handleCatalogRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if err := s.catalog.RefreshFromURL(s.cfg.RadarCatalogURL); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "generatedAt": s.catalog.Get().GeneratedAt})
}

func (s *Server) handleCA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", "attachment; filename=\"potatonetwork-ca.pem\"")
	_, _ = w.Write(s.ca.CertPEM)
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	_ = s.rules.Reload()
	writeJSON(w, 200, map[string]any{
		"path":    s.rules.Path(),
		"source":  s.rules.Source(),
		"compile": s.rules.CompileErr(),
		"ok":      s.rules.CompileErr() == "",
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
