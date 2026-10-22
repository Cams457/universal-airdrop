// Package serveur : le démon. Serveur HTTPS sur le réseau local (toutes interfaces : box, partage
// de connexion, point d'accès) et interface de contrôle sur 127.0.0.1 (interface graphique et CLI).
package serveur

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Cams457/universal-airdrop/internal/appareils"
	"github.com/Cams457/universal-airdrop/internal/blocs"
	"github.com/Cams457/universal-airdrop/internal/boite"
	"github.com/Cams457/universal-airdrop/internal/client"
	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/decouverte"
	"github.com/Cams457/universal-airdrop/internal/identite"
	"github.com/Cams457/universal-airdrop/internal/mdns"
	"github.com/Cams457/universal-airdrop/internal/partage"
	"github.com/Cams457/universal-airdrop/internal/reseau"
	"github.com/Cams457/universal-airdrop/internal/securite"
)

// Version du programme (fixée à la compilation).
var Version = "dev"

// Evenement affiché dans l'interface (reçus, liens, erreurs).
type Evenement struct {
	Heure  time.Time `json:"heure"`
	Type   string    `json:"type"` // recu, lien, envoye, info, erreur
	Texte  string    `json:"texte"`
	Chemin string    `json:"chemin,omitempty"`
	// Titre et Corps : la notification Windows à afficher (vide = aucune).
	Titre string `json:"-"`
	Corps string `json:"-"`
}

// Service : l'état complet du démon.
type Service struct {
	Cfg        *config.Config
	ID         *identite.Identite
	Reg        *appareils.Registre
	Partages   *partage.Registre
	Boites     *boite.Boites
	Decouverte *decouverte.Decouverte
	Mdns       *mdns.Annonce
	Secret     string
	Verbeux    bool
	// SurEvenement est appelée à chaque événement (icône de notification).
	SurEvenement func(Evenement)
	// OuvrirLien est remplaçable par les tests.
	OuvrirLien func(string) error
	// CopierTexte met un lien ou un texte reçu dans le presse-papiers (nil : désactivé).
	CopierTexte func(string) error
	// Notifier affiche une notification système (nil : aucune).
	Notifier func(titre, corps string) error

	mu          sync.Mutex
	receptions  map[string]*blocs.Reception
	appairageB  *appairageB
	appairagesA map[string]*appairageA
	envois      map[string]*envoi
	selections  map[string]selection
	evenements  []Evenement
	echecs      map[string][]time.Time
	distants    map[string][]string // téléphones des PC appairés (relais)
	demandes    map[string]*demande // iPhone demandant à être ajouté (sans QR code)
	attente     struct {            // fichiers reçus en attente de notification groupée
		n      int
		auteur string
		nom    string
		total  int64
		minute *time.Timer
	}

	public, controle *http.Server
	annuler          context.CancelFunc
}

// Nouveau prépare le service (identité, registre) sans rien démarrer.
func Nouveau(cfg *config.Config) (*Service, error) {
	id, err := identite.Ouvrir(cfg.Donnees, cfg.Nom, []string{cfg.NomMdns() + ".local"},
		func() []string { return reseau.IPs(false) })
	if err != nil {
		return nil, fmt.Errorf("identité : %w", err)
	}
	s := &Service{Cfg: cfg, ID: id, Reg: appareils.Ouvrir(cfg.Donnees), Partages: partage.NouveauRegistre(),
		Secret: securite.NouveauJeton(), Verbeux: true, receptions: map[string]*blocs.Reception{},
		appairagesA: map[string]*appairageA{}, envois: map[string]*envoi{}, selections: map[string]selection{},
		echecs: map[string][]time.Time{}, distants: map[string][]string{}, demandes: map[string]*demande{},
		Boites: boite.Ouvrir(cfg.Donnees)}
	s.Partages.SurFin = func(p *partage.Partage) {
		if p.Temporaires && len(p.Fichiers) > 0 {
			os.RemoveAll(filepath.Dir(p.Fichiers[0]))
		}
	}
	return s, nil
}

func (s *Service) tmp() string { return filepath.Join(s.Cfg.DossierReception(), ".en_cours") }

func (s *Service) temporaire() string { return filepath.Join(s.Cfg.Donnees, "temporaire") }

// Evenement enregistre un événement (40 derniers gardés) et le journalise.
func (s *Service) Evenement(type_, texte, chemin string) {
	s.evenement(Evenement{Heure: time.Now(), Type: type_, Texte: texte, Chemin: chemin})
}

// EvenementNotifie : un événement accompagné d'une notification Windows.
func (s *Service) EvenementNotifie(type_, texte, chemin, titre, corps string) {
	s.evenement(Evenement{Heure: time.Now(), Type: type_, Texte: texte, Chemin: chemin, Titre: titre, Corps: corps})
}

func (s *Service) evenement(e Evenement) {
	texte := e.Texte
	s.mu.Lock()
	s.evenements = append(s.evenements, e)
	if len(s.evenements) > 40 {
		s.evenements = s.evenements[len(s.evenements)-40:]
	}
	s.mu.Unlock()
	ligne := e.Heure.Format("2006-01-02 15:04:05") + "  " + texte
	if f, err := os.OpenFile(filepath.Join(s.Cfg.Donnees, "journal.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		fmt.Fprintln(f, ligne)
		f.Close()
	}
	if s.Verbeux {
		log.Println(texte)
	}
	if s.SurEvenement != nil {
		go s.SurEvenement(e)
	}
	if e.Titre != "" && s.Notifier != nil {
		go s.Notifier(e.Titre, e.Corps)
	}
}

// regroupement : une rafale de fichiers (40 photos) donne une seule notification.
const regroupement = 1500 * time.Millisecond

// recu enregistre un fichier reçu et programme une notification groupée.
func (s *Service) recu(auteur, chemin string, taille int64) {
	s.Evenement("recu", fmt.Sprintf("Reçu de « %s » : %s (%s)", auteur, filepath.Base(chemin), TailleLisible(taille)), chemin)
	s.mu.Lock()
	defer s.mu.Unlock()
	a := &s.attente
	if a.n > 0 && a.auteur != auteur {
		a.auteur = "plusieurs appareils"
	} else {
		a.auteur = auteur
	}
	a.n++
	a.nom = filepath.Base(chemin)
	a.total += taille
	if a.minute != nil {
		a.minute.Stop()
	}
	a.minute = time.AfterFunc(regroupement, s.notifierRecus)
}

func (s *Service) notifierRecus() {
	s.mu.Lock()
	a := s.attente
	s.attente.n, s.attente.total, s.attente.minute = 0, 0, nil
	s.mu.Unlock()
	if a.n == 0 || s.Notifier == nil {
		return
	}
	titre, corps := "Fichier reçu de "+a.auteur, a.nom+" · "+TailleLisible(a.total)
	if a.n > 1 {
		titre = fmt.Sprintf("%d fichiers reçus de %s", a.n, a.auteur)
		corps = TailleLisible(a.total) + " · dossier « Universal Airdrop » des Téléchargements"
	}
	_ = s.Notifier(titre, corps)
}

// Source d'adresses pour le client : la découverte de ce démon.
func (s *Service) Source() client.Source {
	return func(fp string) map[string]*decouverte.Adresse {
		if s.Decouverte == nil {
			return nil
		}
		if p, ok := s.Decouverte.Actifs()[fp]; ok {
			return p.Adresses
		}
		return nil
	}
}

func (s *Service) ecouteur() (net.Listener, error) {
	lc := net.ListenConfig{}
	// toutes les interfaces : box, partage de connexion de l'iPhone, point d'accès, Bluetooth
	return lc.Listen(context.Background(), "tcp4", fmt.Sprintf("0.0.0.0:%d", s.Cfg.Port))
}

// Demarrer lance les serveurs, la découverte, Bonjour et la surveillance des IP.
func (s *Service) Demarrer() error {
	if err := os.MkdirAll(s.Cfg.DossierReception(), 0o755); err != nil {
		return err
	}
	os.RemoveAll(s.temporaire())
	s.nettoyerRelais()
	ctx, annuler := context.WithCancel(context.Background())
	s.annuler = annuler

	l, err := s.ecouteur()
	if err != nil {
		return fmt.Errorf("port %d déjà utilisé ? %w", s.Cfg.Port, err)
	}
	s.public = &http.Server{
		Handler:           s.routesPubliques(),
		TLSConfig:         s.ID.ConfigServeur(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		TLSNextProto:      map[string]func(*http.Server, *tls.Conn, http.Handler){}, // HTTP/1.1
		ErrorLog:          log.New(discard{}, "", 0),
	}
	go func() { _ = s.public.ServeTLS(l, "", "") }()

	lc, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", s.Cfg.PortControle()))
	if err != nil {
		s.public.Close()
		return fmt.Errorf("port %d déjà utilisé ? %w", s.Cfg.PortControle(), err)
	}
	s.controle = &http.Server{Handler: s.routesControle(), ReadHeaderTimeout: 15 * time.Second}
	go func() { _ = s.controle.Serve(lc) }()
	if err := config.EcrireJSON(filepath.Join(s.Cfg.Donnees, "controle.json"),
		map[string]any{"port": s.Cfg.PortControle(), "secret": s.Secret, "pid": os.Getpid()}, true); err != nil {
		return err
	}

	if s.Cfg.Decouverte {
		s.Decouverte = decouverte.Nouvelle(s.Cfg.Nom, s.ID.Empreinte, s.Cfg.Port, s.Reg.MettreAJourAdresse)
		if err := s.Decouverte.Demarrer(ctx); err != nil {
			s.Evenement("info", "découverte automatique indisponible : saisie d'adresse ou QR code en repli", "")
			s.Decouverte = nil
		}
	}
	if s.Cfg.Mdns {
		s.Mdns = mdns.Nouvelle(s.Cfg.Nom, s.Cfg.NomMdns(), s.ID.Empreinte, s.Cfg.Port)
		if err := s.Mdns.Demarrer(); err != nil {
			s.Evenement("info", "Bonjour indisponible : le Raccourci devra utiliser une adresse IP", "")
			s.Mdns = nil
		}
	}
	go s.surveiller(ctx)
	s.Evenement("info", fmt.Sprintf("« %s » prêt (port %d)", s.Cfg.Nom, s.Cfg.Port), "")
	return nil
}

// surveiller : toutes les 10 s, une nouvelle IP (partage de connexion activé) donne un nouveau
// certificat, servi à chaud, et une annonce Bonjour à jour.
func (s *Service) surveiller(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	go s.rafraichirDistants(ctx)
	tour := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if neuf, err := s.ID.VerifierServeur(); err == nil && neuf {
			s.Evenement("info", "nouveau réseau détecté : certificat mis à jour", "")
		}
		if m := s.mdnsActuel(); m != nil {
			_, _ = m.Actualiser()
		}
		s.Partages.Nettoyer()
		s.nettoyer()
		s.Boites.Nettoyer()
		if tour++; tour%3 == 0 {
			go s.rafraichirDistants(ctx)
		}
	}
}

func (s *Service) nettoyer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sel := range s.selections {
		if time.Since(sel.Cree) > 30*time.Minute {
			delete(s.selections, id)
		}
	}
	for id, e := range s.envois {
		if e.Fini && time.Since(e.Debut) > 30*time.Minute {
			delete(s.envois, id)
		}
	}
	for k, d := range s.demandes {
		if time.Since(d.Cree) > 2*dureeDemande {
			delete(s.demandes, k)
		}
	}
}

// Arreter sauvegarde les réceptions en cours (la reprise repartira de là) et ferme tout.
func (s *Service) Arreter() {
	s.mu.Lock()
	for _, r := range s.receptions {
		r.Sauvegarder()
	}
	s.mu.Unlock()
	if s.annuler != nil {
		s.annuler()
	}
	if m := s.mdnsActuel(); m != nil {
		m.Arreter()
	}
	ctx, fin := context.WithTimeout(context.Background(), 3*time.Second)
	defer fin()
	if s.public != nil {
		_ = s.public.Shutdown(ctx)
	}
	if s.controle != nil {
		_ = s.controle.Shutdown(ctx)
	}
	os.Remove(filepath.Join(s.Cfg.Donnees, "controle.json"))
}

func (s *Service) mdnsActuel() *mdns.Annonce {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Mdns
}

// Renommer applique un nouveau nom immédiatement : certificat, Bonjour et découverte.
func (s *Service) Renommer(nom string) error {
	s.Cfg.Nom = nom
	if err := s.Cfg.Sauver(); err != nil {
		return err
	}
	s.ID.DefinirNomsDNS([]string{s.Cfg.NomMdns() + ".local"})
	if _, err := s.ID.VerifierServeur(); err != nil {
		return err
	}
	if s.Decouverte != nil {
		s.Decouverte.DefinirNom(nom)
	}
	if ancienne := s.mdnsActuel(); ancienne != nil {
		ancienne.Arreter()
		nouvelle := mdns.Nouvelle(nom, s.Cfg.NomMdns(), s.ID.Empreinte, s.Cfg.Port)
		if err := nouvelle.Demarrer(); err != nil {
			nouvelle = nil
		}
		s.mu.Lock()
		s.Mdns = nouvelle
		s.mu.Unlock()
	}
	s.Evenement("info", fmt.Sprintf("Ce PC s'appelle maintenant « %s »", nom), "")
	return nil
}

// URLInterface : adresse de l'interface graphique, avec le secret dans le fragment (jamais envoyé
// au serveur ni enregistré dans l'historique des requêtes).
func (s *Service) URLInterface(chemin string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/%s#k=%s", s.Cfg.PortControle(), strings.TrimPrefix(chemin, "/"), s.Secret)
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

var errInterne = errors.New("erreur interne")
