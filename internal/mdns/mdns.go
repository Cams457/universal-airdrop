// Package mdns annonce le PC en Bonjour / mDNS : il répond au nom fixe « nom-du-pc.local ».
//
// Le Raccourci iOS n'a ainsi qu'UNE adresse à connaître, quel que soit le réseau : via la box, le nom
// se résout en 192.168.x.y ; via le partage de connexion de l'iPhone, en 172.20.10.x. iOS résout
// nativement les noms .local. Le nom est aussi inscrit dans le certificat du PC.
package mdns

import (
	"slices"
	"strings"
	"sync"

	"github.com/grandcat/zeroconf"

	"github.com/Cams457/universal-airdrop/internal/reseau"
)

const Service = "_universal-airdrop._tcp"

type Annonce struct {
	Nom, Hote, Empreinte string
	Port                 int

	mu  sync.Mutex
	srv *zeroconf.Server
	ips []string
}

func Nouvelle(nom, hote, empreinte string, port int) *Annonce {
	return &Annonce{Nom: nom, Hote: hote, Empreinte: empreinte, Port: port}
}

func (a *Annonce) enregistrer() error {
	a.ips = reseau.IPs(false)
	instance := a.Nom
	if len(instance) > 60 {
		instance = instance[:60]
	}
	srv, err := zeroconf.RegisterProxy(instance, Service, "local.", a.Port, a.Hote, a.ips,
		[]string{"fp=" + a.Empreinte[:16], "v=1"}, nil)
	if err != nil {
		return err
	}
	a.srv = srv
	return nil
}

func (a *Annonce) Demarrer() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.enregistrer()
}

// Actualiser : nouvelle IP (partage de connexion activé) -> nouvelle annonce. Renvoie true si changé.
func (a *Annonce) Actualiser() (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.srv == nil || slices.Equal(reseau.IPs(false), a.ips) {
		return false, nil
	}
	a.srv.Shutdown()
	return true, a.enregistrer()
}

func (a *Annonce) Arreter() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.srv != nil {
		a.srv.Shutdown()
		a.srv = nil
	}
}

// NomComplet : « pc-de-camil.local ».
func (a *Annonce) NomComplet() string { return strings.TrimSuffix(a.Hote, ".") + ".local" }
