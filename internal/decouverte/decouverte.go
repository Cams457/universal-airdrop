// Package decouverte : découverte automatique des PC par multicast UDP, sur CHAQUE interface.
//
// Envoyer seulement sur la route par défaut ne suffit pas : en partage de connexion, ou si le PC
// héberge un point d'accès, les voisins sont sur une autre interface que celle d'Internet.
// Chaque annonce dit sur quel type de lien elle part et à quel débit : l'émetteur choisira le chemin
// le plus rapide (Ethernet, puis Wi-Fi, puis Bluetooth).
//
// Sécurité : une annonce n'est qu'un indice d'adresse. Falsifiée, elle provoque au pire un échec de
// connexion, jamais une fuite, car TLS exige la CA épinglée.
package decouverte

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/reseau"
	"github.com/Cams457/universal-airdrop/internal/securite"
)

const (
	periode = 5 * time.Second
	oubli   = 30 * time.Second
)

// Annonce : message envoyé périodiquement.
type Annonce struct {
	App   string      `json:"app"`
	V     int         `json:"v"`
	Nom   string      `json:"nom"`
	FP    string      `json:"fp"`
	Port  int         `json:"port"`
	Lien  reseau.Lien `json:"lien"`
	Mbits int         `json:"mbits"`
}

// Lire valide une annonce reçue ; nil si elle est invalide.
func Lire(b []byte) *Annonce {
	if len(b) > 1024 {
		return nil
	}
	var a Annonce
	if json.Unmarshal(b, &a) != nil || a.App != "universal-airdrop" || len(a.FP) != 64 || a.Port <= 0 || a.Port > 65535 {
		return nil
	}
	if len([]rune(a.Nom)) > 64 {
		a.Nom = string([]rune(a.Nom)[:64])
	}
	switch a.Lien {
	case reseau.Ethernet, reseau.WiFi, reseau.Bluetooth:
	default:
		a.Lien = reseau.Autre
	}
	if a.Mbits < 0 || a.Mbits > 400_000 {
		a.Mbits = 0
	}
	return &a
}

// Adresse : une façon de joindre un pair.
type Adresse struct {
	Port  int         `json:"port"`
	Lien  reseau.Lien `json:"lien"`
	Mbits int         `json:"mbits"`
	Vu    time.Time   `json:"vu"`
}

// Pair : un PC vu sur le réseau, avec TOUTES ses adresses (une par interface).
type Pair struct {
	Nom      string              `json:"nom"`
	Adresses map[string]*Adresse `json:"adresses"`
}

type Decouverte struct {
	Nom, Empreinte string
	Port           int
	SurPair        func(fp, ip string, port int)
	// Autorisee filtre les sources (réseau local uniquement) ; remplaçable par les tests.
	Autorisee func(ip string) bool

	mu      sync.Mutex
	vus     map[string]*Pair
	pc      *ipv4.PacketConn
	conn    net.PacketConn
	groupe  *net.UDPAddr
	jointes map[string]bool
}

func Nouvelle(nom, empreinte string, port int, surPair func(fp, ip string, port int)) *Decouverte {
	return &Decouverte{Nom: nom, Empreinte: empreinte, Port: port, SurPair: surPair, vus: map[string]*Pair{},
		Autorisee: securite.IPAutorisee,
		jointes:   map[string]bool{}, groupe: &net.UDPAddr{IP: net.ParseIP(config.GroupeMulticast), Port: config.PortMulticast}}
}

// DefinirNom : renommage du PC, annoncé dès la prochaine émission.
func (d *Decouverte) DefinirNom(nom string) {
	d.mu.Lock()
	d.Nom = nom
	d.mu.Unlock()
	go d.emettre()
}

// Enregistrer : appelée à chaque annonce reçue.
func (d *Decouverte) Enregistrer(a *Annonce, ip string) {
	d.mu.Lock()
	p, ok := d.vus[a.FP]
	if !ok {
		p = &Pair{Adresses: map[string]*Adresse{}}
		d.vus[a.FP] = p
	}
	p.Nom = a.Nom
	p.Adresses[ip] = &Adresse{Port: a.Port, Lien: a.Lien, Mbits: a.Mbits, Vu: time.Now()}
	d.mu.Unlock()
	if d.SurPair != nil {
		d.SurPair(a.FP, ip, a.Port)
	}
}

// Actifs : pairs vus récemment, avec leurs adresses encore fraîches.
func (d *Decouverte) Actifs() map[string]Pair {
	d.mu.Lock()
	defer d.mu.Unlock()
	sortie := map[string]Pair{}
	for fp, p := range d.vus {
		fr := map[string]*Adresse{}
		for ip, a := range p.Adresses {
			if time.Since(a.Vu) < oubli {
				c := *a
				fr[ip] = &c
			}
		}
		if len(fr) > 0 {
			sortie[fp] = Pair{Nom: p.Nom, Adresses: fr}
		}
	}
	return sortie
}

// carte retrouve l'interface système qui porte une IP.
func carte(ip string) *net.Interface {
	cartes, _ := net.Interfaces()
	for i := range cartes {
		adresses, _ := cartes[i].Addrs()
		for _, a := range adresses {
			if n, ok := a.(*net.IPNet); ok && n.IP.String() == ip {
				return &cartes[i]
			}
		}
	}
	return nil
}

func (d *Decouverte) joindre() {
	for _, ip := range reseau.IPs(true) {
		if d.jointes[ip] {
			continue
		}
		if c := carte(ip); c != nil && d.pc.JoinGroup(c, d.groupe) == nil {
			d.jointes[ip] = true
		}
	}
}

func (d *Decouverte) emettre() {
	interfaces := map[string]reseau.Interface{}
	for _, i := range reseau.Interfaces() {
		interfaces[i.IP] = i
	}
	for _, ip := range reseau.IPs(true) {
		c := carte(ip)
		if c == nil {
			continue
		}
		i := interfaces[ip]
		d.mu.Lock()
		nom := d.Nom
		d.mu.Unlock()
		msg, _ := json.Marshal(Annonce{App: "universal-airdrop", V: 1, Nom: nom, FP: d.Empreinte, Port: d.Port, Lien: i.Lien, Mbits: i.Mbits})
		_ = d.pc.SetMulticastInterface(c) // l'annonce part sur CETTE interface
		_, _ = d.pc.WriteTo(msg, nil, d.groupe)
	}
}

// Demarrer écoute et annonce jusqu'à l'annulation du contexte.
func (d *Decouverte) Demarrer(ctx context.Context) error {
	lc := net.ListenConfig{Control: reutiliser}
	conn, err := lc.ListenPacket(ctx, "udp4", fmt.Sprintf("0.0.0.0:%d", config.PortMulticast))
	if err != nil {
		return err
	}
	d.conn = conn
	d.pc = ipv4.NewPacketConn(conn)
	_ = d.pc.SetMulticastTTL(1) // reste sur le lien local
	_ = d.pc.SetMulticastLoopback(true)
	d.joindre()
	go d.lire()
	go func() {
		t := time.NewTicker(periode)
		defer t.Stop()
		for {
			d.joindre() // nouvelle interface (partage de connexion activé) : on la rejoint
			d.emettre()
			select {
			case <-ctx.Done():
				conn.Close()
				return
			case <-t.C:
			}
		}
	}()
	return nil
}

func (d *Decouverte) lire() {
	tampon := make([]byte, 2048)
	for {
		n, _, src, err := d.pc.ReadFrom(tampon)
		if err != nil {
			return
		}
		u, ok := src.(*net.UDPAddr)
		if !ok || !d.Autorisee(u.IP.String()) {
			continue
		}
		if a := Lire(tampon[:n]); a != nil && a.FP != d.Empreinte {
			d.Enregistrer(a, u.IP.String())
		}
	}
}
