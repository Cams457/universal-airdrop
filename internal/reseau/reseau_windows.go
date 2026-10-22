//go:build windows

package reseau

import (
	"net"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	ifTypeEthernet = 6
	ifTypeWiFi     = 71 // IF_TYPE_IEEE80211
)

// interfacesSysteme interroge Windows (GetAdaptersAddresses) : type réel de la carte et débit négocié.
func interfacesSysteme() []Interface {
	taille := uint32(16 * 1024)
	var tampon []byte
	for range 3 {
		tampon = make([]byte, taille)
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_SKIP_ANYCAST|
			windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER, 0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&tampon[0])), &taille)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return interfacesGo()
		}
	}
	var sortie []Interface
	for a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&tampon[0])); a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp {
			continue
		}
		nom := windows.UTF16PtrToString(a.FriendlyName)
		description := windows.UTF16PtrToString(a.Description)
		lien := Autre
		switch {
		case strings.Contains(strings.ToLower(description+" "+nom), "bluetooth"):
			lien = Bluetooth
		case a.IfType == ifTypeWiFi:
			lien = WiFi
		case a.IfType == ifTypeEthernet:
			lien = Ethernet
		default:
			lien = lienSelonNom(nom)
		}
		mbits := int(a.TransmitLinkSpeed / 1_000_000)
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			ip := u.Address.IP()
			if ip == nil || ip.To4() == nil {
				continue
			}
			sortie = append(sortie, Interface{IP: net.IP(ip.To4()).String(), Nom: nom, Lien: lien, Mbits: mbits})
		}
	}
	return sortie
}
