package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
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

// DocsURL is where GET / on the API port redirects (GitHub Pages / Hextra docs).
const DocsURL = "https://kriakiku.github.io/potato-network/api/"

type Config struct {
	DataDir         string
	APIAddr         string
	APIToken        string
	CatalogCron     string
	BaselineCron    string
	Uplink          string
	RadarCatalogURL string
	ProfileCountry  string // empty = passthrough at boot
	ProfileTier     string // used when ProfileCountry is set; default typical
	PathDelayMaxMs  int    // cap for rules.expr path delay; default 60000
	ShapeExclude    []net.IPNet
}

func FromEnv() (Config, error) {
	token := strings.TrimSpace(os.Getenv("POTATONETWORK_API_TOKEN"))
	if token == "" {
		if f := strings.TrimSpace(os.Getenv("POTATONETWORK_API_TOKEN_FILE")); f != "" {
			if b, err := os.ReadFile(f); err == nil {
				token = strings.TrimSpace(string(b))
			}
		}
	}
	country := strings.ToUpper(strings.TrimSpace(os.Getenv("POTATONETWORK_PROFILE_COUNTRY")))
	tier := ""
	if country != "" {
		tier = strings.TrimSpace(os.Getenv("POTATONETWORK_PROFILE_TIER"))
		if tier == "" {
			tier = "typical"
		}
	}
	exclude, err := ParseIPNets(os.Getenv("POTATONETWORK_SHAPE_EXCLUDE"))
	if err != nil {
		return Config{}, fmt.Errorf("POTATONETWORK_SHAPE_EXCLUDE: %w", err)
	}
	return Config{
		DataDir:        getenv("POTATONETWORK_DATA", "/data"),
		APIAddr:        getenv("POTATONETWORK_API_ADDR", ":7783"),
		APIToken:       token,
		CatalogCron:    getenv("POTATONETWORK_CATALOG_CRON", ""),
		BaselineCron:   getenv("POTATONETWORK_BASELINE_CRON", ""),
		Uplink:         getenv("POTATONETWORK_UPLINK", ""),
		ProfileCountry: country,
		ProfileTier:    tier,
		PathDelayMaxMs: getenvInt("POTATONETWORK_PATH_DELAY_MAX_MS", 60_000),
		ShapeExclude:   exclude,
		RadarCatalogURL: getenv(
			"POTATONETWORK_RADAR_CATALOG_URL",
			"https://raw.githubusercontent.com/kriakiku/potato-network/main/catalog.json",
		),
	}, nil
}

// ParseIPNets parses a comma/space-separated list of IPv4 addresses or CIDRs.
// Bare IPs become /32. Empty input yields nil.
func ParseIPNets(s string) ([]net.IPNet, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]net.IPNet, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, err := parseOneIPNet(f)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func parseOneIPNet(s string) (net.IPNet, error) {
	if strings.Contains(s, "/") {
		ip, ipnet, err := net.ParseCIDR(s)
		if err != nil {
			return net.IPNet{}, fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		if ip.To4() == nil {
			return net.IPNet{}, fmt.Errorf("IPv6 not supported: %q", s)
		}
		return net.IPNet{IP: ip.To4().Mask(ipnet.Mask), Mask: ipnet.Mask}, nil
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return net.IPNet{}, fmt.Errorf("invalid IP %q", s)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return net.IPNet{}, fmt.Errorf("IPv6 not supported: %q", s)
	}
	return net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}, nil
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
