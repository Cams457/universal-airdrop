// Package boite : boîte d'envoi d'un téléphone. Un iPhone ne peut pas recevoir de lui-même (pas de
// serveur possible sans application) : le PC garde donc ce qui lui est destiné (fichiers, liens,
// textes) jusqu'à ce que le Raccourci « Recevoir » vienne le chercher.
//
// La boîte est identifiée par le NOM du téléphone (sans tenir compte de la casse) : un iPhone ajouté
// deux fois sous le même nom garde une seule boîte. Les éléments expirent au bout de 7 jours.
package boite

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/securite"
)

// Duree de conservation d'un élément non récupéré.
const Duree = 7 * 24 * time.Hour

// Element : un fichier, un lien ou un texte en attente.
type Element struct {
	ID     string    `json:"id"`
	Type   string    `json:"type"` // fichier, lien, texte
	Nom    string    `json:"nom,omitempty"`
	Taille int64     `json:"taille,omitempty"`
	Texte  string    `json:"texte,omitempty"`
	De     string    `json:"de"`
	Cree   time.Time `json:"cree"`
	chemin string
}

// Boites : toutes les boîtes de ce PC. Sûr pour un usage concurrent ; l'index est sauvegardé sur
// disque à chaque changement (les éléments survivent à un redémarrage).
type Boites struct {
	dossier string
	mu      sync.Mutex
	b       map[string][]*Element // clé : nom du téléphone en minuscules
}

type sauvegarde struct {
	Elements map[string][]*Element `json:"elements"`
	Chemins  map[string]string     `json:"chemins"`
}

// Ouvrir charge les boîtes depuis dossier/boites.
func Ouvrir(dossier string) *Boites {
	b := &Boites{dossier: filepath.Join(dossier, "boites"), b: map[string][]*Element{}}
	var s sauvegarde
	if ok, _ := config.LireJSON(filepath.Join(b.dossier, "index.json"), &s); ok {
		for cle, l := range s.Elements {
			for _, e := range l {
				e.chemin = s.Chemins[e.ID]
				if e.Type == "fichier" {
					if _, err := os.Stat(e.chemin); err != nil {
						continue // fichier disparu : on oublie l'élément
					}
				}
				b.b[cle] = append(b.b[cle], e)
			}
		}
	}
	return b
}

func cle(telephone string) string { return strings.ToLower(strings.TrimSpace(telephone)) }

func (b *Boites) sousDossier(telephone string) string {
	h := sha256.Sum256([]byte(cle(telephone)))
	return filepath.Join(b.dossier, hex.EncodeToString(h[:8]))
}

func (b *Boites) sauver() {
	s := sauvegarde{Elements: b.b, Chemins: map[string]string{}}
	for _, l := range b.b {
		for _, e := range l {
			if e.chemin != "" {
				s.Chemins[e.ID] = e.chemin
			}
		}
	}
	_ = os.MkdirAll(b.dossier, 0o700)
	_ = config.EcrireJSON(filepath.Join(b.dossier, "index.json"), s, true)
}

// DeposerFichier copie (ou déplace, si deplacer) un fichier dans la boîte du téléphone.
func (b *Boites) DeposerFichier(telephone, de, source string, deplacer bool) (*Element, error) {
	st, err := os.Stat(source)
	if err != nil || st.IsDir() {
		return nil, errors.New("fichier introuvable")
	}
	dossier := b.sousDossier(telephone)
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		return nil, err
	}
	e := &Element{ID: securite.JetonCourt(), Type: "fichier", Nom: securite.NomSur(filepath.Base(source)),
		Taille: st.Size(), De: de, Cree: time.Now()}
	e.chemin = filepath.Join(dossier, e.ID+"-"+e.Nom)
	if deplacer && os.Rename(source, e.chemin) == nil {
		// déplacé sans copie
	} else if err := copier(source, e.chemin); err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.b[cle(telephone)] = append(b.b[cle(telephone)], e)
	b.sauver()
	b.mu.Unlock()
	return e, nil
}

func copier(src, dst string) error {
	if os.Link(src, dst) == nil { // même disque : instantané, sans place supplémentaire
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(out, in, make([]byte, 1<<20)); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// DeposerTexte dépose un lien ou un texte.
func (b *Boites) DeposerTexte(telephone, de, texte string, lien bool) *Element {
	t := "texte"
	if lien {
		t = "lien"
	}
	e := &Element{ID: securite.JetonCourt(), Type: t, Texte: texte, De: de, Cree: time.Now()}
	b.mu.Lock()
	b.b[cle(telephone)] = append(b.b[cle(telephone)], e)
	b.sauver()
	b.mu.Unlock()
	return e
}

// Lister renvoie les éléments en attente, du plus ancien au plus récent.
func (b *Boites) Lister(telephone string) []Element {
	b.mu.Lock()
	defer b.mu.Unlock()
	var l []Element
	for _, e := range b.b[cle(telephone)] {
		l = append(l, *e)
	}
	sort.SliceStable(l, func(i, j int) bool { return l[i].Cree.Before(l[j].Cree) })
	return l
}

// Nombre d'éléments en attente.
func (b *Boites) Nombre(telephone string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.b[cle(telephone)])
}

// Fichier renvoie le chemin d'un fichier en attente (pour le servir).
func (b *Boites) Fichier(telephone, id string) (Element, string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.b[cle(telephone)] {
		if e.ID == id && e.Type == "fichier" {
			return *e, e.chemin, true
		}
	}
	return Element{}, "", false
}

// Retirer supprime un élément (récupéré) et son fichier.
func (b *Boites) Retirer(telephone, id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	l := b.b[cle(telephone)]
	for i, e := range l {
		if e.ID == id {
			if e.chemin != "" {
				os.Remove(e.chemin)
			}
			b.b[cle(telephone)] = append(l[:i], l[i+1:]...)
			break
		}
	}
	if len(b.b[cle(telephone)]) == 0 {
		delete(b.b, cle(telephone))
	}
	b.sauver()
}

// Vider supprime toute la boîte (téléphone retiré).
func (b *Boites) Vider(telephone string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.b, cle(telephone))
	os.RemoveAll(b.sousDossier(telephone))
	b.sauver()
}

// Nettoyer retire les éléments plus vieux que Duree.
func (b *Boites) Nettoyer() {
	b.mu.Lock()
	defer b.mu.Unlock()
	change := false
	for k, l := range b.b {
		var garde []*Element
		for _, e := range l {
			if time.Since(e.Cree) > Duree {
				if e.chemin != "" {
					os.Remove(e.chemin)
				}
				change = true
				continue
			}
			garde = append(garde, e)
		}
		if len(garde) == 0 {
			delete(b.b, k)
		} else {
			b.b[k] = garde
		}
	}
	if change {
		b.sauver()
	}
}
