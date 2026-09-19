//go:build !linux

package datapath

import "net"

func InstallBase(mitmPort int) error { return nil }
func SetExempt(apiPort int, dnsUpstream string, exclude []net.IPNet) error { return nil }
func ClearExempt() {}
func Teardown() {}
