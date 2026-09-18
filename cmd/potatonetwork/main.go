package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	_ "github.com/breml/rootcerts" // embed Mozilla CA roots (no system ca-certificates package)

	"github.com/kriakiku/potato-network/internal/api"
	"github.com/kriakiku/potato-network/internal/ca"
	"github.com/kriakiku/potato-network/internal/catalog"
	"github.com/kriakiku/potato-network/internal/config"
	"github.com/kriakiku/potato-network/internal/cronbaseline"
	"github.com/kriakiku/potato-network/internal/croncatalog"
	"github.com/kriakiku/potato-network/internal/dnsfwd"
	"github.com/kriakiku/potato-network/internal/mitm"
	"github.com/kriakiku/potato-network/internal/rules"
	pnruntime "github.com/kriakiku/potato-network/internal/runtime"
	"github.com/kriakiku/potato-network/internal/shape"
)

// @title						PotatoNetwork API
// @version					1.0
// @description				Last-mile network emulator control plane for Docker sidecars.
// @description				Optional Bearer auth when POTATONETWORK_API_TOKEN is set.
// @contact.name				PotatoNetwork
// @contact.url				https://github.com/kriakiku/potato-network
// @license.name				See repository LICENSE
// @license.url				https://github.com/kriakiku/potato-network
// @host						localhost:7783
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Optional. Format: Bearer followed by the API token.
func main() {
	cfg := config.FromEnv()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	uplink, err := shape.ResolveUplink(cfg.Uplink)
	if err != nil {
		log.Fatalf("uplink: %v", err)
	}
	log.Printf("PotatoNetwork uplink=%s data=%s api=%s", uplink, cfg.DataDir, cfg.APIAddr)

	apiPort := 7783
	if _, portStr, err := net.SplitHostPort(cfg.APIAddr); err == nil {
		if p, err := strconv.Atoi(portStr); err == nil {
			apiPort = p
		}
	}

	cat, err := catalog.NewManager(cfg.DataDir)
	if err != nil {
		log.Fatalf("catalog: %v", err)
	}

	// Upstream from container resolv.conf (Docker 127.0.0.11 or compose `dns:`), then point clients at us.
	dnsUpstream := dnsfwd.DetectUpstream()
	if err := dnsfwd.PointClientsAtLocal(); err != nil {
		log.Printf("WARN: resolv.conf → 127.0.0.1: %v (set dns: [127.0.0.1] on this service)", err)
	}

	sh := shape.New(uplink, apiPort, dnsUpstream)
	st := pnruntime.New(cfg.DataDir, cat, sh)
	if cfg.ProfileCountry != "" {
		p, err := st.ApplyCountryTier(cfg.ProfileCountry, cfg.ProfileTier)
		if err != nil {
			log.Fatalf("boot profile %s/%s: %v", cfg.ProfileCountry, cfg.ProfileTier, err)
		}
		log.Printf("boot profile country=%s tier=%s id=%s", p.Country, p.Tier, p.ID)
	} else if err := st.ClearPassthrough(); err != nil {
		log.Fatalf("boot passthrough: %v", err)
	}

	bundle, err := ca.LoadOrCreate(cfg.DataDir)
	if err != nil {
		log.Fatalf("ca: %v", err)
	}
	log.Printf("CA ready at %s", ca.CertPath(cfg.DataDir))

	rulesPath := rules.Path(cfg.DataDir)
	if err := rules.EnsureDefault(rulesPath); err != nil {
		log.Fatalf("rules: %v", err)
	}
	eng := rules.New(rulesPath)
	if err := eng.Reload(); err != nil {
		log.Printf("rules compile warning: %v (fallback dest=cf)", err)
	}

	dns := dnsfwd.New(dnsUpstream)
	if err := dns.Start(); err != nil {
		log.Fatalf("dns: %v", err)
	}

	proxy := mitm.New(config.MITMPort, bundle, eng, st)
	if err := proxy.Start(); err != nil {
		log.Fatalf("mitm: %v", err)
	}

	cronbaseline.Start(cfg.BaselineCron, st)
	croncatalog.Start(cfg.CatalogCron, cat, cfg.RadarCatalogURL)

	srv := api.New(cfg, st, cat, sh, eng, bundle)
	httpSrv := &http.Server{Addr: cfg.APIAddr, Handler: srv.Handler()}
	go func() {
		log.Printf("API listening %s (auth=%v)", cfg.APIAddr, cfg.APIToken != "")
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("api: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("shutting down…")
	proxy.Stop()
	dns.Stop()
	_ = sh.Clear()
	_ = httpSrv.Close()
}
