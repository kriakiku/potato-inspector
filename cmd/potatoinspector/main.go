package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

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
		// local dev: relative to cwd
		if _, err2 := os.Stat("mitmaddon"); err2 == nil {
			addonDir = "mitmaddon"
		}
	}
	mm := mitm.New(st, fw, addonDir, cfg.WGIface)
	_, _, _ = mm.EnsureCA()

	dns := dnsfwd.New(fw, settings.ClientDNS, cfg.WGIface)

	// Start networking (may fail on macOS without TUN — log and continue for panel-only dev)
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

	certFile := filepath.Join(cfg.DataDir, "panel.crt")
	keyFile := filepath.Join(cfg.DataDir, "panel.key")
	if err := ensurePanelTLS(certFile, keyFile); err != nil {
		log.Fatal(err)
	}

	addr := fmt.Sprintf(":%d", cfg.PanelPort)
	httpServer := &http.Server{Addr: addr, Handler: srv.Handler()}

	go func() {
		log.Printf("Panel listening on https://0.0.0.0%s", addr)
		if err := httpServer.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
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
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Password), bcrypt.DefaultCost)
	if err != nil {
		return settings, err
	}
	settings.PasswordHash = string(hash)
	if err := st.SaveSettings(settings); err != nil {
		return settings, err
	}
	log.Printf("initialized settings (default password from POTATOINSPECTOR_PASSWORD or 'potato')")
	return settings, nil
}

func ensurePanelTLS(certFile, keyFile string) error {
	if store.Exists(certFile) && store.Exists(keyFile) {
		return nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "PotatoInspector Panel"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(5 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost", "potatoinspector"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	cf, err := os.Create(certFile)
	if err != nil {
		return err
	}
	_ = pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	_ = cf.Close()
	kf, err := os.OpenFile(keyFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	_ = pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	return kf.Close()
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
