package dnsfwd

import (
	"fmt"
	"net"
	"os"
	"strings"
)

const resolvPath = "/etc/resolv.conf"

// nameserverIP extracts an IPv4/IPv6 address from upstream (may include :53).
func nameserverIP(upstream string) (string, error) {
	host := strings.TrimSpace(upstream)
	if host == "" {
		host = "1.1.1.1"
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host, "]") && net.ParseIP(host) == nil {
		maybe := host[:i]
		if net.ParseIP(maybe) != nil {
			host = maybe
		}
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("upstream dns must be an IP, got %q", upstream)
	}
	return ip.String(), nil
}

// ApplySystemResolver writes Upstream DNS into /etc/resolv.conf for container lookups.
func ApplySystemResolver(upstream string) error {
	ip, err := nameserverIP(upstream)
	if err != nil {
		return err
	}
	body := fmt.Sprintf("# Managed by PotatoInspector\nnameserver %s\noptions edns0\n", ip)
	return os.WriteFile(resolvPath, []byte(body), 0o644)
}
