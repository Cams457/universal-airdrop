//go:build windows

package decouverte

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// reutiliser : sous Windows, SO_REUSEADDR suffit pour partager un port multicast UDP.
func reutiliser(_, _ string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		_ = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
	})
}
