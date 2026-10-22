package serveur

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/Cams457/universal-airdrop/internal/client"
	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/identite"
	"github.com/Cams457/universal-airdrop/internal/partage"
	"github.com/Cams457/universal-airdrop/internal/reseau"
	"github.com/Cams457/universal-airdrop/internal/securite"
	"github.com/Cams457/universal-airdrop/internal/systeme"
	"github.com/Cams457/universal-airdrop/internal/ui"
)

// SurQuitter est appelée quand l'interface demande l'arrêt.
var SurQuitter func()

type selection struct {
	Chemins     []string
	Temporaires bool
	Cree        time.Time
}

type envoi struct {
	ID       string
	Cible    string
	Fichiers int
	Total    int64
	FaitA    atomic.Int64
	Debut    time.Time
	Fini     bool
	Erreur   string
	Mbits    float64
	Bilans   []client.Bilan
	Annule   bool
	annuler  context.CancelFunc
}

type vueEnvoi struct {
	ID       string    `json:"id"`
	Cible    string    `json:"cible"`
	Fichiers int       `json:"fichiers"`
	Total    int64     `json:"total"`
	Fait     int64     `json:"fait"`
	Debut    time.Time `json:"debut"`
	Fini     bool      `json:"fini"`
	Erreur   string    `json:"erreur,omitempty"`
	Mbits    float64   `json:"mbits"`
	Annule   bool      `json:"annule"`
}

type appairageA struct {
	Etat     string `json:"etat"` // connexion, a_confirmer, termine, erreur
	Code     string `json:"code,omitempty"`
	NomB     string `json:"nom,omitempty"`
	FPB      string `json:"empreinte,omitempty"`
	Erreur   string `json:"erreur,omitempty"`
	decision chan bool
}

// filtreControle : uniquement depuis ce PC, avec le bon nom d'hôte (contre le « DNS rebinding »)
// et le secret local pour toute action (/controle/*). Les fichiers de l'interface sont publics.
func (s *Service) filtreControle(h http.Handler) http.Handler {
	hotes := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", s.Cfg.PortControle()): true,
		fmt.Sprintf("localhost:%d", s.Cfg.PortControle()): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := ipDe(r); ip != "127.0.0.1" && ip != "::1" {
			erreur(w, http.StatusForbidden, "contrôle local uniquement")
			return
		}
		if !hotes[r.Host] {
			erreur(w, http.StatusForbidden, "nom d'hôte refusé")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/controle/") &&
			!securite.EgalTempsConstant(r.Header.Get("X-Controle"), s.Secret) {
			erreur(w, http.StatusUnauthorized, "secret de contrôle invalide")
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (s *Service) routesControle() http.Handler {
	m := http.NewServeMux()
	m.Handle("GET /", ui.Interface())
	m.Handle("GET /statique/", http.StripPrefix("/statique/", ui.Public())) // guide.js partagé
	m.HandleFunc("GET /controle/etat", s.cEtat)
	m.HandleFunc("GET /controle/vus", s.cVus)
	m.HandleFunc("POST /controle/reglages", s.cReglages)
	m.HandleFunc("POST /controle/systeme", s.cSysteme)
	m.HandleFunc("POST /controle/ouvrir", s.cOuvrir)
	m.HandleFunc("POST /controle/appairage/ouvrir", func(w http.ResponseWriter, r *http.Request) {
		s.OuvrirAppairage()
		ecrireJSON(w, 200, map[string]int{"duree": config.DureeAppairageS})
	})
	m.HandleFunc("GET /controle/appairage", s.cAppairageB)
	m.HandleFunc("POST /controle/appairage/decision", s.cDecisionB)
	m.HandleFunc("POST /controle/appairer", s.cAppairer)
	m.HandleFunc("GET /controle/appairer/{id}", s.cEtatAppairer)
	m.HandleFunc("POST /controle/appairer/{id}/decision", s.cDecisionA)
	m.HandleFunc("POST /controle/selection", s.cSelection)
	m.HandleFunc("GET /controle/selection/{id}", s.cLireSelection)
	m.HandleFunc("POST /controle/televerser", s.cTeleverser)
	m.HandleFunc("POST /controle/envoyer", s.cEnvoyer)
	m.HandleFunc("POST /controle/lien", s.cLien)
	m.HandleFunc("POST /controle/partager", s.cPartager)
	m.HandleFunc("POST /controle/telephone/ajouter", s.cAjouterTelephone)
	m.HandleFunc("POST /controle/telephone/envoi", s.cEnvoiTelephone)
	m.HandleFunc("POST /controle/revoquer", s.cRevoquer)
	m.HandleFunc("POST /controle/telephone/demande", s.cDecisionDemande)
	m.HandleFunc("POST /controle/envoi/{id}/annuler", s.cAnnulerEnvoi)
	m.HandleFunc("POST /controle/boite/vider", s.cViderBoite)
	m.HandleFunc("GET /controle/telephone/page", s.cPageIphone)
	m.HandleFunc("POST /controle/quitter", func(w http.ResponseWriter, r *http.Request) {
		ecrireJSON(w, 200, map[string]bool{"ok": true})
		if SurQuitter != nil {
			go func() { time.Sleep(200 * time.Millisecond); SurQuitter() }()
		}
	})
	return s.filtreControle(m)
}

// QR renvoie une image PNG de QR code, en data URI, pour l'interface.
func QR(texte string) string {
	png, err := qrcode.Encode(texte, qrcode.Medium, 480)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

type adresseUI struct {
	reseau.Interface
	URL string `json:"url"`
}

func (s *Service) adresses() []adresseUI {
	var l []adresseUI
	for _, i := range reseau.Classees() {
		l = append(l, adresseUI{Interface: i, URL: fmt.Sprintf("https://%s:%d", i.IP, s.Cfg.Port)})
	}
	return l
}

func (s *Service) cEtat(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	evts := append([]Evenement(nil), s.evenements...)
	envois := []vueEnvoi{} // copies prises sous verrou : l'envoi continue pendant l'encodage
	for _, e := range s.envois {
		envois = append(envois, vueEnvoi{ID: e.ID, Cible: e.Cible, Fichiers: e.Fichiers, Total: e.Total,
			Fait: e.FaitA.Load(), Debut: e.Debut, Fini: e.Fini, Erreur: e.Erreur, Mbits: e.Mbits, Annule: e.Annule})
	}
	s.mu.Unlock()
	vus := map[string]string{}
	if s.Decouverte != nil {
		for fp, p := range s.Decouverte.Actifs() {
			vus[fp] = p.Nom
		}
	}
	nomMdns := ""
	if m := s.mdnsActuel(); m != nil {
		nomMdns = m.NomComplet()
	}
	ecrireJSON(w, 200, map[string]any{
		"version": Version, "nom": s.Cfg.Nom, "nom_mdns": nomMdns, "empreinte": identite.Lisible(s.ID.Empreinte),
		"port": s.Cfg.Port, "reception": s.Cfg.DossierReception(), "ouvrir_liens": s.Cfg.OuvrirLiens,
		"configure": s.Cfg.Configure, "lien_raccourci": s.Cfg.LienRaccourci,
		"adresses": s.adresses(), "appareils": s.Reg.Liste(), "vus": vus,
		"evenements": evts, "envois": envois, "systeme": systeme.LireEtat(),
		"telephones_distants": s.telephonesDistants(), "en_attente": s.enAttente(), "demandes": s.demandesEnAttente(),
	})
}

func (s *Service) cVus(w http.ResponseWriter, r *http.Request) {
	if s.Decouverte == nil {
		ecrireJSON(w, 200, map[string]any{})
		return
	}
	ecrireJSON(w, 200, s.Decouverte.Actifs())
}

func (s *Service) cReglages(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nom           *string `json:"nom"`
		Reception     *string `json:"reception"`
		OuvrirLiens   *bool   `json:"ouvrir_liens"`
		Configure     *bool   `json:"configure"`
		LienRaccourci *string `json:"lien_raccourci"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	if c.Nom != nil {
		n := strings.TrimSpace(*c.Nom)
		if n == "" || len([]rune(n)) > 60 {
			erreur(w, http.StatusBadRequest, "nom vide ou trop long (60 caractères au plus)")
			return
		}
		if n != s.Cfg.Nom {
			if err := s.Renommer(n); err != nil {
				erreur(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}
	if c.Reception != nil {
		if !filepath.IsAbs(*c.Reception) {
			erreur(w, http.StatusBadRequest, "le dossier doit être un chemin complet")
			return
		}
		s.Cfg.Reception = *c.Reception
	}
	if c.OuvrirLiens != nil {
		s.Cfg.OuvrirLiens = *c.OuvrirLiens
	}
	if c.Configure != nil {
		s.Cfg.Configure = *c.Configure
	}
	if c.LienRaccourci != nil {
		if *c.LienRaccourci != "" && !strings.HasPrefix(*c.LienRaccourci, "https://www.icloud.com/shortcuts/") {
			erreur(w, http.StatusBadRequest, "le lien doit commencer par https://www.icloud.com/shortcuts/")
			return
		}
		s.Cfg.LienRaccourci = *c.LienRaccourci
	}
	if err := s.Cfg.Sauver(); err != nil {
		erreur(w, http.StatusInternalServerError, err.Error())
		return
	}
	ecrireJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Service) cSysteme(w http.ResponseWriter, r *http.Request) {
	var c struct {
		PareFeu     bool  `json:"pare_feu"`
		Demarrage   *bool `json:"demarrage"`
		EnvoyerVers *bool `json:"envoyer_vers"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	var err error
	switch {
	case c.PareFeu:
		err = systeme.AjouterPareFeu()
	case c.Demarrage != nil:
		err = systeme.Demarrage(*c.Demarrage)
	case c.EnvoyerVers != nil:
		err = systeme.EnvoyerVers(*c.EnvoyerVers)
	}
	if err != nil {
		erreur(w, http.StatusBadRequest, err.Error())
		return
	}
	ecrireJSON(w, 200, systeme.LireEtat())
}

func (s *Service) cOuvrir(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Chemin string `json:"chemin"`
	}
	_ = lireJSON(w, r, &c)
	dossier := s.Cfg.DossierReception()
	// seulement un fichier DU dossier de réception : l'interface ne peut pas ouvrir n'importe quoi
	if c.Chemin != "" && filepath.Dir(filepath.Clean(c.Chemin)) == filepath.Clean(dossier) {
		_ = systeme.OuvrirFichier(c.Chemin)
	} else {
		_ = os.MkdirAll(dossier, 0o755)
		_ = systeme.OuvrirDossier(dossier)
	}
	ecrireJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- appairage ----------

func (s *Service) cAppairageB(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.appairageB
	if a == nil {
		ecrireJSON(w, 200, map[string]string{"etat": "ferme"})
		return
	}
	etat := a.Etat
	if !a.ouverte() && etat != "termine" && etat != "refuse" {
		etat = "expire"
	}
	rep := map[string]any{"etat": etat, "reste": max(0, int(time.Until(a.Expire).Seconds()))}
	if a.Pair.FP != "" {
		rep["nom"], rep["ip"], rep["empreinte"] = a.Pair.Nom, a.Pair.IP, identite.Lisible(a.Pair.FP)
	}
	if etat == "a_confirmer" {
		rep["code"] = a.Code
	}
	ecrireJSON(w, 200, rep)
}

func (s *Service) cDecisionB(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Accepter bool `json:"accepter"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	if err := s.DecisionAppairage(c.Accepter); err != nil {
		erreur(w, http.StatusConflict, err.Error())
		return
	}
	ecrireJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Service) moi() client.Moi {
	return client.Moi{Nom: s.Cfg.Nom, Port: s.Cfg.Port, ID: s.ID, Reg: s.Reg}
}

// cAppairer : côté A, lancé depuis l'interface ; le code s'affiche, l'utilisateur confirme.
func (s *Service) cAppairer(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Hote string `json:"hote"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	hote, port := strings.TrimSpace(c.Hote), s.Cfg.Port
	if h, p, ok := strings.Cut(hote, ":"); ok {
		v, err := strconv.Atoi(p)
		if err != nil {
			erreur(w, http.StatusBadRequest, "port invalide")
			return
		}
		hote, port = h, v
	}
	if hote == "" {
		erreur(w, http.StatusBadRequest, "adresse vide")
		return
	}
	id := securite.JetonCourt()
	a := &appairageA{Etat: "connexion", decision: make(chan bool, 1)}
	s.mu.Lock()
	s.appairagesA[id] = a
	s.mu.Unlock()
	go func() {
		ctx, fin := context.WithTimeout(context.Background(), time.Duration(config.DureeAppairageS+30)*time.Second)
		defer fin()
		nom, _, err := client.Appairer(ctx, s.moi(), hote, port, func(code, nomB, fpB string) bool {
			s.mu.Lock()
			a.Etat, a.Code, a.NomB, a.FPB = "a_confirmer", code, nomB, fpB
			s.mu.Unlock()
			select {
			case d := <-a.decision:
				return d
			case <-ctx.Done():
				return false
			}
		})
		s.mu.Lock()
		if err != nil {
			a.Etat, a.Erreur = "erreur", err.Error()
		} else {
			a.Etat, a.NomB = "termine", nom
		}
		s.mu.Unlock()
		if err == nil {
			s.Evenement("info", fmt.Sprintf("Appairé avec « %s »", nom), "")
		}
	}()
	ecrireJSON(w, 200, map[string]string{"id": id})
}

func (s *Service) cEtatAppairer(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.appairagesA[r.PathValue("id")]
	if !ok {
		erreur(w, http.StatusNotFound, "appairage inconnu")
		return
	}
	ecrireJSON(w, 200, a)
}

func (s *Service) cDecisionA(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Accepter bool `json:"accepter"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	s.mu.Lock()
	a, ok := s.appairagesA[r.PathValue("id")]
	s.mu.Unlock()
	if !ok || a.Etat != "a_confirmer" {
		erreur(w, http.StatusConflict, "aucun code à confirmer")
		return
	}
	select {
	case a.decision <- c.Accepter:
	default:
	}
	ecrireJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- sélections, envois, partages ----------

func (s *Service) nouvelleSelection(chemins []string, temp bool) string {
	id := securite.JetonCourt()
	s.mu.Lock()
	s.selections[id] = selection{Chemins: chemins, Temporaires: temp, Cree: time.Now()}
	s.mu.Unlock()
	return id
}

// cSelection : la ligne de commande (« Envoyer vers ») dépose des chemins ; l'interface choisit la cible.
func (s *Service) cSelection(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Chemins []string `json:"chemins"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	var ok []string
	for _, ch := range c.Chemins {
		if st, err := os.Stat(ch); err == nil && st.Mode().IsRegular() {
			ok = append(ok, ch)
		}
	}
	if len(ok) == 0 {
		erreur(w, http.StatusBadRequest, "aucun fichier valide")
		return
	}
	id := s.nouvelleSelection(ok, false)
	ecrireJSON(w, 200, map[string]string{"id": id, "url": s.URLInterface("?sel=" + id)})
}

func (s *Service) cLireSelection(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	sel, ok := s.selections[r.PathValue("id")]
	s.mu.Unlock()
	if !ok {
		erreur(w, http.StatusNotFound, "sélection expirée")
		return
	}
	type f struct {
		Nom    string `json:"nom"`
		Taille string `json:"taille"`
	}
	var l []f
	for _, ch := range sel.Chemins {
		if st, err := os.Stat(ch); err == nil {
			l = append(l, f{filepath.Base(ch), TailleLisible(st.Size())})
		}
	}
	ecrireJSON(w, 200, map[string]any{"fichiers": l})
}

// cTeleverser : fichiers glissés dans l'interface, copiés dans un dossier temporaire.
func (s *Service) cTeleverser(w http.ResponseWriter, r *http.Request) {
	lecteur, err := r.MultipartReader()
	if err != nil {
		erreur(w, http.StatusBadRequest, "formulaire invalide")
		return
	}
	dossier := filepath.Join(s.temporaire(), securite.JetonCourt())
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		erreur(w, http.StatusInternalServerError, err.Error())
		return
	}
	var chemins []string
	for {
		p, err := lecteur.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			os.RemoveAll(dossier)
			erreur(w, http.StatusBadRequest, "envoi interrompu")
			return
		}
		if p.FileName() == "" {
			continue
		}
		cible, _ := securite.CheminLibre(dossier, securite.NomSur(p.FileName()))
		f, err := os.Create(cible)
		if err == nil {
			_, err = io.CopyBuffer(f, p, make([]byte, 1<<20))
			f.Close()
		}
		if err != nil {
			os.RemoveAll(dossier)
			erreur(w, http.StatusInternalServerError, "copie impossible")
			return
		}
		chemins = append(chemins, cible)
	}
	if len(chemins) == 0 {
		os.RemoveAll(dossier)
		erreur(w, http.StatusBadRequest, "aucun fichier")
		return
	}
	ecrireJSON(w, 200, map[string]string{"selection": s.nouvelleSelection(chemins, true)})
}

func (s *Service) prendreSelection(id string) (selection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sel, ok := s.selections[id]
	if ok {
		delete(s.selections, id)
	}
	return sel, ok
}

// cibles : la liste des destinataires d'une requête (« cibles », ou « cible » pour l'ancien format),
// sans doublon ni nom vide.
func cibles(liste []string, une string) []string {
	vus := map[string]bool{}
	var l []string
	for _, c := range append(liste, une) {
		c = strings.TrimSpace(c)
		if c != "" && !vus[strings.ToLower(c)] {
			vus[strings.ToLower(c)] = true
			l = append(l, c)
		}
	}
	return l
}

// planInterface : les appareils cochés dans l'interface (PC, téléphones de ce PC, téléphones
// d'un autre PC joints par relais).
func (s *Service) planInterface(noms []string, distants []distant) (plan, error) {
	p := s.resoudre(noms, s.Cfg.Nom)
	if len(p.Inconnus) > 0 {
		return p, fmt.Errorf("appareil inconnu : %s", strings.Join(p.Inconnus, ", "))
	}
	for _, d := range distants {
		if _, pc, ok := s.Reg.TrouverPC(d.Via); ok && strings.TrimSpace(d.Nom) != "" {
			p.Distants = append(p.Distants, distant{Nom: strings.TrimSpace(d.Nom), Via: pc.Nom})
		} else {
			return p, fmt.Errorf("« %s » n'est pas un PC appairé", d.Via)
		}
	}
	p.Moi = false // s'envoyer à soi-même n'a pas de sens ici
	if p.vide() {
		return p, errors.New("choisis au moins un appareil")
	}
	return p, nil
}

// cEnvoyer envoie une sélection de fichiers à un ou plusieurs appareils, en parallèle.
func (s *Service) cEnvoyer(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Cible     string    `json:"cible"`
		Cibles    []string  `json:"cibles"`
		Distants  []distant `json:"distants"`
		Selection string    `json:"selection"`
		Chemins   []string  `json:"chemins"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	p, err := s.planInterface(cibles(c.Cibles, c.Cible), c.Distants)
	if err != nil {
		erreur(w, http.StatusBadRequest, err.Error())
		return
	}
	sel := selection{Chemins: c.Chemins}
	if c.Selection != "" {
		var ok bool
		if sel, ok = s.prendreSelection(c.Selection); !ok {
			erreur(w, http.StatusNotFound, "sélection expirée")
			return
		}
	}
	if len(sel.Chemins) == 0 {
		erreur(w, http.StatusBadRequest, "aucun fichier à envoyer")
		return
	}
	ids := s.distribuer(sel.Chemins, p, "", sel.Temporaires)
	ecrireJSON(w, 200, map[string]any{"ids": ids, "boites": p.Tels})
}

func (s *Service) executerEnvoi(e *envoi, pc string, chemins []string, o client.Options) {
	ctx, annuler := context.WithCancel(context.Background())
	defer annuler()
	s.mu.Lock()
	e.annuler = annuler
	s.mu.Unlock()
	o.Paralleles, o.Source = s.Cfg.Paralleles, s.Source()
	o.Progres = func(fait, _ int64) { e.FaitA.Store(fait) }
	bilans, err := client.Envoyer(ctx, s.Reg, pc, chemins, o)
	duree := time.Since(e.Debut)
	annule := ctx.Err() != nil
	s.mu.Lock()
	e.Fini, e.Bilans, e.Annule = true, bilans, annule
	if err != nil && !annule {
		e.Erreur = err.Error()
	} else if err == nil && duree > 0 {
		e.Mbits = 8 * float64(e.Total) / duree.Seconds() / 1e6
	}
	s.mu.Unlock()
	switch {
	case annule:
		s.Evenement("info", fmt.Sprintf("Envoi vers « %s » annulé", e.Cible), "")
	case err != nil:
		s.Evenement("erreur", fmt.Sprintf("Envoi vers « %s » échoué : %s", e.Cible, err), "")
	default:
		s.Evenement("envoye", fmt.Sprintf("Envoyé à « %s » : %d fichier(s), %s", e.Cible, len(chemins), TailleLisible(e.Total)), "")
	}
}

// cAnnulerEnvoi interrompt un envoi en cours ; le PC receveur efface ce qu'il avait déjà reçu.
func (s *Service) cAnnulerEnvoi(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	e, ok := s.envois[r.PathValue("id")]
	var annuler context.CancelFunc
	if ok && !e.Fini {
		annuler = e.annuler
	}
	s.mu.Unlock()
	if !ok {
		erreur(w, http.StatusNotFound, "envoi inconnu")
		return
	}
	if annuler != nil {
		annuler()
	}
	ecrireJSON(w, 200, map[string]bool{"ok": true})
}

// cViderBoite annule ce qui attend encore un téléphone (pas encore récupéré par « Recevoir »).
func (s *Service) cViderBoite(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nom string `json:"nom"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	n := s.Boites.Nombre(c.Nom)
	s.Boites.Vider(c.Nom)
	s.Evenement("info", fmt.Sprintf("Envoi en attente pour « %s » annulé (%d élément(s))", c.Nom, n), "")
	ecrireJSON(w, 200, map[string]int{"annules": n})
}

// cLien envoie un lien ou un texte à un ou plusieurs appareils ; il y est copié dans le
// presse-papiers (ou mis en attente pour un téléphone).
func (s *Service) cLien(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Cible    string    `json:"cible"`
		Cibles   []string  `json:"cibles"`
		Distants []distant `json:"distants"`
		URL      string    `json:"url"`
		Texte    string    `json:"texte"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	texte := strings.TrimSpace(c.URL + c.Texte)
	if !texteAcceptable(texte) {
		erreur(w, http.StatusBadRequest, "texte vide, trop long ou contenant des caractères de contrôle")
		return
	}
	p, err := s.planInterface(cibles(c.Cibles, c.Cible), c.Distants)
	if err != nil {
		erreur(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, fin := context.WithTimeout(r.Context(), 20*time.Second)
	defer fin()
	echecs := s.distribuerTexte(ctx, texte, p, "")
	type resultat struct {
		Cible  string `json:"cible"`
		Erreur string `json:"erreur,omitempty"`
	}
	var res []resultat
	for _, n := range p.PCs {
		res = append(res, resultat{n, echecs[n]})
	}
	for _, d := range p.Distants {
		l := fmt.Sprintf("%s (via %s)", d.Nom, d.Via)
		res = append(res, resultat{l, echecs[l]})
	}
	for _, t := range p.Tels {
		res = append(res, resultat{Cible: t})
	}
	if len(echecs) == len(res) {
		erreur(w, http.StatusBadGateway, res[0].Erreur)
		return
	}
	ecrireJSON(w, 200, map[string]any{"ok": true, "resultats": res})
}

type lienQR struct {
	URL    string `json:"url"`
	QR     string `json:"qr"`
	Reseau string `json:"reseau"`
}

func (s *Service) liensQR(chemin string) []lienQR {
	var l []lienQR
	for _, a := range s.adresses() {
		l = append(l, lienQR{URL: a.URL + chemin, Reseau: a.Reseau})
	}
	if len(l) > 0 {
		l[0].QR = QR(l[0].URL)
	}
	return l
}

func (s *Service) cPartager(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Selection string   `json:"selection"`
		Chemins   []string `json:"chemins"`
		URL       string   `json:"url"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	p := &partage.Partage{Type: partage.Fichiers, Fichiers: c.Chemins, URL: c.URL}
	if c.Selection != "" {
		sel, ok := s.prendreSelection(c.Selection)
		if !ok {
			erreur(w, http.StatusNotFound, "sélection expirée")
			return
		}
		p.Fichiers, p.Temporaires = sel.Chemins, sel.Temporaires
	}
	if p.URL != "" && !securite.URLAutorisee(p.URL) {
		erreur(w, http.StatusBadRequest, "lien refusé : seuls http et https sont acceptés")
		return
	}
	for _, f := range p.Fichiers {
		if st, err := os.Stat(f); err != nil || !st.Mode().IsRegular() {
			erreur(w, http.StatusBadRequest, "fichier introuvable : "+filepath.Base(f))
			return
		}
	}
	if len(p.Fichiers) == 0 && p.URL == "" {
		erreur(w, http.StatusBadRequest, "rien à partager")
		return
	}
	j := s.Partages.Creer(p, config.DureePartageS*time.Second)
	ecrireJSON(w, 200, map[string]any{"liens": s.liensQR("/p/" + j), "duree": config.DureePartageS})
}

// cAjouterTelephone : assistant d'ajout d'un téléphone (profil iOS, puis jeton à usage unique).
func (s *Service) cAjouterTelephone(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nom string `json:"nom"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	nom := strings.TrimSpace(c.Nom)
	if nom == "" {
		nom = "Téléphone"
	}
	jeton, err := s.Reg.AjouterTelephone(nom)
	if err != nil {
		erreur(w, http.StatusInternalServerError, err.Error())
		return
	}
	adresse := fmt.Sprintf("https://%s.local:%d", s.Cfg.NomMdns(), s.Cfg.Port)
	var secours []string
	for _, a := range s.adresses() {
		secours = append(secours, a.URL)
	}
	texte := "Jeton :\n" + jeton + "\n\nAdresse du PC :\n" + adresse + "\n\nAdresses de secours :\n" + strings.Join(secours, "\n")
	jt := s.Partages.Creer(&partage.Partage{Type: partage.Texte, Texte: texte}, 30*time.Minute)
	s.Evenement("info", fmt.Sprintf("Téléphone « %s » ajouté", nom), "")
	ecrireJSON(w, 200, map[string]any{
		"profil": s.liensQR("/iphone"), "jeton": s.liensQR("/p/" + jt),
		"empreinte": identite.Lisible(s.ID.Empreinte), "adresse": adresse,
		"lien_raccourci": s.Cfg.LienRaccourci, "raccourci_qr": qrSiLien(s.Cfg.LienRaccourci),
	})
}

func qrSiLien(l string) string {
	if l == "" {
		return ""
	}
	return QR(l)
}

// cEnvoiTelephone : page d'envoi pour n'importe quel téléphone, valable 15 minutes.
func (s *Service) cEnvoiTelephone(w http.ResponseWriter, r *http.Request) {
	j := s.Partages.Creer(&partage.Partage{Type: partage.Envoi}, config.DureeEnvoiMobileS*time.Second)
	ecrireJSON(w, 200, map[string]any{"liens": s.liensQR("/envoyer/" + j), "duree": config.DureeEnvoiMobileS})
}

func (s *Service) cRevoquer(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nom string `json:"nom"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	n, err := s.Reg.Revoquer(c.Nom)
	if err != nil {
		erreur(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Boites.Vider(c.Nom)
	s.Evenement("info", fmt.Sprintf("« %s » retiré des appareils de confiance", c.Nom), "")
	ecrireJSON(w, 200, map[string]int{"retires": n})
}

// cPageIphone : l'adresse de la page /iphone à ouvrir dans Safari (à taper ou à scanner).
func (s *Service) cPageIphone(w http.ResponseWriter, r *http.Request) {
	local := fmt.Sprintf("https://%s.local:%d/iphone", s.Cfg.NomMdns(), s.Cfg.Port)
	liens := []lienQR{{URL: local, QR: QR(local), Reseau: "nom du PC (Bonjour)"}}
	for _, l := range s.liensQR("/iphone") {
		l.QR = ""
		liens = append(liens, l)
	}
	ecrireJSON(w, 200, map[string]any{"liens": liens, "lien_raccourci": s.Cfg.LienRaccourci,
		"raccourci_qr": qrSiLien(s.Cfg.LienRaccourci)})
}
