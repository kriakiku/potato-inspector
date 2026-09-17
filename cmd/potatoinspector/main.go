package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/potatoinspector/potato-inspector/internal/catalog"
	"github.com/potatoinspector/potato-inspector/internal/config"
	"github.com/potatoinspector/potato-inspector/internal/dnsfwd"
	"github.com/potatoinspector/potato-inspector/internal/flows"
	"github.com/potatoinspector/potato-inspector/internal/ignore"
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

	cat, err := catalog.NewManager(st)
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

	ign := ignore.NewRuntime(settings.SystemIgnoreEnabled, settings.CustomIgnore)
	shaper := shape.New(cfg.WGIface)
	shaper.SetIgnoreExempt(ign.Active())

	addonDir := getenv("POTATOINSPECTOR_ADDON", "/app/mitmaddon")
	if _, err := os.Stat(addonDir); err != nil {
		if _, err2 := os.Stat("mitmaddon"); err2 == nil {
			addonDir = "mitmaddon"
		}
	}
	mm := mitm.New(st, fw, cat, addonDir, cfg.WGIface)
	_, _, _ = mm.EnsureCA()

	dns := dnsfwd.New(fw, settings.ClientDNS, cfg.WGIface, ign)

	wgOK := false
	if err := wgm.Start(); err != nil {
		log.Printf("WARN: WireGuard start failed (panel still up): %v", err)
	} else {
		wgOK = true
		log.Printf("WireGuard up on %s port %d pub=%s", cfg.WGIface, settings.WGPort, wgm.ServerPublicKey())
	}

	hostRtt := ensureHostRtt(cat, st, &settings)

	if wgOK {
		applied := false
		if settings.ActiveCountry != "" && settings.ActiveTier != "" {
			if p, err := cat.ProfileFor(settings.ActiveCountry, settings.ActiveTier, hostRtt); err == nil {
				if err := shaper.Apply(p); err != nil {
					log.Printf("WARN: apply catalog: %v", err)
				} else {
					applied = true
					settings.ActiveProfileID = p.ID
					_ = st.SaveSettings(settings)
				}
			} else {
				log.Printf("WARN: catalog profile: %v", err)
			}
		}
		if !applied {
			if p, ok := reg.Get(settings.ActiveProfileID); ok {
				if err := shaper.Apply(p); err != nil {
					log.Printf("WARN: apply profile: %v", err)
				}
			}
		}
		if settings.DNSIntercept {
			if err := dns.Start(); err != nil {
				log.Printf("WARN: DNS intercept: %v", err)
			}
		}
		if err := mm.Start(); err != nil {
			log.Printf("WARN: MITM start: %v", err)
		} else {
			settings.MITMEnabled = true
			_ = st.SaveSettings(settings)
		}
	}

	srv := panel.New(st, wgm, shaper, reg, cat, mm, dns, ign, fw, web.FS(), cfg.PanelPort, cfg.RadarCatalogURL)

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
	ignore.ClearMangle()
	wgm.Stop()
	_ = httpServer.Close()
}

func ensureHostRtt(cat *catalog.Manager, st *store.Store, settings *store.Settings) map[string]int {
	if len(settings.HostRtt) > 0 {
		cat.SetHostRtt(settings.HostRtt)
		log.Printf("host RTT restored from settings (%d dests)", len(settings.HostRtt))
		return settings.HostRtt
	}
	rtt := cat.ProbeHostRtt()
	settings.HostRtt = rtt
	settings.HostRttProbedAt = time.Now().UTC().Format(time.RFC3339)
	_ = st.SaveSettings(*settings)
	log.Printf("host RTT probed (%d dests)", len(rtt))
	return rtt
}

func loadOrInitSettings(st *store.Store, cfg config.Config) (store.Settings, error) {
	if store.Exists(st.SettingsPath()) {
		raw, _ := os.ReadFile(st.SettingsPath())
		settings, err := st.LoadSettings()
		if err != nil {
			return settings, err
		}
		changed := false
		if settings.ActiveCountry == "" {
			settings.ActiveCountry = "BD"
			changed = true
		}
		if settings.ActiveTier == "" {
			settings.ActiveTier = "typical"
			changed = true
		}
		if !settings.MITMEnabled {
			settings.MITMEnabled = true
			changed = true
		}
		// Older settings lacked the key → default on
		if !bytes.Contains(raw, []byte("systemIgnoreEnabled")) {
			settings.SystemIgnoreEnabled = true
			changed = true
		}
		if !bytes.Contains(raw, []byte("customIgnore")) {
			settings.CustomIgnore = ignore.DefaultCustom()
			changed = true
		}
		if changed {
			_ = st.SaveSettings(settings)
		}
		return settings, nil
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
