package wg

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// ResolveUplink picks the NAT egress interface.
//
// Priority:
//  1. POTATOINSPECTOR_UPLINK when set (must exist) — overrides saved settings
//  2. saved, if that interface exists
//  3. default-route interface (ip route)
//  4. error (caller should fail startup)
func ResolveUplink(saved string) (string, string, error) {
	if v, ok := os.LookupEnv("POTATOINSPECTOR_UPLINK"); ok {
		name := strings.TrimSpace(v)
		if name == "" {
			return "", "", fmt.Errorf("POTATOINSPECTOR_UPLINK is set but empty")
		}
		if !IfaceExists(name) {
			return "", "", fmt.Errorf("POTATOINSPECTOR_UPLINK=%q: interface not found", name)
		}
		return name, "env", nil
	}

	saved = strings.TrimSpace(saved)
	if saved != "" && IfaceExists(saved) {
		return saved, "settings", nil
	}

	if detected, err := DefaultRouteIface(); err == nil && detected != "" {
		reason := "default-route"
		if saved != "" {
			reason = fmt.Sprintf("default-route (saved %q missing)", saved)
		}
		return detected, reason, nil
	}

	if saved != "" {
		return "", "", fmt.Errorf("uplink %q not found and no default route interface", saved)
	}
	return "", "", fmt.Errorf("uplink not set: set POTATOINSPECTOR_UPLINK or ensure a default IPv4 route exists")
}

// IfaceExists reports whether a network interface is present.
func IfaceExists(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	_, err := net.InterfaceByName(name)
	return err == nil
}

// DefaultRouteIface returns the interface name of the first IPv4 default route.
func DefaultRouteIface() (string, error) {
	out, err := exec.Command("ip", "-4", "route", "show", "default").Output()
	if err != nil {
		return "", err
	}
	return parseDefaultRouteIface(string(out))
}

func parseDefaultRouteIface(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "default") {
			continue
		}
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "dev" {
				name := fields[i+1]
				if name != "" {
					return name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no default route device in: %q", strings.TrimSpace(out))
}
