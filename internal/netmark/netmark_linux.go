//go:build linux

package netmark

import (
	"context"
	"net"
	"syscall"
	"time"

	"github.com/kriakiku/potato-network/internal/config"
)

// DialContext dials with SO_MARK=MarkNoRedirect so traffic skips MITM REDIRECT.
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d := &net.Dialer{
		Timeout: 30 * time.Second,
		Control: markControl,
	}
	return d.DialContext(ctx, network, address)
}

// DialTimeout is DialContext with a fixed timeout (for TCP RTT probes).
func DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	d := &net.Dialer{
		Timeout: timeout,
		Control: markControl,
	}
	return d.Dial(network, address)
}

func markControl(network, address string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, config.MarkNoRedirect)
	})
	if err != nil {
		return err
	}
	return opErr
}
