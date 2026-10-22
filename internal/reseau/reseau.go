// Package reseau liste les adresses IP du PC interface par interface, avec le type de lien
// (Ethernet, Wi-Fi, Bluetooth) et son débit négocié.
//
// Un PC peut avoir à la fois une IP de box (192.168.1.x), une IP du partage de connexion de
// l'iPhone (172.20.10.x, reçue en Wi-Fi, USB ou Bluetooth) et l'IP 192.168.137.1 s'il héberge
// lui-même un point d'accès mobile Windows.
package reseau

import (
	"net"
	"sort"
	"strings"
)

// Lien : type physique d'une interface.
type Lien string

const (
	Ethernet  Lien = "ethernet"
	WiFi      Lien = "wifi"
	Bluetooth Lien = "bluetooth"
	Autre     Lien = "autre"
)

// DebitParDefaut, en Mbit/s, quand la carte ne donne pas son débit.
var DebitParDefaut = map[Lien]int{Ethernet: 1000, WiFi: 300, Bluetooth: 2, Autre: 100}

// DebitBluetoothUtile : estimation du débit utile d'un PAN Bluetooth, à mesurer.
const DebitBluetoothUtile = 1.5

// Interface décrit une adresse IPv4 locale.
type Interface struct {
	IP     string `json:"ip"`
	Nom    string `json:"nom"`
	Lien   Lien   `json:"lien"`
	Mbits  int    `json:"mbits"` // débit PHYSIQUE négocié ; en Wi-Fi, le débit utile n'en vaut que 50 à 70 %
	Reseau string `json:"reseau"`
}

// lienSelonNom : repli quand le système ne donne pas le type (tests, autres OS).
func lienSelonNom(nom string) Lien {
	n := strings.ToLower(nom)
	switch {
	case strings.Contains(n, "bluetooth") || strings.HasPrefix(n, "bnep"):
		return Bluetooth
	case strings.Contains(n, "wi-fi") || strings.Contains(n, "wifi") || strings.Contains(n, "wlan") ||
		strings.Contains(n, "wireless") || strings.Contains(n, "sans fil") || strings.HasPrefix(n, "wl") ||
		strings.Contains(n, "local* "):
		return WiFi
	case strings.Contains(n, "ethernet") || strings.HasPrefix(n, "eth") || strings.HasPrefix(n, "en"):
		return Ethernet
	}
	return Autre
}

// TypeReseau décrit le réseau en clair pour l'utilisateur.
func TypeReseau(ip string, l Lien) string {
	switch {
	case l == Bluetooth:
		return "Bluetooth (partage de connexion du téléphone, 1 à 2 Mbit/s : petits fichiers)"
	case strings.HasPrefix(ip, "172.20.10."):
		return "partage de connexion de l'iPhone"
	case strings.HasPrefix(ip, "192.168.137."):
		return "point d'accès mobile de ce PC"
	case strings.HasPrefix(ip, "192.168.43."):
		return "partage de connexion d'un téléphone Android"
	case strings.HasPrefix(ip, "127."):
		return "boucle locale"
	}
	return "réseau local (box ou autre)"
}

// fournisseur est remplaçable par les tests.
var fournisseur = interfacesSysteme

// Interfaces : toutes les adresses IPv4 utilisables (sans 169.254.x.x ni boucle locale).
func Interfaces() []Interface {
	var sortie []Interface
	for _, i := range fournisseur() {
		if strings.HasPrefix(i.IP, "169.254.") || strings.HasPrefix(i.IP, "127.") {
			continue
		}
		if i.Mbits <= 0 {
			i.Mbits = DebitParDefaut[i.Lien]
		}
		i.Reseau = TypeReseau(i.IP, i.Lien)
		sortie = append(sortie, i)
	}
	return sortie
}

// IPs : ensemble des IP locales, avec ou sans la boucle locale.
func IPs(avecBoucle bool) []string {
	var ips []string
	for _, i := range Interfaces() {
		ips = append(ips, i.IP)
	}
	if avecBoucle {
		ips = append(ips, "127.0.0.1")
	}
	sort.Strings(ips)
	return ips
}

func priorite(i Interface) int {
	switch {
	case i.Lien == Bluetooth:
		return 3
	case strings.HasPrefix(i.IP, "172.20.10."), strings.HasPrefix(i.IP, "192.168.43."):
		return 0
	case strings.HasPrefix(i.IP, "192.168.137."):
		return 1
	}
	return 2
}

// Classees : ordre de préférence pour le QR code destiné à un téléphone. Sur un partage de
// connexion, le téléphone joint forcément le PC par cette adresse ; le Bluetooth vient en dernier.
func Classees() []Interface {
	l := Interfaces()
	sort.SliceStable(l, func(a, b int) bool {
		if priorite(l[a]) != priorite(l[b]) {
			return priorite(l[a]) < priorite(l[b])
		}
		return l[a].IP < l[b].IP
	})
	return l
}

// IPLocaleVers : IP locale que le système utiliserait pour joindre ip (aucun paquet n'est envoyé).
func IPLocaleVers(ip string) string {
	c, err := net.Dial("udp4", net.JoinHostPort(ip, "9"))
	if err != nil {
		return ""
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

// InterfaceDe renvoie l'interface qui porte cette IP locale.
func InterfaceDe(ip string) (Interface, bool) {
	for _, i := range Interfaces() {
		if i.IP == ip {
			return i, true
		}
	}
	return Interface{}, false
}

// CapaciteVers : capacité estimée d'un chemin = le plus lent des deux bouts.
func CapaciteVers(ip string, lienDistant Lien, mbitsDistant int) int {
	local := DebitParDefaut[Autre]
	if i, ok := InterfaceDe(IPLocaleVers(ip)); ok {
		local = i.Mbits
	}
	if mbitsDistant <= 0 {
		mbitsDistant = DebitParDefaut[lienDistant]
		if mbitsDistant == 0 {
			mbitsDistant = DebitParDefaut[Autre]
		}
	}
	return min(local, mbitsDistant)
}

// ViaBluetooth : le chemin vers ip passe-t-il par une interface Bluetooth ?
func ViaBluetooth(ip string) bool {
	i, ok := InterfaceDe(IPLocaleVers(ip))
	return ok && i.Lien == Bluetooth
}

// DureeBluetoothS : durée estimée d'un envoi de n octets en Bluetooth.
func DureeBluetoothS(n int64) float64 { return 8 * float64(n) / (DebitBluetoothUtile * 1e6) }

// interfacesGo : liste portable fondée sur net.Interfaces (le type est deviné d'après le nom).
func interfacesGo() []Interface {
	var sortie []Interface
	cartes, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, c := range cartes {
		if c.Flags&net.FlagUp == 0 {
			continue
		}
		adresses, _ := c.Addrs()
		for _, a := range adresses {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			sortie = append(sortie, Interface{IP: n.IP.String(), Nom: c.Name, Lien: lienSelonNom(c.Name)})
		}
	}
	return sortie
}
