//go:build !linux

package netmark

import (
	"context"
	"net"
	"time"
)

// DialContext is a plain dial outside Linux (no nftables MITM).
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

// DialTimeout is a plain dial with timeout outside Linux.
func DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, address, timeout)
}
