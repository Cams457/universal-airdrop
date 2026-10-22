// Package appareils tient le registre des appareils de confiance : PC appairés (CA épinglée)
// et jetons d'accès (téléphones et PC). Seul le hachage des jetons acceptés est stocké.
//
// Limite connue : le jeton sortant (celui que CE PC présente aux autres) est stocké en clair dans
// le profil utilisateur, comme un mot de passe enregistré. Hors du modèle de menace : un logiciel
// malveillant déjà présent sur le PC.
package appareils

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/securite"
)

// PC : un pair appairé.
type PC struct {
	Nom          string `json:"nom"`
	CAPEM        string `json:"ca_pem"`
	Adresse      string `json:"adresse"`
	Port         int    `json:"port"`
	JetonSortant string `json:"jeton_sortant"`
	Ajoute       string `json:"ajoute"`
}

// Acces : un jeton accepté par ce PC.
type Acces struct {
	Nom       string `json:"nom"`
	Type      string `json:"type"` // "pc" ou "telephone"
	Empreinte string `json:"empreinte,omitempty"`
	Cree      string `json:"cree,omitempty"`
}

type donnees struct {
	PCs    map[string]*PC    `json:"pcs"`
	Jetons map[string]*Acces `json:"jetons"`
}

// Registre est sûr pour un usage concurrent ; il se recharge si le fichier change sur disque
// (une révocation faite ailleurs s'applique donc immédiatement au démon).
type Registre struct {
	chemin string
	mu     sync.Mutex
	d      donnees
	mtime  time.Time
}

func Ouvrir(dossier string) *Registre {
	r := &Registre{chemin: filepath.Join(dossier, "appareils.json")}
	r.d = donnees{PCs: map[string]*PC{}, Jetons: map[string]*Acces{}}
	r.mu.Lock()
	r.rafraichir()
	r.mu.Unlock()
	return r
}

func (r *Registre) rafraichir() {
	st, err := os.Stat(r.chemin)
	if err != nil {
		return
	}
	if st.ModTime().Equal(r.mtime) {
		return
	}
	var d donnees
	if ok, err := config.LireJSON(r.chemin, &d); ok && err == nil {
		if d.PCs == nil {
			d.PCs = map[string]*PC{}
		}
		if d.Jetons == nil {
			d.Jetons = map[string]*Acces{}
		}
		r.d = d
		r.mtime = st.ModTime()
	}
}

func (r *Registre) sauver() error {
	if err := config.EcrireJSON(r.chemin, r.d, true); err != nil {
		return err
	}
	if st, err := os.Stat(r.chemin); err == nil {
		r.mtime = st.ModTime()
	}
	return nil
}

func maintenant() string { return time.Now().Format(time.RFC3339) }

// AjouterPC enregistre un pair appairé et le jeton qu'il présentera.
func (r *Registre) AjouterPC(fp, nom, caPEM, adresse string, port int, jetonSortant, jetonEntrant string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rafraichir()
	for h, a := range r.d.Jetons {
		if a.Empreinte == fp {
			delete(r.d.Jetons, h)
		}
	}
	r.d.PCs[fp] = &PC{Nom: nom, CAPEM: caPEM, Adresse: adresse, Port: port, JetonSortant: jetonSortant, Ajoute: maintenant()}
	r.d.Jetons[securite.HacherJeton(jetonEntrant)] = &Acces{Nom: nom, Type: "pc", Empreinte: fp}
	return r.sauver()
}

// MettreAJourAdresse : appelée par la découverte, seulement pour un pair déjà connu.
func (r *Registre) MettreAJourAdresse(fp, adresse string, port int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rafraichir()
	if p, ok := r.d.PCs[fp]; ok && (p.Adresse != adresse || p.Port != port) {
		p.Adresse, p.Port = adresse, port
		_ = r.sauver()
	}
}

// TrouverPC par nom (insensible à la casse), empreinte ou adresse.
func (r *Registre) TrouverPC(cible string) (string, PC, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rafraichir()
	if p, ok := r.d.PCs[cible]; ok {
		return cible, *p, true
	}
	hote := strings.Split(cible, ":")[0]
	for fp, p := range r.d.PCs {
		if strings.EqualFold(p.Nom, cible) || p.Adresse == hote {
			return fp, *p, true
		}
	}
	return "", PC{}, false
}

// AjouterTelephone crée un jeton d'accès pour un téléphone et le renvoie (affiché une seule fois).
func (r *Registre) AjouterTelephone(nom string) (string, error) {
	j := securite.NouveauJeton()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rafraichir()
	r.d.Jetons[securite.HacherJeton(j)] = &Acces{Nom: nom, Type: "telephone", Cree: maintenant()}
	return j, r.sauver()
}

// Verifier renvoie l'appareil associé au jeton. La recherche porte sur le hachage du jeton.
func (r *Registre) Verifier(jeton string) (Acces, bool) {
	if jeton == "" || len(jeton) > 256 {
		return Acces{}, false
	}
	h := securite.HacherJeton(jeton)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rafraichir()
	for cle, a := range r.d.Jetons {
		if securite.EgalTempsConstant(cle, h) {
			return *a, true
		}
	}
	return Acces{}, false
}

// Revoquer retire tous les jetons et appairages portant ce nom.
func (r *Registre) Revoquer(nom string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rafraichir()
	n := 0
	for h, a := range r.d.Jetons {
		if strings.EqualFold(a.Nom, nom) {
			delete(r.d.Jetons, h)
			n++
		}
	}
	for fp, p := range r.d.PCs {
		if strings.EqualFold(p.Nom, nom) {
			delete(r.d.PCs, fp)
			n++
		}
	}
	return n, r.sauver()
}

// Ligne : vue d'un appareil pour l'affichage.
type Ligne struct {
	Nom       string `json:"nom"`
	Type      string `json:"type"`
	Adresse   string `json:"adresse,omitempty"`
	Empreinte string `json:"empreinte,omitempty"`
}

func (r *Registre) Liste() []Ligne {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rafraichir()
	var l []Ligne
	for fp, p := range r.d.PCs {
		l = append(l, Ligne{Nom: p.Nom, Type: "pc", Adresse: p.Adresse, Empreinte: fp})
	}
	for _, a := range r.d.Jetons {
		if a.Type == "telephone" {
			l = append(l, Ligne{Nom: a.Nom, Type: "telephone"})
		}
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Type+l[i].Nom < l[j].Type+l[j].Nom })
	return l
}
