package config

import (
	"os"
	"strings"
)

// Marks for local-exit datapath (nftables meta mark + tc fw filter).
const (
	MarkExemptShape = 0x1 // skip netem (API, DNS upstream)
	MarkNoRedirect  = 0x2 // skip MITM REDIRECT (MITM dial-out, API, DNS upstream)
)

// MITMPort is the internal transparent-proxy listen port (nftables REDIRECT target).
// Not published; not configurable — only the process and nftables need to agree.
const MITMPort = 8443

type Config struct {
	DataDir         string
	APIAddr         string
	APIToken        string
	CatalogCron     string
	BaselineCron    string
	Uplink          string
	RadarCatalogURL string
	DocsURL         string // browser hit on / → redirect here
}

func FromEnv() Config {
	token := strings.TrimSpace(os.Getenv("POTATONETWORK_API_TOKEN"))
	if token == "" {
		if f := strings.TrimSpace(os.Getenv("POTATONETWORK_API_TOKEN_FILE")); f != "" {
			if b, err := os.ReadFile(f); err == nil {
				token = strings.TrimSpace(string(b))
			}
		}
	}
	return Config{
		DataDir:      getenv("POTATONETWORK_DATA", "/data"),
		APIAddr:      getenv("POTATONETWORK_API_ADDR", ":7783"),
		APIToken:     token,
		CatalogCron:  getenv("POTATONETWORK_CATALOG_CRON", ""),
		BaselineCron: getenv("POTATONETWORK_BASELINE_CRON", ""),
		Uplink:       getenv("POTATONETWORK_UPLINK", ""),
		RadarCatalogURL: getenv(
			"POTATONETWORK_RADAR_CATALOG_URL",
			"https://raw.githubusercontent.com/kriakiku/potato-network/main/profiles/radar/catalog.json",
		),
		DocsURL: getenv(
			"POTATONETWORK_DOCS_URL",
			"https://kriakiku.github.io/potato-network/api/",
		),
	}
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
