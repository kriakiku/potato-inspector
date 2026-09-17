package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DataDir  string
	WGSubnet string
	WGPort   int
	PanelPort int
	Uplink   string
	WGIface  string
	Password string // bootstrap password from env (only used if no hash yet)
}

func FromEnv() Config {
	return Config{
		DataDir:   getenv("POTATOINSPECTOR_DATA", "/data"),
		WGSubnet:  getenv("POTATOINSPECTOR_WG_SUBNET", "10.8.0.0/24"),
		WGPort:    getenvInt("POTATOINSPECTOR_WG_PORT", 51820),
		PanelPort: getenvInt("POTATOINSPECTOR_PANEL", 8443),
		Uplink:    getenv("POTATOINSPECTOR_UPLINK", "eth0"),
		WGIface:   getenv("POTATOINSPECTOR_WG_IFACE", "wg0"),
		Password:  getenv("POTATOINSPECTOR_PASSWORD", "potato"),
	}
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
	if err != nil {
		return def
	}
	return n
}
