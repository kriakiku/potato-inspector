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

	"github.com/potatoinspector/potato-inspector/internal/api"
	"github.com/potatoinspector/potato-inspector/internal/ca"
	"github.com/potatoinspector/potato-inspector/internal/catalog"
	"github.com/potatoinspector/potato-inspector/internal/config"
	"github.com/potatoinspector/potato-inspector/internal/cronbaseline"
	"github.com/potatoinspector/potato-inspector/internal/croncatalog"
	"github.com/potatoinspector/potato-inspector/internal/dnsfwd"
	"github.com/potatoinspector/potato-inspector/internal/mitm"
	"github.com/potatoinspector/potato-inspector/internal/rules"
	pnruntime "github.com/potatoinspector/potato-inspector/internal/runtime"
	"github.com/potatoinspector/potato-inspector/internal/shape"
)

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
	_ = st.ClearPassthrough()

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
