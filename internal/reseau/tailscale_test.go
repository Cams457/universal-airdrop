package reseau

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTailscale(t *testing.T) {
	if !EstTailscale("100.101.102.103") || EstTailscale("100.128.0.1") || EstTailscale("192.168.1.2") {
		t.Fatal("plage 100.64.0.0/10 mal reconnue")
	}
	ancien, ancienF := commandeStatut, fournisseur
	defer func() { commandeStatut, fournisseur = ancien, ancienF; nomTS.quand = time.Time{} }()

	commandeStatut = func(context.Context) ([]byte, error) {
		return []byte(`{"BackendState":"Running","Self":{"DNSName":"PC-de-Camil.tail1234.ts.net."}}`), nil
	}
	nomTS.quand = time.Time{}
	if n := NomTailscale(); n != "pc-de-camil.tail1234.ts.net" {
		t.Fatalf("nom MagicDNS : %q", n)
	}
	commandeStatut = func(context.Context) ([]byte, error) { return []byte(`{"BackendState":"Stopped"}`), nil }
	nomTS.quand = time.Time{}
	if NomTailscale() != "" {
		t.Fatal("Tailscale arrêté : aucun nom attendu")
	}
	commandeStatut = func(context.Context) ([]byte, error) { return nil, errors.New("absent") }
	nomTS.quand = time.Time{}
	if NomTailscale() != "" {
		t.Fatal("Tailscale absent : aucun nom attendu")
	}

	fournisseur = func() []Interface {
		return []Interface{
			{IP: "100.101.102.103", Nom: "Tailscale", Lien: Autre, Mbits: 100000},
			{IP: "192.168.1.20", Nom: "Wi-Fi", Lien: WiFi, Mbits: 866},
		}
	}
	c := Classees()
	if len(c) != 2 || c[1].IP != "100.101.102.103" || c[1].Lien != Tailscale || c[1].Mbits != DebitTailscale {
		t.Fatalf("Tailscale doit venir après le réseau local, avec un débit réaliste : %+v", c)
	}
}
