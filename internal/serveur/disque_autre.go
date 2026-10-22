//go:build !windows

package serveur

import "golang.org/x/sys/unix"

// espaceLibre : octets disponibles pour l'utilisateur sur le volume de ce dossier.
func espaceLibre(dossier string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dossier, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
