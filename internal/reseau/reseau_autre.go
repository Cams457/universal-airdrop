//go:build !windows

package reseau

import (
	"os"
	"strconv"
	"strings"
)

// interfacesSysteme : sous Linux, le type et le débit viennent de /sys/class/net.
func interfacesSysteme() []Interface {
	l := interfacesGo()
	for k := range l {
		base := "/sys/class/net/" + l[k].Nom
		if _, err := os.Stat(base + "/wireless"); err == nil {
			l[k].Lien = WiFi
		}
		if brut, err := os.ReadFile(base + "/speed"); err == nil {
			if v, err := strconv.Atoi(strings.TrimSpace(string(brut))); err == nil && v > 0 {
				l[k].Mbits = v
			}
		}
	}
	return l
}
