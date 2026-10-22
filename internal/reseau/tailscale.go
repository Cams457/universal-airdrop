package reseau

import (
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Tailscale : réseau privé chiffré (WireGuard) qui relie tes appareils où qu'ils soient (4G, autre
// Wi-Fi). Chaque appareil y reçoit une adresse fixe dans 100.64.0.0/10 et un nom
// « appareil.xxxx.ts.net » (MagicDNS). Universal Airdrop s'en sert comme d'un réseau local de plus.
const Tailscale Lien = "tailscale"

// DebitTailscale : estimation prudente (direct sur le même Wi-Fi : bien plus ; par relais DERP : moins).
const DebitTailscale = 50

var plageTailscale = netip.MustParsePrefix("100.64.0.0/10")

// EstTailscale : l'adresse appartient-elle à la plage de Tailscale ?
func EstTailscale(ip string) bool {
	a, err := netip.ParseAddr(ip)
	return err == nil && plageTailscale.Contains(a)
}

var nomTS struct {
	sync.Mutex
	nom   string
	quand time.Time
}

// commandeStatut est remplaçable par les tests.
var commandeStatut = statutTailscale

// NomTailscale renvoie le nom MagicDNS de ce PC (« pc-de-camil.tail1234.ts.net »), ou "" si
// Tailscale n'est pas installé ou pas connecté. Résultat gardé une minute.
func NomTailscale() string {
	nomTS.Lock()
	defer nomTS.Unlock()
	if time.Since(nomTS.quand) < time.Minute {
		return nomTS.nom
	}
	nomTS.quand = time.Now()
	nomTS.nom = ""
	ctx, fin := context.WithTimeout(context.Background(), 3*time.Second)
	defer fin()
	brut, err := commandeStatut(ctx)
	if err != nil {
		return ""
	}
	var st struct {
		BackendState string
		Self         struct{ DNSName string }
	}
	if json.Unmarshal(brut, &st) == nil && st.BackendState == "Running" {
		nomTS.nom = strings.TrimSuffix(strings.ToLower(st.Self.DNSName), ".")
	}
	return nomTS.nom
}
