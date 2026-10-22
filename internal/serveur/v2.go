package serveur

// v2 : envoi direct entre appareils, sans QR code.
//
//   - Boîte d'envoi par téléphone : le PC garde ce qui est destiné à un iPhone (fichiers, liens,
//     textes) jusqu'à ce que le Raccourci « Recevoir » vienne le chercher (/api/rapide/boite).
//   - Destinations : l'iPhone demande la liste des appareils (/api/rapide/destinations), l'utilisateur
//     en coche un ou plusieurs, et le PC relaie vers les autres PC et téléphones (en-tête
//     X-Destinations). L'iPhone n'a donc besoin de connaître qu'un seul PC.
//   - Relais entre PC : un PC transmet à un autre PC appairé l'appareil d'origine (X-De) et, le cas
//     échéant, le téléphone de ce PC à qui remettre l'envoi (X-Pour).
//   - Ajout d'un iPhone sans QR code : depuis la page /iphone, l'iPhone demande l'accès ; le PC
//     affiche un code à comparer ; une fois accepté, la page remet le jeton.

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Cams457/universal-airdrop/internal/appareils"
	"github.com/Cams457/universal-airdrop/internal/client"
	"github.com/Cams457/universal-airdrop/internal/reseau"
	"github.com/Cams457/universal-airdrop/internal/securite"
	"github.com/Cams457/universal-airdrop/internal/ui"
)

// distant : un téléphone d'un autre PC, joignable par relais.
type distant struct {
	Nom string `json:"nom"`
	Via string `json:"via"`
}

// decoderListe : « A|B » ou « A, B », éventuellement encodé en URL (les Raccourcis savent le faire).
func decoderListe(v string) []string {
	if strings.Contains(v, "%") {
		if d, err := url.QueryUnescape(v); err == nil {
			v = d
		}
	}
	sep := "|"
	if !strings.Contains(v, "|") {
		sep = ","
	}
	var l []string
	for _, x := range strings.Split(v, sep) {
		if x = strings.TrimSpace(x); x != "" {
			l = append(l, x)
		}
	}
	return l
}

// origine : l'auteur affiché et, pour un relais venant d'un PC, les téléphones destinataires.
func origine(r *http.Request, a appareils.Acces) (string, []string) {
	if a.Type != "pc" {
		return a.Nom, nil
	}
	auteur := a.Nom
	if de, err := url.QueryUnescape(r.Header.Get("X-De")); err == nil && strings.TrimSpace(de) != "" {
		de = strings.TrimSpace(de)
		if len(de) > 60 {
			de = de[:60]
		}
		auteur = fmt.Sprintf("%s (via %s)", de, a.Nom)
	}
	return auteur, decoderListe(r.Header.Get("X-Pour"))
}

// plan : où va un envoi.
type plan struct {
	Moi      bool
	PCs      []string
	Tels     []string
	Distants []distant
	Inconnus []string
}

func (p plan) vide() bool {
	return !p.Moi && len(p.PCs) == 0 && len(p.Tels) == 0 && len(p.Distants) == 0
}

// resoudre traduit des noms d'appareils en plan d'envoi. exclure : l'appareil qui envoie.
func (s *Service) resoudre(noms []string, exclure string) plan {
	var p plan
	vus := map[string]bool{}
	for _, n := range noms {
		cle := strings.ToLower(n)
		if vus[cle] {
			continue
		}
		vus[cle] = true
		switch {
		case strings.EqualFold(n, s.Cfg.Nom):
			p.Moi = true
		case strings.EqualFold(n, exclure):
		default:
			if _, pc, ok := s.Reg.TrouverPC(n); ok {
				p.PCs = append(p.PCs, pc.Nom)
			} else if t, ok := s.Reg.TrouverTelephone(n); ok {
				p.Tels = append(p.Tels, t)
			} else {
				p.Inconnus = append(p.Inconnus, n)
			}
		}
	}
	return p
}

// ---------- boîtes d'envoi ----------

// deposer : fichiers pour des téléphones de ce PC (copie : la source peut servir à d'autres envois).
func (s *Service) deposer(tels []string, de string, chemins []string) {
	if de == "" {
		de = s.Cfg.Nom
	}
	for _, t := range tels {
		n := 0
		for _, c := range chemins {
			if _, err := s.Boites.DeposerFichier(t, de, c, false); err == nil {
				n++
			}
		}
		s.EvenementNotifie("envoye", fmt.Sprintf("Pour « %s » : %d fichier(s) en attente — il les reçoit avec l'app (bouton Recevoir) ou le Raccourci « Recevoir »", t, n), "", "", "")
	}
}

func (s *Service) deposerTexte(tels []string, de, texte string) {
	if de == "" {
		de = s.Cfg.Nom
	}
	for _, t := range tels {
		s.Boites.DeposerTexte(t, de, texte, securite.URLAutorisee(texte))
		s.Evenement("envoye", fmt.Sprintf("Pour « %s » : lien ou texte en attente (app ou Raccourci « Recevoir »)", t), "")
	}
}

// distribuer envoie des fichiers déjà reçus vers les PC et téléphones du plan. Si temporaires,
// le dossier des fichiers est supprimé une fois tous les relais terminés.
func (s *Service) distribuer(chemins []string, p plan, de string, temporaires bool) []string {
	s.deposer(p.Tels, de, chemins)
	var tous sync.WaitGroup
	ids := []string{}
	lancer := func(pc, libelle string, o client.Options) {
		var total int64
		for _, c := range chemins {
			if st, err := os.Stat(c); err == nil {
				total += st.Size()
			}
		}
		e := &envoi{ID: securite.JetonCourt(), Cible: libelle, Fichiers: len(chemins), Total: total, Debut: time.Now()}
		s.mu.Lock()
		s.envois[e.ID] = e
		s.mu.Unlock()
		ids = append(ids, e.ID)
		tous.Add(1)
		go func() {
			defer tous.Done()
			s.executerEnvoi(e, pc, chemins, o)
		}()
	}
	for _, pc := range p.PCs {
		lancer(pc, pc, client.Options{De: de})
	}
	for _, d := range p.Distants {
		lancer(d.Via, fmt.Sprintf("%s (via %s)", d.Nom, d.Via), client.Options{De: de, Pour: []string{d.Nom}})
	}
	if temporaires && len(chemins) > 0 {
		go func() {
			tous.Wait()
			os.RemoveAll(filepath.Dir(chemins[0]))
		}()
	}
	return ids
}

// enAttente : nombre d'éléments en attente par téléphone (affiché dans l'interface).
func (s *Service) enAttente() map[string]int {
	m := map[string]int{}
	for _, t := range s.Reg.Telephones() {
		if n := s.Boites.EnAttente(t); n > 0 {
			m[t] = n
		}
	}
	return m
}

// distribuerTexte : un lien ou un texte vers les PC et téléphones du plan. Renvoie les échecs.
func (s *Service) distribuerTexte(ctx context.Context, texte string, p plan, de string) map[string]string {
	s.deposerTexte(p.Tels, de, texte)
	var mu sync.Mutex
	echecs := map[string]string{}
	var tous sync.WaitGroup
	envoyer := func(pc, libelle string, o client.Options) {
		tous.Add(1)
		go func() {
			defer tous.Done()
			o.Source = s.Source()
			if err := client.EnvoyerTexte(ctx, s.Reg, pc, texte, o); err != nil {
				mu.Lock()
				echecs[libelle] = err.Error()
				mu.Unlock()
				s.Evenement("erreur", fmt.Sprintf("Lien non transmis à « %s » : %s", libelle, err), "")
			} else {
				s.Evenement("envoye", fmt.Sprintf("Lien envoyé à « %s »", libelle), "")
			}
		}()
	}
	for _, pc := range p.PCs {
		envoyer(pc, pc, client.Options{De: de})
	}
	for _, d := range p.Distants {
		envoyer(d.Via, fmt.Sprintf("%s (via %s)", d.Nom, d.Via), client.Options{De: de, Pour: []string{d.Nom}})
	}
	tous.Wait()
	return echecs
}

// dossierRelais : dossier temporaire pour un fichier qui ne fait que passer par ce PC. Il est
// placé à côté des réceptions en cours (même disque : le renommage final reste instantané).
func (s *Service) dossierRelais() (string, error) {
	d := filepath.Join(s.tmp(), "relais-"+securite.JetonCourt())
	return d, os.MkdirAll(d, 0o700)
}

// nettoyerRelais : au démarrage, les relais interrompus par un arrêt brutal.
func (s *Service) nettoyerRelais() {
	l, _ := filepath.Glob(filepath.Join(s.tmp(), "relais-*"))
	for _, d := range l {
		os.RemoveAll(d)
	}
}

// ---------- API téléphone ----------

// rDestinations : la liste à proposer dans le Raccourci « Envoyer » (ce PC en premier).
func (s *Service) rDestinations(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	type appareil struct {
		Nom  string `json:"nom"`
		Type string `json:"type"` // pc ou telephone
	}
	l := []string{s.Cfg.Nom}
	app := []appareil{{s.Cfg.Nom, "pc"}}
	for _, n := range s.Reg.NomsPC() {
		l = append(l, n)
		app = append(app, appareil{n, "pc"})
	}
	for _, t := range s.Reg.Telephones() {
		if !strings.EqualFold(t, a.Nom) {
			l = append(l, t)
			app = append(app, appareil{t, "telephone"})
		}
	}
	ecrireJSON(w, 200, map[string]any{"destinations": l, "appareils": app, "ce_pc": s.Cfg.Nom})
}

// albumPhotos : ces fichiers vont dans Photos ; les autres dans l'app Fichiers.
func albumPhotos(nom string) bool {
	switch strings.ToLower(filepath.Ext(nom)) {
	case ".jpg", ".jpeg", ".png", ".heic", ".heif", ".gif", ".webp", ".mov", ".mp4", ".m4v":
		return true
	}
	return false
}

// rBoite : ce qui attend ce téléphone. Les liens et textes sont remis dans la réponse (et retirés) ;
// les fichiers ont chacun une URL à télécharger, retirée une fois le téléchargement complet.
func (s *Service) rBoite(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	if a.Type != "telephone" {
		erreur(w, http.StatusForbidden, "réservé aux téléphones")
		return
	}
	type fichier struct {
		URL    string `json:"url"`
		Nom    string `json:"nom"`
		Taille int64  `json:"taille"`
		Album  bool   `json:"album"`
		// Destination : « photos » ou « fichiers », en texte (plus simple à comparer dans un Raccourci).
		Destination string `json:"destination"`
		De          string `json:"de"`
	}
	// garder=1 (app iPhone) : rien n'est retiré ; l'app marque chaque élément enregistré (ou copié),
	// et tout reste visible dans « Reçus » jusqu'à suppression ou expiration (7 jours).
	garder := r.URL.Query().Get("garder") == "1"
	type element struct {
		Cree        time.Time `json:"cree"`
		Enregistre  bool      `json:"enregistre"`
		ID          string    `json:"id"`
		Type        string    `json:"type"`
		Nom         string    `json:"nom,omitempty"`
		Taille      int64     `json:"taille,omitempty"`
		Texte       string    `json:"texte,omitempty"`
		URL         string    `json:"url,omitempty"`
		Destination string    `json:"destination,omitempty"`
		De          string    `json:"de"`
	}
	fichiers := []fichier{}
	elements := []element{}
	textes, liens := []string{}, []string{}
	des := map[string]bool{}
	var remis []string
	defer func() {
		if len(remis) > 0 {
			s.Boites.Remettre(a.Nom, remis...)
		}
	}()
	for _, e := range s.Boites.Lister(a.Nom) {
		el := element{ID: e.ID, Type: e.Type, Nom: e.Nom, Taille: e.Taille, Texte: e.Texte, De: e.De,
			Cree: e.Cree, Enregistre: e.Enregistre}
		if e.Type == "fichier" {
			el.URL = "https://" + r.Host + "/api/rapide/boite/" + e.ID
			el.Destination = "fichiers"
			if albumPhotos(e.Nom) {
				el.Destination = "photos"
			}
		}
		if garder {
			elements = append(elements, el) // la liste « Reçus » montre aussi les déjà enregistrés
			if e.Type != "fichier" && !e.Remis {
				remis = append(remis, e.ID) // le texte est dans la réponse : il est sur l'iPhone
			}
		}
		if e.Enregistre {
			continue // le résumé, le badge et le Raccourci ne comptent que les nouveautés
		}
		des[e.De] = true
		switch e.Type {
		case "fichier":
			dest := "fichiers"
			if albumPhotos(e.Nom) {
				dest = "photos"
			}
			fichiers = append(fichiers, fichier{URL: "https://" + r.Host + "/api/rapide/boite/" + e.ID,
				Nom: e.Nom, Taille: e.Taille, Album: dest == "photos", Destination: dest, De: e.De})
		default:
			textes = append(textes, e.Texte)
			if e.Type == "lien" {
				liens = append(liens, e.Texte)
			}
			if !garder {
				s.Boites.Retirer(a.Nom, e.ID)
			}
		}
	}
	var origines []string
	for d := range des {
		if d != "" {
			origines = append(origines, d)
		}
	}
	sort.Strings(origines)
	resume := "Rien à recevoir"
	if n := len(fichiers) + len(textes); n > 0 {
		var morceaux []string
		if len(fichiers) > 0 {
			morceaux = append(morceaux, fmt.Sprintf("%d fichier(s)", len(fichiers)))
		}
		if len(textes) > 0 {
			morceaux = append(morceaux, fmt.Sprintf("%d lien(s) ou texte(s), copié(s)", len(textes)))
		}
		resume = strings.Join(morceaux, " et ")
		if len(origines) > 0 {
			resume += " de " + strings.Join(origines, ", ")
		}
	}
	ecrireJSON(w, 200, map[string]any{
		"nombre": len(fichiers) + len(textes), "nombre_fichiers": len(fichiers), "fichiers": fichiers,
		"texte": strings.Join(textes, "\n"), "liens": liens, "resume": resume, "elements": elements,
	})
}

// rBoiteFichier sert un fichier en attente, puis le retire s'il a été lu en entier.
func (s *Service) rBoiteFichier(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	e, chemin, trouve := s.Boites.Fichier(a.Nom, r.PathValue("id"))
	if !trouve {
		erreur(w, http.StatusNotFound, "déjà reçu ou expiré")
		return
	}
	f, err := os.Open(chemin)
	if err != nil {
		erreur(w, http.StatusNotFound, "fichier introuvable")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(e.Nom)))
	switch strings.ToLower(filepath.Ext(e.Nom)) {
	case ".heic":
		ct = "image/heic"
	case ".heif":
		ct = "image/heif"
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(e.Nom))
	ecrit := &compteur{ResponseWriter: w}
	http.ServeContent(ecrit, r, e.Nom, st.ModTime(), f)
	if r.Header.Get("Range") == "" && ecrit.n == st.Size() && r.URL.Query().Get("garder") == "1" {
		s.Boites.Remettre(a.Nom, e.ID) // arrivé sur l'iPhone : plus « en attente » côté PC
	}
	if r.Header.Get("Range") == "" && ecrit.n == st.Size() && r.URL.Query().Get("garder") != "1" {
		s.Boites.Retirer(a.Nom, e.ID)
		s.Evenement("envoye", fmt.Sprintf("« %s » a reçu %s", a.Nom, e.Nom), "")
	}
}

type compteur struct {
	http.ResponseWriter
	n int64
}

func (c *compteur) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.n += int64(n)
	return n, err
}

// rTelephones : à un PC appairé, la liste de nos téléphones (il pourra leur envoyer par relais).
func (s *Service) rTelephones(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	if a.Type != "pc" {
		erreur(w, http.StatusForbidden, "réservé aux PC appairés")
		return
	}
	ecrireJSON(w, 200, map[string]any{"telephones": s.Reg.Telephones()})
}

// rafraichirDistants interroge les PC appairés sur leurs téléphones (toutes les 30 s).
func (s *Service) rafraichirDistants(ctx context.Context) {
	noms := s.Reg.NomsPC()
	res := map[string][]string{}
	var mu sync.Mutex
	var tous sync.WaitGroup
	for _, pc := range noms {
		tous.Add(1)
		go func() {
			defer tous.Done()
			c, fin := context.WithTimeout(ctx, 4*time.Second)
			defer fin()
			if l, err := client.Telephones(c, s.Reg, pc, s.Source()); err == nil {
				mu.Lock()
				res[pc] = l
				mu.Unlock()
			}
		}()
	}
	tous.Wait()
	s.mu.Lock()
	s.distants = res
	s.mu.Unlock()
}

// telephonesDistants : téléphones joignables par relais, hors ceux que ce PC connaît déjà.
func (s *Service) telephonesDistants() []distant {
	locaux := map[string]bool{}
	for _, t := range s.Reg.Telephones() {
		locaux[strings.ToLower(t)] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var l []distant
	vus := map[string]bool{}
	for pc, tels := range s.distants {
		for _, t := range tels {
			if !locaux[strings.ToLower(t)] && !vus[strings.ToLower(t)] {
				vus[strings.ToLower(t)] = true
				l = append(l, distant{Nom: t, Via: pc})
			}
		}
	}
	return l
}

// ---------- ajout d'un iPhone sans QR code ----------

type demande struct {
	Court  string    `json:"id"`
	Nom    string    `json:"nom"`
	Code   string    `json:"code"`
	IP     string    `json:"ip"`
	Cree   time.Time `json:"-"`
	etat   string    // attente, acceptee, refusee
	jeton  string
	secret string
}

const dureeDemande = 5 * time.Minute

func (s *Service) demandeExpiree(d *demande) bool { return time.Since(d.Cree) > dureeDemande }

// rDemander : l'iPhone (page /iphone) demande à être ajouté. Aucun jeton n'est remis tant que
// l'utilisateur n'a pas accepté sur le PC, après avoir comparé le code affiché des deux côtés.
func (s *Service) rDemander(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nom string `json:"nom"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	nom := strings.TrimSpace(c.Nom)
	if r := []rune(nom); len(r) > 40 {
		nom = string(r[:40])
	}
	if nom == "" || !texteAcceptable(nom) || strings.ContainsAny(nom, "|,\n\r\t") {
		erreur(w, http.StatusBadRequest, "nom invalide (pas de | ni de virgule)")
		return
	}
	ip := ipDe(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	attente := 0
	for k, d := range s.demandes {
		if s.demandeExpiree(d) || (d.IP == ip && d.etat == "attente") {
			delete(s.demandes, k) // une seule demande en cours par appareil
			continue
		}
		if d.etat == "attente" {
			attente++
		}
	}
	if attente >= 3 {
		erreur(w, http.StatusTooManyRequests, "trop de demandes en cours : réessaie dans quelques minutes")
		return
	}
	d := &demande{Court: securite.JetonCourt(), Nom: nom, Code: securite.CodeAleatoire(),
		IP: ip, Cree: time.Now(), etat: "attente", secret: securite.NouveauJeton()}
	s.demandes[d.secret] = d
	go s.EvenementNotifie("info", fmt.Sprintf("« %s » demande à être ajouté (code %s)", nom, d.Code), "",
		"Nouvel appareil : "+nom, "Ouvre Universal Airdrop pour accepter. Code : "+d.Code)
	ecrireJSON(w, 200, map[string]any{"id": d.secret, "code": d.Code, "expire_s": int(dureeDemande.Seconds())})
}

// rSuivreDemande : la page interroge l'état ; une fois acceptée, elle reçoit le jeton (une fois).
func (s *Service) rSuivreDemande(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	d, ok := s.demandes[r.PathValue("id")]
	if ok && (s.demandeExpiree(d) || d.etat != "attente") {
		delete(s.demandes, r.PathValue("id"))
	}
	s.mu.Unlock()
	if !ok || (s.demandeExpiree(d) && d.etat == "attente") {
		erreur(w, http.StatusNotFound, "demande expirée : recommence")
		return
	}
	rep := map[string]any{"etat": d.etat}
	if d.etat == "acceptee" {
		var secours []string
		for _, a := range s.adresses() {
			secours = append(secours, a.URL)
		}
		rep["jeton"] = d.jeton
		poserCookieApp(w, d.jeton)
		rep["adresse"] = fmt.Sprintf("https://%s.local:%d", s.Cfg.NomMdns(), s.Cfg.Port)
		if ts := reseau.NomTailscale(); ts != "" {
			rep["adresse_distance"] = fmt.Sprintf("https://%s:%d", ts, s.Cfg.Port)
		}
		rep["secours"] = secours
		rep["pc"] = s.Cfg.Nom
	}
	ecrireJSON(w, 200, rep)
}

// demandesEnAttente : pour l'interface du PC.
func (s *Service) demandesEnAttente() []demande {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := []demande{}
	for _, d := range s.demandes {
		if d.etat == "attente" && !s.demandeExpiree(d) {
			l = append(l, *d)
		}
	}
	return l
}

// cDecisionDemande : l'utilisateur accepte ou refuse, sur le PC.
func (s *Service) cDecisionDemande(w http.ResponseWriter, r *http.Request) {
	var c struct {
		ID       string `json:"id"`
		Accepter bool   `json:"accepter"`
	}
	if !lireJSON(w, r, &c) {
		return
	}
	s.mu.Lock()
	var d *demande
	for _, x := range s.demandes {
		if x.Court == c.ID && x.etat == "attente" && !s.demandeExpiree(x) {
			d = x
		}
	}
	s.mu.Unlock()
	if d == nil {
		erreur(w, http.StatusNotFound, "demande expirée")
		return
	}
	if !c.Accepter {
		s.mu.Lock()
		d.etat = "refusee"
		s.mu.Unlock()
		ecrireJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	jeton, err := s.Reg.AjouterTelephone(d.Nom)
	if err != nil {
		erreur(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mu.Lock()
	d.etat, d.jeton = "acceptee", jeton
	s.mu.Unlock()
	s.Evenement("info", fmt.Sprintf("« %s » ajouté aux appareils de confiance", d.Nom), "")
	ecrireJSON(w, 200, map[string]bool{"ok": true})
}

// rRecu : l'app iPhone accuse réception d'un élément (photo enregistrée, lien copié) : il est retiré.
func (s *Service) rRecu(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	if a.Type != "telephone" {
		erreur(w, http.StatusForbidden, "réservé aux téléphones")
		return
	}
	id := r.PathValue("id")
	e, ok := s.Boites.Marquer(a.Nom, id)
	if !ok {
		erreur(w, http.StatusNotFound, "supprimé ou expiré")
		return
	}
	if e.Type == "fichier" {
		s.Evenement("envoye", fmt.Sprintf("« %s » a enregistré %s", a.Nom, e.Nom), "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// rSupprimer : l'utilisateur retire un élément de sa liste « Reçus » (le fichier est effacé du PC).
func (s *Service) rSupprimer(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	if a.Type != "telephone" {
		erreur(w, http.StatusForbidden, "réservé aux téléphones")
		return
	}
	s.Boites.Retirer(a.Nom, r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// rMoi : l'app iPhone vérifie son jeton et récupère les adresses du PC (pour les Raccourcis).
func (s *Service) rMoi(w http.ResponseWriter, r *http.Request) {
	a, _, ok := s.authentifier(w, r)
	if !ok {
		return
	}
	if a.Type == "telephone" { // l'app relie déjà : la copie de secours est (re)posée
		poserCookieApp(w, strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
	}
	var secours []string
	for _, x := range s.adresses() {
		secours = append(secours, x.URL)
	}
	distance := ""
	if ts := reseau.NomTailscale(); ts != "" {
		distance = fmt.Sprintf("https://%s:%d", ts, s.Cfg.Port)
	}
	hote, _, _ := net.SplitHostPort(r.Host)
	viaTailscale := strings.HasSuffix(strings.ToLower(hote), ".ts.net") || reseau.EstTailscale(hote)
	ecrireJSON(w, 200, map[string]any{"nom": a.Nom, "type": a.Type, "pc": s.Cfg.Nom,
		"adresse": fmt.Sprintf("https://%s.local:%d", s.Cfg.NomMdns(), s.Cfg.Port), "secours": secours,
		"adresse_distance": distance, "via_tailscale": viaTailscale, "version": Version, "en_attente": s.Boites.Nombre(a.Nom)})
}

// pageAppli : l'app iPhone (à ajouter à l'écran d'accueil depuis Safari).
func (s *Service) pageAppli(w http.ResponseWriter, r *http.Request) {
	ui.PageAppli(w, s.Cfg.Nom)
}

func (s *Service) manifeste(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	ecrireJSONBrut(w, map[string]any{
		"name": "Universal Airdrop", "short_name": "Airdrop", "start_url": "/app", "scope": "/",
		"display": "standalone", "background_color": "#f2f2f7", "theme_color": "#2f6fd6", "lang": "fr",
		"icons": []map[string]string{
			{"src": "/statique/app-192.png", "sizes": "192x192", "type": "image/png"},
			{"src": "/statique/app-512.png", "sizes": "512x512", "type": "image/png"},
		},
	})
}

// nouveaux : éléments pas encore enregistrés par le téléphone.
func (s *Service) nouveaux(telephone string) int {
	n := 0
	for _, e := range s.Boites.Lister(telephone) {
		if !e.Enregistre {
			n++
		}
	}
	return n
}

// ---------- copie de secours du jeton de l'app ----------
//
// iOS peut effacer le stockage local d'une page (navigation privée, données de sites effacées, plafond
// de 7 jours de Safari pour le stockage écrit par script) : l'app redemandait alors de relier
// l'iPhone. Le jeton est donc aussi gardé dans un cookie HttpOnly, Secure, SameSite=Strict, limité
// aux routes /api/rapide/ : seul ce PC le lit, et seulement depuis ses propres pages.
const cookieApp = "ua_app"

func poserCookieApp(w http.ResponseWriter, jeton string) {
	if jeton == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieApp, Value: jeton, Path: "/api/rapide/", MaxAge: 400 * 24 * 3600,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func effacerCookieApp(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieApp, Value: "", Path: "/api/rapide/", MaxAge: -1,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// rReprendre : l'app a perdu son jeton ; s'il reste dans le cookie et qu'il est toujours valide,
// il lui est rendu (lecture réservée aux pages de ce PC : pas d'en-têtes CORS). Les échecs comptent
// dans la limitation habituelle.
func (s *Service) rReprendre(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieApp)
	if err != nil || c.Value == "" {
		erreur(w, http.StatusNotFound, "aucun lien mémorisé")
		return
	}
	r.Header.Set("Authorization", "Bearer "+c.Value)
	a, _, ok := s.authentifier(w, r)
	if !ok {
		effacerCookieApp(w)
		return
	}
	if a.Type != "telephone" {
		erreur(w, http.StatusForbidden, "réservé aux téléphones")
		return
	}
	ecrireJSON(w, 200, map[string]string{"jeton": c.Value, "nom": a.Nom})
}

// rOublier : « Délier cet iPhone » efface aussi la copie de secours.
func (s *Service) rOublier(w http.ResponseWriter, r *http.Request) {
	effacerCookieApp(w)
	ecrireJSON(w, 200, map[string]bool{"ok": true})
}
