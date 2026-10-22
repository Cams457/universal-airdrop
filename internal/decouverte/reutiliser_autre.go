//go:build !windows

package decouverte

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// reutiliser permet à plusieurs sockets d'écouter le port multicast. SO_REUSEADDR seul : avec
// SO_REUSEPORT, Linux répartirait les datagrammes entre les sockets au lieu de les copier à chacun.
func reutiliser(_, _ string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	})
}
