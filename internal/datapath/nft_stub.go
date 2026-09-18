//go:build !linux

package datapath

func InstallBase(mitmPort int) error { return nil }
func SetExempt(apiPort int, dnsUpstream string) error { return nil }
func ClearExempt() {}
func Teardown() {}
