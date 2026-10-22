package decouverte

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Cams457/universal-airdrop/internal/reseau"
)

func TestAnnonces(t *testing.T) {
	b, _ := json.Marshal(Annonce{App: "universal-airdrop", V: 1, Nom: "PC-A", FP: strings.Repeat("f", 64), Port: 47800, Lien: reseau.Ethernet, Mbits: 1000})
	a := Lire(b)
	if a == nil || a.Lien != reseau.Ethernet || a.Mbits != 1000 {
		t.Fatalf("annonce mal lue : %+v", a)
	}
	for _, brut := range []string{"", "{}", "pas du json", `{"app":"autre"}`, `{"app":"universal-airdrop","fp":"court","port":1}`,
		`{"app":"universal-airdrop","fp":"` + strings.Repeat("f", 64) + `","port":70000}`, strings.Repeat("x", 2000)} {
		if Lire([]byte(brut)) != nil {
			t.Errorf("annonce invalide acceptée : %.40q", brut)
		}
	}
	if Lire([]byte(`{"app":"universal-airdrop","fp":"`+strings.Repeat("f", 64)+`","port":1,"lien":"???","mbits":-3}`)).Lien != reseau.Autre {
		t.Error("lien inconnu non ramené à autre")
	}
}

func TestToutesLesAdresses(t *testing.T) {
	d := Nouvelle("B", strings.Repeat("b", 64), 1, nil)
	fp := strings.Repeat("a", 64)
	d.Enregistrer(&Annonce{Nom: "A", FP: fp, Port: 47800, Lien: reseau.WiFi}, "192.168.1.20")
	d.Enregistrer(&Annonce{Nom: "A", FP: fp, Port: 47800, Lien: reseau.Ethernet}, "192.168.1.21")
	if len(d.Actifs()[fp].Adresses) != 2 {
		t.Fatal("les deux adresses doivent être gardées")
	}
}

func TestEchangeMulticastReel(t *testing.T) {
	ctx, fin := context.WithCancel(context.Background())
	defer fin()
	vus := make(chan string, 10)
	a := Nouvelle("A", strings.Repeat("a", 64), 1111, nil)
	b := Nouvelle("B", strings.Repeat("b", 64), 2222, func(fp, ip string, port int) { vus <- fp })
	b.Autorisee = func(string) bool { return true } // le bac à sable utilise 192.0.2.x (plage de documentation)
	if err := a.Demarrer(ctx); err != nil {
		t.Skipf("multicast indisponible : %v", err)
	}
	if err := b.Demarrer(ctx); err != nil {
		t.Skipf("multicast indisponible : %v", err)
	}
	a.emettre()
	select {
	case fp := <-vus:
		if fp != strings.Repeat("a", 64) {
			t.Fatal("mauvais pair")
		}
	case <-time.After(3 * time.Second):
		t.Skip("aucun paquet multicast reçu (environnement sans multicast)")
	}
}
