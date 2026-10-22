// Package blocs : découpage, vérification et reprise.
//
// Un fichier de N octets est découpé en n = ceil(N / B) blocs de taille B (le dernier peut être plus
// court). Chaque bloc porte son SHA-256 ; le récepteur note les blocs validés, et après une coupure
// seuls les blocs manquants sont renvoyés. Intégrité globale : racine = SHA-256(h_0 || … || h_{n-1}).
//
// Vitesse : le fichier .part reste ouvert, les blocs s'écrivent en parallèle avec WriteAt, et l'état
// n'est sauvegardé qu'au plus toutes les 0,5 s. Sûreté : à la reprise, chaque bloc noté est relu et
// revérifié, donc un état en avance sur le disque (coupure de courant) ne valide jamais un faux bloc.
package blocs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/securite"
)

const periodeEtat = 500 * time.Millisecond

func NombreBlocs(taille, b int64) int64 {
	if taille == 0 {
		return 0
	}
	return (taille + b - 1) / b
}

func LongueurBloc(i, taille, b int64) int64 { return min(b, taille-i*b) }

// LireEtHacher lit le bloc i d'un fichier et calcule son SHA-256.
func LireEtHacher(f *os.File, i, taille, b int64, tampon []byte) ([]byte, string, error) {
	n := LongueurBloc(i, taille, b)
	tampon = tampon[:n]
	if _, err := f.ReadAt(tampon, i*b); err != nil && !errors.Is(err, io.EOF) {
		return nil, "", err
	}
	s := sha256.Sum256(tampon)
	return tampon, hex.EncodeToString(s[:]), nil
}

// Racine des hachages de blocs.
func Racine(hachages []string) string {
	s := sha256.New()
	for _, x := range hachages {
		b, _ := hex.DecodeString(x)
		s.Write(b)
	}
	return hex.EncodeToString(s.Sum(nil))
}

type etat struct {
	Nom        string            `json:"nom"`
	Taille     int64             `json:"taille"`
	TailleBloc int64             `json:"taille_bloc"`
	Appareil   string            `json:"appareil"`
	N          int64             `json:"n"`
	Recus      map[string]string `json:"recus"`
}

// Reception : état d'une réception en cours.
type Reception struct {
	mu        sync.Mutex
	dossier   string
	ident     string
	e         etat
	f         *os.File
	sauvegard time.Time
}

func (r *Reception) part() string  { return filepath.Join(r.dossier, r.ident+".part") }
func (r *Reception) json_() string { return filepath.Join(r.dossier, r.ident+".json") }

// Ouvrir reprend une réception compatible ou en crée une nouvelle.
func Ouvrir(dossier, ident, nom string, taille, b int64, appareil string) (*Reception, error) {
	if err := os.MkdirAll(dossier, 0o755); err != nil {
		return nil, err
	}
	r := &Reception{dossier: dossier, ident: ident, sauvegard: time.Now()}
	var e etat
	ok, _ := config.LireJSON(r.json_(), &e)
	_, errPart := os.Stat(r.part())
	compatible := ok && errPart == nil && e.Taille == taille && e.TailleBloc == b && e.Appareil == appareil
	if !compatible {
		e = etat{Nom: nom, Taille: taille, TailleBloc: b, Appareil: appareil, N: NombreBlocs(taille, b), Recus: map[string]string{}}
		f, err := os.Create(r.part())
		if err != nil {
			return nil, err
		}
		if err := f.Truncate(taille); err != nil { // réserve la taille (fichier creux sur NTFS et ext4)
			f.Close()
			return nil, err
		}
		r.f = f
	} else {
		f, err := os.OpenFile(r.part(), os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		r.f = f
	}
	r.e = e
	if compatible {
		r.reverifier()
	}
	return r, nil
}

func (r *Reception) reverifier() {
	valides := map[string]string{}
	tampon := make([]byte, r.e.TailleBloc)
	for cle, sha := range r.e.Recus {
		i, err := strconv.ParseInt(cle, 10, 64)
		if err != nil || i < 0 || i >= r.e.N {
			continue
		}
		if _, h, err := LireEtHacher(r.f, i, r.e.Taille, r.e.TailleBloc, tampon); err == nil && h == sha {
			valides[cle] = sha
		}
	}
	r.e.Recus = valides
}

func (r *Reception) Appareil() string { return r.e.Appareil }
func (r *Reception) Nom() string      { return r.e.Nom }

func (r *Reception) Manquants() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var m []int64
	for i := int64(0); i < r.e.N; i++ {
		if _, ok := r.e.Recus[strconv.FormatInt(i, 10)]; !ok {
			m = append(m, i)
		}
	}
	return m
}

// EcrireBloc vérifie puis écrit le bloc i ; renvoie la raison d'un refus, ou "".
// Le hachage et l'écriture se font hors verrou : plusieurs blocs avancent en parallèle.
func (r *Reception) EcrireBloc(i int64, donnees []byte, shaAnnonce string) string {
	if i < 0 || i >= r.e.N {
		return "indice de bloc hors limites"
	}
	if int64(len(donnees)) != LongueurBloc(i, r.e.Taille, r.e.TailleBloc) {
		return "longueur de bloc incorrecte"
	}
	s := sha256.Sum256(donnees)
	sha := hex.EncodeToString(s[:])
	if !securite.EgalTempsConstant(sha, strings.ToLower(shaAnnonce)) {
		return "empreinte du bloc incorrecte"
	}
	if _, err := r.f.WriteAt(donnees, i*r.e.TailleBloc); err != nil {
		return "écriture impossible : " + err.Error()
	}
	r.mu.Lock()
	r.e.Recus[strconv.FormatInt(i, 10)] = sha
	if r.e.N > 1 && time.Since(r.sauvegard) >= periodeEtat {
		_ = config.EcrireJSON(r.json_(), r.e, false)
		r.sauvegard = time.Now()
	}
	r.mu.Unlock()
	return ""
}

// Sauvegarder : à l'arrêt du démon, l'état le plus récent part sur disque.
func (r *Reception) Sauvegarder() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		_ = r.f.Sync()
	}
	_ = config.EcrireJSON(r.json_(), r.e, false)
}

// Finaliser vérifie la racine puis renomme le .part en fichier définitif (sans écraser).
func (r *Reception) Finaliser(dossierReception, racineEmetteur string) (string, string) {
	if m := r.Manquants(); len(m) > 0 {
		return "", strconv.Itoa(len(m)) + " bloc(s) manquant(s)"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	h := make([]string, r.e.N)
	for i := range h {
		h[i] = r.e.Recus[strconv.Itoa(i)]
	}
	if !securite.EgalTempsConstant(Racine(h), strings.ToLower(racineEmetteur)) {
		return "", "empreinte racine différente"
	}
	r.f.Close()
	r.f = nil
	final, err := securite.CheminLibre(dossierReception, r.e.Nom)
	if err != nil {
		return "", err.Error()
	}
	if err := os.Rename(r.part(), final); err != nil {
		return "", err.Error()
	}
	os.Remove(r.json_())
	return final, ""
}

// Abandonner supprime la réception en cours.
func (r *Reception) Abandonner() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
	os.Remove(r.part())
	os.Remove(r.json_())
}

// Fermer libère le fichier sans rien supprimer (tests, arrêt).
func (r *Reception) Fermer() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
}
