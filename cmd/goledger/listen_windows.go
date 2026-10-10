//go:build windows

package main

import (
	"context"
	"errors"
	"net"
	"syscall"
)

// soExclusiveAddrUse is SO_EXCLUSIVEADDRUSE (^SO_REUSEADDR). Without it,
// Windows lets a socket bound to a specific address (127.0.0.1:8080) share the
// port with an existing wildcard listener (:8080), which would silently create
// a second, shadowing GoLedger listener.
const soExclusiveAddrUse = int(^uint32(syscall.SO_REUSEADDR))

const (
	wsaEACCES     = syscall.Errno(10013)
	wsaEADDRINUSE = syscall.Errno(10048)
)

func listenTCP(addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var sockErr error
			if err := c.Control(func(fd uintptr) {
				sockErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
			}); err != nil {
				return err
			}
			return sockErr
		},
	}
	return lc.Listen(context.Background(), "tcp", addr)
}

// isAddrInUse reports whether err means the address is taken. Windows reports
// WSAEACCES when an exclusive bind collides with an existing listener.
func isAddrInUse(err error) bool {
	return errors.Is(err, wsaEADDRINUSE) || errors.Is(err, wsaEACCES)
}
