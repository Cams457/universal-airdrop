package reseau

import "testing"

func TestClassementEtBluetooth(t *testing.T) {
	ancien := fournisseur
	defer func() { fournisseur = ancien }()
	fournisseur = func() []Interface {
		return []Interface{
			{IP: "192.168.1.20", Nom: "Wi-Fi", Lien: WiFi, Mbits: 866},
			{IP: "172.20.10.3", Nom: "Connexion réseau Bluetooth", Lien: Bluetooth},
			{IP: "192.168.137.1", Nom: "Connexion au réseau local* 10", Lien: WiFi},
			{IP: "169.254.3.3", Nom: "x", Lien: Autre},
			{IP: "127.0.0.1", Nom: "lo", Lien: Autre},
		}
	}
	c := Classees()
	if len(c) != 3 || c[0].IP != "192.168.137.1" || c[1].IP != "192.168.1.20" || c[2].IP != "172.20.10.3" {
		t.Fatalf("ordre inattendu : %+v", c)
	}
	if c[2].Mbits != DebitParDefaut[Bluetooth] {
		t.Fatal("débit par défaut du Bluetooth non appliqué")
	}
	if d := DureeBluetoothS(500_000_000); d < 2500 || d > 2800 {
		t.Fatalf("durée Bluetooth pour 500 Mo : %.0f s", d)
	}
}

func TestLienSelonNom(t *testing.T) {
	cas := map[string]Lien{"Wi-Fi": WiFi, "wlan0": WiFi, "Ethernet 2": Ethernet, "eth0": Ethernet,
		"Connexion réseau Bluetooth": Bluetooth, "bnep0": Bluetooth, "lo": Autre}
	for nom, l := range cas {
		if lienSelonNom(nom) != l {
			t.Errorf("%s -> %s, attendu %s", nom, lienSelonNom(nom), l)
		}
	}
}
