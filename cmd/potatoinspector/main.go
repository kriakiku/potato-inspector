package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/potatoinspector/potato-inspector/internal/config"
	"github.com/potatoinspector/potato-inspector/internal/dnsfwd"
	"github.com/potatoinspector/potato-inspector/internal/flows"
	"github.com/potatoinspector/potato-inspector/internal/mitm"
	"github.com/potatoinspector/potato-inspector/internal/panel"
	"github.com/potatoinspector/potato-inspector/internal/profiles"
	"github.com/potatoinspector/potato-inspector/internal/shape"
	"github.com/potatoinspector/potato-inspector/internal/store"
	"github.com/potatoinspector/potato-inspector/internal/wg"
	"github.com/potatoinspector/potato-inspector/web"
)

func main() {
	cfg := config.FromEnv()
	log.Printf("PotatoInspector starting data=%s wg=%s:%d panel=:%d", cfg.DataDir, cfg.WGSubnet, cfg.WGPort, cfg.PanelPort)

	st := store.New(cfg.DataDir)
	if err := st.EnsureDirs(); err != nil {
		log.Fatal(err)
	}

	settings, err := loadOrInitSettings(st, cfg)
	if err != nil {
		log.Fatal(err)
	}

	if !store.Exists(st.MITMRulesPath()) {
		_ = st.SaveMITMRules(store.DefaultMITMRules())
	}

	reg, err := profiles.NewRegistry(st)
	if err != nil {
		log.Fatal(err)
	}

	fw := flows.New()
	fw.SetEnabled(settings.CaptureEnabled)
	defer fw.Close()

	wgm, err := wg.NewManager(cfg.WGIface, settings.WGSubnet, settings.WGPort, settings.Uplink, st)
	if err != nil {
		log.Fatal(err)
	}

	shaper := shape.New(cfg.WGIface)
	addonDir := getenv("POTATOINSPECTOR_ADDON", "/app/mitmaddon")
	if _, err := os.Stat(addonDir); err != nil {
		if _, err2 := os.Stat("mitmaddon"); err2 == nil {
			addonDir = "mitmaddon"
		}
	}
	mm := mitm.New(st, fw, addonDir, cfg.WGIface)
	_, _, _ = mm.EnsureCA()

	dns := dnsfwd.New(fw, settings.ClientDNS, cfg.WGIface)

	if err := wgm.Start(); err != nil {
		log.Printf("WARN: WireGuard start failed (panel still up): %v", err)
	} else {
		log.Printf("WireGuard up on %s port %d pub=%s", cfg.WGIface, settings.WGPort, wgm.ServerPublicKey())
		if p, ok := reg.Get(settings.ActiveProfileID); ok {
			if err := shaper.Apply(p); err != nil {
				log.Printf("WARN: apply profile: %v", err)
			}
		}
		if settings.DNSIntercept {
			if err := dns.Start(); err != nil {
				log.Printf("WARN: DNS intercept: %v", err)
			}
		}
		if settings.MITMEnabled {
			if err := mm.Start(); err != nil {
				log.Printf("WARN: MITM start: %v", err)
			}
		}
	}

	srv := panel.New(st, wgm, shaper, reg, mm, dns, fw, web.FS(), cfg.PanelPort)

	addr := fmt.Sprintf(":%d", cfg.PanelPort)
	httpServer := &http.Server{Addr: addr, Handler: srv.Handler()}

	go func() {
		log.Printf("Panel listening on http://0.0.0.0%s (put TLS on your reverse proxy)", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Println("shutting down")
	_ = mm.Stop()
	dns.Stop()
	wgm.Stop()
	_ = httpServer.Close()
}

func loadOrInitSettings(st *store.Store, cfg config.Config) (store.Settings, error) {
	if store.Exists(st.SettingsPath()) {
		return st.LoadSettings()
	}
	settings := store.DefaultSettings(cfg.WGSubnet, cfg.WGPort, cfg.Uplink)
	if err := st.SaveSettings(settings); err != nil {
		return settings, err
	}
	log.Printf("initialized settings")
	return settings, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
