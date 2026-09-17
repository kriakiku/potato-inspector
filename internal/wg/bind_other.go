//go:build !linux

package wg

import "golang.zx2c4.com/wireguard/conn"

func NewStdNetBind() conn.Bind {
	return conn.NewDefaultBind()
}
