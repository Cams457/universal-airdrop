// Package partage gère les liens à jeton affichés en QR code :
//   - partage PC -> téléphone (fichiers, lien, ou texte à usage unique) ;
//   - page d'envoi téléphone -> PC, utilisable depuis le navigateur de n'importe quel téléphone.
//
// Les jetons font 128 bits : deviner une URL valide a une probabilité de 2^-128 par essai.
// L'URL ne contient que le jeton, jamais de nom de fichier.
package partage

import (
	"sync"
	"time"

	"github.com/Cams457/universal-airdrop/internal/securite"
)

// Type de partage.
type Type int

const (
	Fichiers Type = iota // PC -> téléphone
	Texte                // texte affiché une seule fois (jeton d'accès)
	Envoi                // page téléphone -> PC
)

type Partage struct {
	Type        Type
	Fichiers    []string
	URL         string
	Texte       string
	Expire      time.Time
	Telecharges map[int]bool
	Temporaires bool // fichiers copiés exprès pour le partage : à supprimer ensuite
}

func (p *Partage) Complet() bool {
	return len(p.Fichiers) > 0 && len(p.Telecharges) == len(p.Fichiers)
}

type Registre struct {
	mu sync.Mutex
	p  map[string]*Partage
	// SurFin est appelée quand un partage de fichiers temporaires disparaît.
	SurFin func(*Partage)
}

func NouveauRegistre() *Registre { return &Registre{p: map[string]*Partage{}} }

func (r *Registre) Creer(p *Partage, duree time.Duration) string {
	j := securite.JetonCourt()
	p.Expire = time.Now().Add(duree)
	p.Telecharges = map[int]bool{}
	r.mu.Lock()
	r.p[j] = p
	r.mu.Unlock()
	return j
}

func (r *Registre) Obtenir(jeton string, t Type) *Partage {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.p[jeton]
	if !ok || p.Type != t {
		return nil
	}
	if time.Now().After(p.Expire) {
		r.retirer(jeton)
		return nil
	}
	return p
}

// Consommer retire un partage (texte à usage unique).
func (r *Registre) Consommer(jeton string) {
	r.mu.Lock()
	r.retirer(jeton)
	r.mu.Unlock()
}

// Marquer : après le téléchargement complet de chaque fichier, le partage disparaît (usage unique).
func (r *Registre) Marquer(jeton string, i int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.p[jeton]; ok {
		p.Telecharges[i] = true
		if p.Complet() && p.URL == "" {
			r.retirer(jeton)
		}
	}
}

func (r *Registre) retirer(jeton string) {
	if p, ok := r.p[jeton]; ok {
		delete(r.p, jeton)
		if r.SurFin != nil {
			go r.SurFin(p)
		}
	}
}

func (r *Registre) Nettoyer() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for j, p := range r.p {
		if time.Now().After(p.Expire) {
			r.retirer(j)
		}
	}
}

// Premier renvoie un jeton existant du type donné (tests).
func (r *Registre) Premier(t Type) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for j, p := range r.p {
		if p.Type == t {
			return j
		}
	}
	return ""
}
