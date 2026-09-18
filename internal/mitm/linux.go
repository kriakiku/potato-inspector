//go:build linux

package mitm

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"github.com/kriakiku/potato-network/internal/config"
	"github.com/kriakiku/potato-network/internal/datapath"
)

func dialMarked(addr string) (net.Conn, error) {
	d := &net.Dialer{
		Timeout: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, config.MarkNoRedirect)
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}
	return d.Dial("tcp", addr)
}

func originalDst(c net.Conn) (string, int, error) {
	tcp, ok := c.(*net.TCPConn)
	if !ok {
		return "", 0, fmt.Errorf("not tcp")
	}
	rc, err := tcp.SyscallConn()
	if err != nil {
		return "", 0, err
	}
	const soOriginalDst = 80
	var ip net.IP
	var port int
	var opErr error
	err = rc.Control(func(fd uintptr) {
		var addr syscall.RawSockaddrInet4
		size := uint32(unsafe.Sizeof(addr))
		_, _, errno := syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			uintptr(syscall.IPPROTO_IP),
			uintptr(soOriginalDst),
			uintptr(unsafe.Pointer(&addr)),
			uintptr(unsafe.Pointer(&size)),
			0,
		)
		if errno != 0 {
			opErr = errno
			return
		}
		ip = net.IPv4(addr.Addr[0], addr.Addr[1], addr.Addr[2], addr.Addr[3])
		port = int(binary.BigEndian.Uint16((*[2]byte)(unsafe.Pointer(&addr.Port))[:]))
	})
	if err != nil {
		return "", 0, err
	}
	if opErr != nil {
		return "", 0, opErr
	}
	return ip.String(), port, nil
}

func SetupRedirect(mitmPort int) error {
	return datapath.InstallBase(mitmPort)
}

func ClearRedirect() {
	datapath.Teardown()
}
