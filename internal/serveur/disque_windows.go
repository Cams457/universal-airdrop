//go:build windows

package serveur

import "golang.org/x/sys/windows"

// espaceLibre : octets disponibles pour l'utilisateur sur le volume de ce dossier.
func espaceLibre(dossier string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(dossier)
	if err != nil {
		return 0, err
	}
	var libre, total, totalLibre uint64
	if err := windows.GetDiskFreeSpaceEx(p, &libre, &total, &totalLibre); err != nil {
		return 0, err
	}
	return libre, nil
}
