//go:build !linux

package mitm

import (
	"fmt"
	"net"
	"time"

	"github.com/kriakiku/potato-network/internal/netmark"
)

func dialMarked(addr string) (net.Conn, error) {
	return netmark.DialTimeout("tcp", addr, 30*time.Second)
}

func originalDst(c net.Conn) (string, int, error) {
	if ra, ok := c.RemoteAddr().(*net.TCPAddr); ok {
		return ra.IP.String(), ra.Port, nil
	}
	return "", 0, fmt.Errorf("original dst requires linux REDIRECT")
}

func SetupRedirect(mitmPort int) error {
	return nil // no-op outside linux containers
}

func ClearRedirect() {}
