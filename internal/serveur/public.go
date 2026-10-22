package serveur

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Cams457/universal-airdrop/internal/appareils"
	"github.com/Cams457/universal-airdrop/internal/blocs"
	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/identite"
	"github.com/Cams457/universal-airdrop/internal/partage"
	"github.com/Cams457/universal-airdrop/internal/profilios"
	"github.com/Cams457/universal-airdrop/internal/securite"
	"github.com/Cams457/universal-airdrop/internal/ui"
)

const (
	tailleBlocMin  = 64 << 10
	tailleBlocMax  = 16 << 20
	tailleMax      = 1 << 42 // 4 Tio
	echecsMax      = 20
	fenetreEchecs  = 10 * time.Minute
	tentativesAppB = 3
	limiteJSON     = 256 << 10
)

func ecrireJSON(w http.ResponseWriter, statut int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statut)
	_ = json.NewEncoder(w).Encode(v)
}

func erreur(w http.ResponseWriter, statut int, message string) {
	ecrireJSON(w, statut, map[string]string{"erreur": message})
}

func lireJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limiteJSON)).Decode(v); err != nil {
		erreur(w, http.StatusBadRequest, "requête invalide")
		return false
	}
	return true
}

func ipDe(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// filtreReseau : seules les adresses du réseau local sont acceptées ; en-têtes de sécurité.
func (s *Service) filtreReseau(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !securite.IPAutorisee(ipDe(r)) {
			erreur(w, http.StatusForbidden, "seules les adresses du réseau local sont acceptées")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (s *Service) routesPubliques() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/info", s.info)
	m.HandleFunc("POST /api/v1/appairage/debut", s.appairageDebut)
	m.HandleFunc("POST /api/v1/appairage/revele", s.appairageRevele)
	m.HandleFunc("POST /api/v1/appairage/confirmer", s.appairageConfirmer)
	m.HandleFunc("POST /api/v1/transfert/preparer", s.transfertPreparer)
	m.HandleFunc("PUT /api/v1/transfert/{ident}/bloc/{i}", s.transfertBloc)
	m.HandleFunc("POST /api/v1/transfert/{ident}/terminer", s.transfertTerminer)
	m.HandleFunc("DELETE /api/v1/transfert/{ident}", s.transfertAbandonner)
	m.HandleFunc("POST /api/rapide/fichier", s.rapideFichier)
	m.HandleFunc("POST /api/rapide/lien", s.rapideLien)
	m.HandleFunc("GET /api/rapide/destinations", s.rDestinations)
	m.HandleFunc("GET /api/rapide/boite", s.rBoite)
	m.HandleFunc("GET /api/rapide/boite/{id}", s.rBoiteFichier)
	m.HandleFunc("GET /api/v1/telephones", s.rTelephones)
	m.HandleFunc("POST /api/v1/telephone/demande", s.rDemander)
	m.HandleFunc("GET /api/v1/telephone/demande/{id}", s.rSuivreDemande)
	m.HandleFunc("GET /iphone", s.pageIphone)
	m.HandleFunc("GET /profil.mobileconfig", s.profilIOS)
	m.HandleFunc("GET /p/{jeton}", s.pagePartage)
	m.HandleFunc("GET /p/{jeton}/{i}", s.telechargerPartage)
	m.HandleFunc("GET /envoyer/{jeton}", s.pageEnvoi)
	m.HandleFunc("POST /envoyer/{jeton}", s.recevoirEnvoi)
	m.Handle("GET /statique/", http.StripPrefix("/statique/", ui.Public()))
	return s.filtreReseau(m)
}

// authentifier vérifie le jeton (en-tête Authorization: Bearer, ou X-Jeton) et limite les essais.
func (s *Service) authentifier(w http.ResponseWriter, r *http.Request) (appareils.Acces, string, bool) {
	ip := ipDe(r)
	s.mu.Lock()
	var recents []time.Time
	for _, t := range s.echecs[ip] {
		if time.Since(t) < fenetreEchecs {
			recents = append(recents, t)
		}
	}
	s.echecs[ip] = recents
	s.mu.Unlock()
	if len(recents) >= echecsMax {
		erreur(w, http.StatusTooManyRequests, "trop d'échecs d'authentification, réessaie dans 10 minutes")
		return appareils.Acces{}, "", false
	}
	jeton := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if jeton == "" || jeton == r.Header.Get("Authorization") {
		jeton = strings.TrimSpace(r.Header.Get("X-Jeton"))
	}
	a, ok := s.Reg.Verifier(jeton)
	if !ok {
		s.mu.Lock()
		s.echecs[ip] = append(s.echecs[ip], time.Now())
		s.mu.Unlock()
		erreur(w, http.StatusUnauthorized, "jeton absent ou invalide")
		return appareils.Acces{}, "", false
	}
	return a, a.Type + ":" + a.Nom + ":" + a.Empreinte, true
}

func (s *Service) info(w http.ResponseWriter, r *http.Request) {
	ecrireJSON(w, 200, map[string]any{"app": "universal-airdrop", "version": Version, "nom": s.Cfg.Nom, "empreinte": s.ID.Empreinte})
}

// ---------- transfert PC -> PC par blocs ----------

func (s *Service) transfertPreparer(w http.ResponseWriter, r *http.Request) {
	a, cle, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	var c struct {
		Nom        string `json:"nom"`
		Taille     int64  `json:"taille"`
		TailleBloc int64  `json:"taille_bloc"`
		Cle        string `json:"cle"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	if c.Taille < 0 || c.Taille > tailleMax || c.TailleBloc < tailleBlocMin || c.TailleBloc > tailleBlocMax {
		erreur(w, http.StatusBadRequest, "taille ou taille de bloc hors limites")
		return
	}
	if libre, err := espaceLibre(s.Cfg.DossierReception()); err == nil && libre < uint64(c.Taille) {
		erreur(w, http.StatusInsufficientStorage, "espace disque insuffisant")
		return
	}
	if len(c.Cle) > 128 {
		c.Cle = c.Cle[:128]
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%d", cle, c.Cle, c.Taille, c.TailleBloc)))
	ident := hex.EncodeToString(h[:16])
	s.mu.Lock()
	rec := s.receptions[ident] // l'état en mémoire fait foi ; le disque sert après un redémarrage
	s.mu.Unlock()
	if rec == nil {
		var err error
		rec, err = blocs.Ouvrir(s.tmp(), ident, securite.NomSur(c.Nom), c.Taille, c.TailleBloc, cle)
		if err != nil {
			erreur(w, http.StatusInternalServerError, "impossible de préparer la réception")
			return
		}
		s.mu.Lock()
		s.receptions[ident] = rec
		s.mu.Unlock()
	}
	_ = a
	m := rec.Manquants()
	if m == nil {
		m = []int64{}
	}
	ecrireJSON(w, 200, map[string]any{"id": ident, "manquants": m})
}

func (s *Service) reception(w http.ResponseWriter, r *http.Request, cle string) *blocs.Reception {
	s.mu.Lock()
	rec := s.receptions[r.PathValue("ident")]
	s.mu.Unlock()
	if rec == nil {
		erreur(w, http.StatusNotFound, "transfert inconnu : rappeler preparer")
		return nil
	}
	if rec.Appareil() != cle {
		erreur(w, http.StatusForbidden, "transfert d'un autre appareil")
		return nil
	}
	return rec
}

func (s *Service) transfertBloc(w http.ResponseWriter, r *http.Request) {
	_, cle, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	rec := s.reception(w, r, cle)
	if rec == nil {
		return
	}
	i, err := strconv.ParseInt(r.PathValue("i"), 10, 64)
	if err != nil {
		erreur(w, http.StatusBadRequest, "indice invalide")
		return
	}
	// Lecture dans un tampon pré-alloué et recyclé : ni agrandissements successifs, ni copies.
	if r.ContentLength <= 0 || r.ContentLength > tailleBlocMax {
		erreur(w, http.StatusBadRequest, "longueur de bloc absente ou trop grande")
		return
	}
	tampon := prendreTampon(int(r.ContentLength))
	defer rendreTampon(tampon)
	donnees := tampon[:r.ContentLength]
	if _, err := io.ReadFull(r.Body, donnees); err != nil {
		erreur(w, http.StatusBadRequest, "bloc incomplet")
		return
	}
	if raison := rec.EcrireBloc(i, donnees, r.Header.Get("X-Bloc-SHA256")); raison != "" {
		erreur(w, http.StatusUnprocessableEntity, raison)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// transfertAbandonner : l'expéditeur a annulé ; on efface le fichier partiel et son état.
func (s *Service) transfertAbandonner(w http.ResponseWriter, r *http.Request) {
	a, cle, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	rec := s.reception(w, r, cle)
	if rec == nil {
		return
	}
	s.mu.Lock()
	delete(s.receptions, r.PathValue("ident"))
	s.mu.Unlock()
	rec.Abandonner()
	s.Evenement("info", fmt.Sprintf("« %s » a annulé son envoi", a.Nom), "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) transfertTerminer(w http.ResponseWriter, r *http.Request) {
	a, cle, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	rec := s.reception(w, r, cle)
	if rec == nil {
		return
	}
	var c struct {
		Racine string `json:"racine"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	auteur, pour := origine(r, a)
	var tels []string
	for _, n := range pour {
		if t, ok := s.Reg.TrouverTelephone(n); ok {
			tels = append(tels, t)
		}
	}
	dossier := s.Cfg.DossierReception()
	if len(tels) > 0 { // relais pour nos téléphones : le fichier ne fait que passer par ce PC
		var err error
		if dossier, err = s.dossierRelais(); err != nil {
			erreur(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	final, raison := rec.Finaliser(dossier, c.Racine)
	if raison != "" {
		erreur(w, http.StatusUnprocessableEntity, raison)
		return
	}
	s.mu.Lock()
	delete(s.receptions, r.PathValue("ident"))
	s.mu.Unlock()
	st, _ := os.Stat(final)
	if len(tels) > 0 {
		s.deposer(tels, auteur, []string{final})
		os.RemoveAll(dossier)
	} else {
		s.recu(auteur, final, st.Size())
	}
	ecrireJSON(w, 200, map[string]any{"nom": filepath.Base(final), "taille": st.Size()})
}

var tampons = sync.Pool{New: func() any { b := make([]byte, 0, 4<<20); return &b }}

func prendreTampon(n int) []byte {
	b := *(tampons.Get().(*[]byte))
	if cap(b) < n {
		b = make([]byte, n)
	}
	return b[:n]
}

func rendreTampon(b []byte) {
	if cap(b) <= tailleBlocMax {
		b = b[:0]
		tampons.Put(&b)
	}
}

// ---------- téléphones : API rapide (Raccourci iOS) et page d'envoi ----------

// ecrireFlux écrit un flux reçu directement sur disque, sans tout garder en mémoire.
func (s *Service) ecrireFlux(src io.Reader, nom, auteur, dossier string, annoncer bool) (string, error) {
	if err := os.MkdirAll(s.tmp(), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.tmp(), "telephone-*.part")
	if err != nil {
		return "", err
	}
	_, errCopie := io.CopyBuffer(tmp, src, make([]byte, 1<<20))
	errFerm := tmp.Close()
	if errCopie != nil || errFerm != nil {
		os.Remove(tmp.Name())
		if errCopie != nil {
			return "", errCopie
		}
		return "", errFerm
	}
	final, err := securite.CheminLibre(dossier, securite.NomSur(nom))
	if err == nil {
		err = os.Rename(tmp.Name(), final)
		if err != nil { // autre disque (dossier temporaire) : copie
			err = copierFichier(tmp.Name(), final)
		}
	}
	os.Remove(tmp.Name())
	if err != nil {
		return "", err
	}
	if annoncer {
		st, _ := os.Stat(final)
		s.recu(auteur, final, st.Size())
	}
	return final, nil
}

func nomParDefaut() string { return "fichier-" + time.Now().Format("20060102-150405") }

// recevoirFichiers : multipart (plusieurs fichiers) ou corps brut avec l'en-tête X-Nom, écrits dans
// dossier. En cas d'erreur, la réponse est déjà écrite et ok vaut false.
func (s *Service) recevoirFichiers(w http.ResponseWriter, r *http.Request, auteur, dossier string, annoncer bool) ([]string, bool) {
	if r.ContentLength > 0 {
		if libre, err := espaceLibre(s.Cfg.DossierReception()); err == nil && libre < uint64(r.ContentLength) {
			erreur(w, http.StatusInsufficientStorage, "espace disque insuffisant")
			return nil, false
		}
	}
	var recus []string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		lecteur, err := r.MultipartReader()
		if err != nil {
			erreur(w, http.StatusBadRequest, "formulaire invalide")
			return nil, false
		}
		for {
			p, err := lecteur.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				erreur(w, http.StatusBadRequest, "formulaire interrompu")
				return nil, false
			}
			if p.FileName() == "" {
				p.Close()
				continue
			}
			final, err := s.ecrireFlux(p, nomPartie(p), auteur, dossier, annoncer)
			p.Close()
			if err != nil {
				erreur(w, http.StatusInternalServerError, "écriture impossible")
				return nil, false
			}
			recus = append(recus, final)
		}
	} else {
		nom, _ := url.QueryUnescape(r.Header.Get("X-Nom"))
		if nom == "" {
			nom = nomParDefaut()
		}
		final, err := s.ecrireFlux(r.Body, nom, auteur, dossier, annoncer)
		if err != nil {
			erreur(w, http.StatusBadRequest, "envoi interrompu")
			return nil, false
		}
		recus = append(recus, final)
	}
	if len(recus) == 0 {
		erreur(w, http.StatusBadRequest, "aucun fichier dans la requête")
		return nil, false
	}
	return recus, true
}

func noms(chemins []string) []string {
	l := make([]string, len(chemins))
	for i, c := range chemins {
		l[i] = filepath.Base(c)
	}
	return l
}

// recevoirFormulaire : réception simple sur ce PC (page d'envoi par QR code).
func (s *Service) recevoirFormulaire(w http.ResponseWriter, r *http.Request, auteur string) {
	if recus, ok := s.recevoirFichiers(w, r, auteur, s.Cfg.DossierReception(), true); ok {
		ecrireJSON(w, 200, map[string]any{"recus": noms(recus)})
	}
}

func copierFichier(src, dst string) error {
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

func nomPartie(p *multipart.Part) string {
	if n := p.FileName(); n != "" {
		return n
	}
	return nomParDefaut()
}

// rapideFichier : fichiers envoyés par le Raccourci (ou relayés par un PC pour nos téléphones).
// En-tête facultatif X-Destinations : « PC de Camil|PC portable|iPad » ; sans lui, ce PC seul.
func (s *Service) rapideFichier(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	auteur, pour := origine(r, a)
	var p plan
	if len(pour) > 0 { // relais d'un PC appairé vers nos téléphones
		for _, n := range pour {
			if t, ok := s.Reg.TrouverTelephone(n); ok {
				p.Tels = append(p.Tels, t)
			}
		}
	} else if noms := decoderListe(r.Header.Get("X-Destinations")); len(noms) > 0 {
		p = s.resoudre(noms, a.Nom)
	} else {
		p.Moi = true
	}
	if p.vide() {
		erreur(w, http.StatusBadRequest, "aucun destinataire connu de ce PC")
		return
	}
	dossier := s.Cfg.DossierReception()
	if !p.Moi {
		var err error
		if dossier, err = s.dossierRelais(); err != nil {
			erreur(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	recus, ok := s.recevoirFichiers(w, r, auteur, dossier, p.Moi)
	if !ok {
		if !p.Moi {
			os.RemoveAll(dossier)
		}
		return
	}
	s.distribuer(recus, p, auteur, !p.Moi)
	ecrireJSON(w, 200, map[string]any{"recus": noms(recus), "relais": p.PCs, "en_attente_pour": p.Tels, "inconnus": p.Inconnus})
}

// TexteMax : taille maximale d'un lien ou d'un texte envoyé (le reste passe par un fichier).
const TexteMax = 64 << 10

// texteAcceptable : texte non vide, UTF-8 valide, sans caractère de contrôle hors tabulation et
// retour à la ligne (évite d'injecter des séquences d'échappement dans un terminal où on le colle).
func texteAcceptable(t string) bool {
	if t == "" || len(t) > TexteMax || !utf8.ValidString(t) {
		return false
	}
	for _, c := range t {
		if c < 0x20 && c != '\n' && c != '\t' && c != '\r' || c == 0x7f {
			return false
		}
	}
	return true
}

// rapideLien reçoit un lien ou un texte : il est copié dans le presse-papiers (Ctrl+V) et,
// pour un lien http(s), ouvert dans le navigateur si le réglage est actif.
func (s *Service) rapideLien(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	corps, err := io.ReadAll(http.MaxBytesReader(w, r.Body, TexteMax+1024))
	if err != nil {
		erreur(w, http.StatusRequestEntityTooLarge, "texte trop long : envoie-le plutôt comme fichier")
		return
	}
	texte := ""
	ct := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, "application/json"):
		var c struct {
			URL   string `json:"url"`
			Texte string `json:"texte"`
		}
		_ = json.Unmarshal(corps, &c)
		texte = c.URL + c.Texte
	case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
		v, _ := url.ParseQuery(string(corps))
		texte = v.Get("url") + v.Get("texte")
	default:
		texte = string(corps)
	}
	texte = strings.TrimSpace(texte)
	if !texteAcceptable(texte) {
		erreur(w, http.StatusBadRequest, "texte vide, trop long ou contenant des caractères de contrôle")
		return
	}
	auteur, pour := origine(r, a)
	var p plan
	if len(pour) > 0 {
		for _, n := range pour {
			if t, ok := s.Reg.TrouverTelephone(n); ok {
				p.Tels = append(p.Tels, t)
			}
		}
	} else if noms := decoderListe(r.Header.Get("X-Destinations")); len(noms) > 0 {
		p = s.resoudre(noms, a.Nom)
	} else {
		p.Moi = true
	}
	if p.vide() {
		erreur(w, http.StatusBadRequest, "aucun destinataire connu de ce PC")
		return
	}
	echecs := map[string]string{}
	if len(p.PCs)+len(p.Tels) > 0 {
		ctx, fin := context.WithTimeout(r.Context(), 20*time.Second)
		echecs = s.distribuerTexte(ctx, texte, plan{PCs: p.PCs, Tels: p.Tels}, auteur)
		fin()
	}
	if !p.Moi {
		ecrireJSON(w, 200, map[string]any{"relais": p.PCs, "en_attente_pour": p.Tels, "echecs": echecs})
		return
	}
	estLien := securite.URLAutorisee(texte)
	if f, err := os.OpenFile(filepath.Join(s.Cfg.Donnees, "liens.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		fmt.Fprintf(f, "%s  %s  %s\n", time.Now().Format(time.RFC3339), auteur, strings.ReplaceAll(texte, "\n", " "))
		f.Close()
	}
	copie, raison := false, "presse-papiers non pris en charge sur ce système"
	if s.CopierTexte != nil {
		if err := s.CopierTexte(texte); err == nil {
			copie = true
		} else {
			raison = err.Error()
		}
	}
	ouvert := false
	if estLien && s.Cfg.OuvrirLiens && s.OuvrirLien != nil {
		ouvert = s.OuvrirLien(texte) == nil
	}
	quoi, apercu := "Texte", texte
	if estLien {
		quoi = "Lien"
	}
	if len([]rune(apercu)) > 140 {
		apercu = string([]rune(apercu)[:140]) + "…"
	}
	corpsNotif := apercu
	if copie {
		corpsNotif = "Copié : Ctrl+V pour le coller.\n" + apercu
	}
	suffixe := " (NON copié : " + raison + ")"
	if copie {
		suffixe = " (copié dans le presse-papiers)"
	}
	s.EvenementNotifie("lien", fmt.Sprintf("%s reçu de « %s »%s : %s", quoi, auteur, suffixe, apercu), "",
		fmt.Sprintf("%s reçu de %s", quoi, auteur), corpsNotif)
	ecrireJSON(w, 200, map[string]any{"ouvert": ouvert, "copie": copie, "lien": estLien, "relais": p.PCs, "en_attente_pour": p.Tels, "echecs": echecs})
}

// pageEnvoi : page d'envoi téléphone -> PC, pour n'importe quel téléphone (Android compris).
func (s *Service) pageEnvoi(w http.ResponseWriter, r *http.Request) {
	if s.Partages.Obtenir(r.PathValue("jeton"), partage.Envoi) == nil {
		pageExpiree(w)
		return
	}
	ui.PageEnvoi(w, s.Cfg.Nom)
}

func (s *Service) recevoirEnvoi(w http.ResponseWriter, r *http.Request) {
	if s.Partages.Obtenir(r.PathValue("jeton"), partage.Envoi) == nil {
		erreur(w, http.StatusNotFound, "lien expiré : rescanne le QR code sur le PC")
		return
	}
	s.recevoirFormulaire(w, r, "téléphone")
}

// profilIOS sert le profil iOS (la CA publique du PC). Il n'a rien de secret : il reste
// téléchargeable sans jeton ni limite d'usage, car Safari le récupère parfois deux fois
// (avant et après « Autoriser »). La sécurité repose sur la vérification de l'empreinte.
func (s *Service) profilIOS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Write(profilios.Generer(s.ID.DERCA(), s.Cfg.Nom, s.ID.Empreinte))
	if r.Method == http.MethodGet {
		s.Evenement("info", "Profil iOS téléchargé par "+ipDe(r), "")
	}
}

// pageIphone : guide d'installation du certificat, ouvert en scannant le QR code de l'assistant.
func (s *Service) pageIphone(w http.ResponseWriter, r *http.Request) {
	ui.PageIphone(w, s.Cfg.Nom, identite.Lisible(s.ID.Empreinte))
}

// ---------- partage PC -> téléphone ----------

func pageExpiree(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	ui.PageMessage(w, "Lien expiré", "Ce lien a expiré ou a déjà été utilisé. Génère un nouveau QR code sur le PC.")
}

func (s *Service) pagePartage(w http.ResponseWriter, r *http.Request) {
	j := r.PathValue("jeton")
	if p := s.Partages.Obtenir(j, partage.Texte); p != nil {
		s.Partages.Consommer(j) // un jeton d'accès ne s'affiche qu'une fois
		ui.PageTexte(w, s.Cfg.Nom, p.Texte)
		return
	}
	p := s.Partages.Obtenir(j, partage.Fichiers)
	if p == nil {
		pageExpiree(w)
		return
	}
	var fichiers []ui.FichierPartage
	for i, f := range p.Fichiers {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		fichiers = append(fichiers, ui.FichierPartage{Nom: filepath.Base(f), Taille: TailleLisible(st.Size()),
			URL: fmt.Sprintf("/p/%s/%d", j, i)})
	}
	ui.PagePartage(w, s.Cfg.Nom, p.URL, fichiers)
}

func (s *Service) telechargerPartage(w http.ResponseWriter, r *http.Request) {
	j := r.PathValue("jeton")
	p := s.Partages.Obtenir(j, partage.Fichiers)
	i, err := strconv.Atoi(r.PathValue("i"))
	if p == nil || err != nil || i < 0 || i >= len(p.Fichiers) {
		erreur(w, http.StatusNotFound, "lien expiré ou fichier introuvable")
		return
	}
	f, err := os.Open(p.Fichiers[i])
	if err != nil {
		erreur(w, http.StatusNotFound, "fichier introuvable")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	nom := filepath.Base(p.Fichiers[i])
	if strings.HasSuffix(strings.ToLower(nom), ".mobileconfig") {
		w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(nom))
	}
	http.ServeContent(w, r, nom, st.ModTime(), f) // gère les reprises (Range) des navigateurs
	if r.Header.Get("Range") == "" {
		s.Partages.Marquer(j, i)
		s.Evenement("envoye", fmt.Sprintf("Téléchargé par %s : %s", ipDe(r), nom), "")
	}
}

// ---------- appairage côté B (celui qui a ouvert la fenêtre) ----------

type appairageB struct {
	Expire     time.Time
	Etat       string // attente, engage, a_confirmer, termine, refuse, expire
	Tentatives int
	Session    string
	Code       string
	Pair       struct {
		FP, CAPEM, Nom, IP string
		Port               int
		AleaB              []byte
	}
	decision chan bool
}

func (a *appairageB) ouverte() bool {
	return time.Now().Before(a.Expire) && a.Etat != "termine" && a.Etat != "refuse" && a.Etat != "expire"
}

func (s *Service) OuvrirAppairage() {
	s.mu.Lock()
	s.appairageB = &appairageB{Expire: time.Now().Add(config.DureeAppairageS * time.Second), Etat: "attente", decision: make(chan bool, 1)}
	s.mu.Unlock()
	s.Evenement("info", "Appairage : fenêtre ouverte pour 2 minutes", "")
}

func (s *Service) appairageDebut(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nom   string `json:"nom"`
		CAPEM string `json:"ca_pem"`
		Port  int    `json:"port"`
	}
	s.mu.Lock()
	a := s.appairageB
	if a == nil || !a.ouverte() || a.Etat != "attente" {
		s.mu.Unlock()
		erreur(w, http.StatusForbidden, "aucune fenêtre d'appairage ouverte sur ce PC")
		return
	}
	if a.Tentatives >= tentativesAppB {
		s.mu.Unlock()
		erreur(w, http.StatusTooManyRequests, "trop de tentatives d'appairage")
		return
	}
	a.Tentatives++
	s.mu.Unlock()
	if !lireJSON(w, r, &c) {
		return
	}
	fpA, _, err := identite.EmpreintePEM(c.CAPEM)
	if err != nil || c.Port <= 0 || c.Port > 65535 {
		erreur(w, http.StatusBadRequest, "demande d'appairage invalide")
		return
	}
	if fpA == s.ID.Empreinte {
		erreur(w, http.StatusBadRequest, "un PC ne s'appaire pas avec lui-même")
		return
	}
	if len([]rune(c.Nom)) > 64 {
		c.Nom = string([]rune(c.Nom)[:64])
	}
	aleaB := securite.Alea(32)
	s.mu.Lock()
	a.Session = securite.JetonCourt()
	a.Pair.FP, a.Pair.CAPEM, a.Pair.Nom, a.Pair.IP, a.Pair.Port, a.Pair.AleaB = fpA, c.CAPEM, c.Nom, ipDe(r), c.Port, aleaB
	a.Etat = "engage"
	session := a.Session
	s.mu.Unlock()
	s.Evenement("info", fmt.Sprintf("Appairage : demande de « %s » depuis %s", c.Nom, ipDe(r)), "")
	ecrireJSON(w, 200, map[string]any{"session": session, "nom": s.Cfg.Nom, "ca_pem": s.ID.CAPEM, "port": s.Cfg.Port,
		"engagement": securite.Engagement(s.ID.Empreinte, fpA, aleaB)})
}

func (s *Service) sessionValide(a *appairageB, session, etat string) bool {
	return a != nil && a.ouverte() && a.Etat == etat && a.Session != "" && securite.EgalTempsConstant(session, a.Session)
}

func (s *Service) appairageRevele(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Session string `json:"session"`
		AleaA   string `json:"alea_a"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.appairageB
	if !s.sessionValide(a, c.Session, "engage") {
		erreur(w, http.StatusForbidden, "session d'appairage invalide")
		return
	}
	aleaA, err := hex.DecodeString(c.AleaA)
	if err != nil || len(aleaA) != 32 {
		erreur(w, http.StatusBadRequest, "aléa invalide")
		return
	}
	a.Code = securite.CodeVerification(a.Pair.FP, s.ID.Empreinte, aleaA, a.Pair.AleaB)
	a.Etat = "a_confirmer"
	ecrireJSON(w, 200, map[string]string{"alea_b": hex.EncodeToString(a.Pair.AleaB)})
}

func (s *Service) appairageConfirmer(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Session    string `json:"session"`
		JetonPourB string `json:"jeton_pour_b"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	s.mu.Lock()
	a := s.appairageB
	if !s.sessionValide(a, c.Session, "a_confirmer") {
		s.mu.Unlock()
		erreur(w, http.StatusForbidden, "session d'appairage invalide")
		return
	}
	reste := time.Until(a.Expire)
	s.mu.Unlock()
	if len(c.JetonPourB) < 20 || len(c.JetonPourB) > 128 {
		erreur(w, http.StatusBadRequest, "jeton invalide")
		return
	}
	var accepte bool
	select {
	case accepte = <-a.decision:
	case <-time.After(reste):
		s.mu.Lock()
		a.Etat = "expire"
		s.mu.Unlock()
		erreur(w, http.StatusRequestTimeout, "pas de décision sur ce PC dans le temps imparti")
		return
	case <-r.Context().Done():
		return
	}
	if !accepte {
		erreur(w, http.StatusForbidden, "appairage refusé sur ce PC")
		return
	}
	jetonPourA := securite.NouveauJeton()
	p := a.Pair
	if err := s.Reg.AjouterPC(p.FP, p.Nom, p.CAPEM, p.IP, p.Port, c.JetonPourB, jetonPourA); err != nil {
		erreur(w, http.StatusInternalServerError, "enregistrement impossible")
		return
	}
	s.mu.Lock()
	a.Etat = "termine"
	s.mu.Unlock()
	s.Evenement("info", fmt.Sprintf("Appairage : « %s » ajouté", p.Nom), "")
	ecrireJSON(w, 200, map[string]string{"jeton_pour_a": jetonPourA})
}

// DecisionAppairage : l'utilisateur de B a comparé les codes.
func (s *Service) DecisionAppairage(accepter bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.appairageB
	if a == nil || a.Etat != "a_confirmer" {
		return fmt.Errorf("aucun code à confirmer")
	}
	if !accepter {
		a.Etat = "refuse"
	}
	a.decision <- accepter
	return nil
}

// TailleLisible : 1,5 Mo, 320 Ko…
func TailleLisible(n int64) string {
	switch {
	case n >= 1e9:
		return strings.Replace(fmt.Sprintf("%.1f Go", float64(n)/1e9), ".", ",", 1)
	case n >= 1e6:
		return strings.Replace(fmt.Sprintf("%.1f Mo", float64(n)/1e6), ".", ",", 1)
	case n >= 1e3:
		return fmt.Sprintf("%d Ko", n/1000)
	}
	return fmt.Sprintf("%d octets", n)
}
