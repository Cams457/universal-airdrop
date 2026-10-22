// Universal Airdrop : envoi de fichiers et de liens entre PC et téléphones, sur le réseau local.
//
// Double-clic : l'application démarre (icône près de l'horloge) et s'ouvre dans le navigateur.
// En ligne de commande : universal-airdrop aide.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Cams457/universal-airdrop/internal/appareils"
	"github.com/Cams457/universal-airdrop/internal/client"
	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/decouverte"
	"github.com/Cams457/universal-airdrop/internal/identite"
	"github.com/Cams457/universal-airdrop/internal/reseau"
	"github.com/Cams457/universal-airdrop/internal/serveur"
	"github.com/Cams457/universal-airdrop/internal/systeme"
)

var version = "dev"

const aide = `Universal Airdrop %s

Sans argument : lance l'application (icône près de l'horloge) et l'ouvre dans le navigateur.

Commandes :
  ouvrir                     ouvre l'application déjà lancée
  envoyer CIBLE FICHIERS...  envoie vers un PC appairé (--paralleles N)
  lien CIBLE URL             ouvre un lien sur un PC appairé
  appareils                  liste des appareils de confiance
  adresses                   adresses IP de ce PC, réseau par réseau
  empreinte                  empreinte du certificat de ce PC
  nommer "NOM"               nom affiché sur les autres appareils
  appairage-ouvrir           ce PC attend une demande d'appairage (2 min)
  appairer ADRESSE           s'appaire avec un PC qui attend
  revoquer NOM               retire un appareil
  demon                      lance le service sans icône ni navigateur
  bench --local | --cible C  mesure le débit (--mo N, --iperf MBITS)
  version
`

func main() {
	serveur.Version = version
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		attacherConsole()
	}
	if len(args) == 0 || args[0] == "--arriere-plan" {
		lancerApplication(len(args) > 0)
		return
	}
	var err error
	switch args[0] {
	case "demon":
		err = demon()
	case "ouvrir":
		err = ouvrirInterface("")
	case "envoyer-vers":
		err = envoyerVers(args[1:])
	case "envoyer":
		err = envoyer(args[1:])
	case "lien":
		err = lien(args[1:])
	case "appareils":
		err = listerAppareils()
	case "adresses":
		listerAdresses()
	case "empreinte":
		err = empreinte()
	case "nommer":
		err = nommer(args[1:])
	case "appairage-ouvrir":
		err = appairageOuvrir()
	case "appairer":
		err = appairer(args[1:])
	case "revoquer":
		err = revoquer(args[1:])
	case "bench":
		err = bench(args[1:])
	case "version", "--version", "-v":
		fmt.Println("Universal Airdrop", version)
	default:
		fmt.Printf(aide, version)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Erreur :", err)
		os.Exit(1)
	}
}

// ---------- application ----------

func lancerApplication(arrierePlan bool) {
	cfg, err := config.Charger("")
	if err != nil {
		fatal(err)
	}
	if ctl, err := lireControle(cfg); err == nil && ctl.actif() {
		_ = systeme.OuvrirURL(ctl.urlInterface("")) // déjà lancée : on ouvre simplement la fenêtre
		return
	}
	s, err := serveur.Nouveau(cfg)
	if err != nil {
		fatal(err)
	}
	s.Verbeux = false
	s.OuvrirLien = systeme.OuvrirURL
	if err := s.Demarrer(); err != nil {
		fatal(err)
	}
	defer s.Arreter()
	if !arrierePlan || !cfg.Configure {
		_ = systeme.OuvrirURL(s.URLInterface(""))
	}
	icone(s)
}

func demon() error {
	cfg, err := config.Charger("")
	if err != nil {
		return err
	}
	s, err := serveur.Nouveau(cfg)
	if err != nil {
		return err
	}
	s.OuvrirLien = systeme.OuvrirURL
	if err := s.Demarrer(); err != nil {
		return err
	}
	fmt.Printf("« %s » prêt. Interface : %s\nCtrl+C pour arrêter.\n", cfg.Nom, s.URLInterface(""))
	attendreArret()
	s.Arreter()
	return nil
}

// attendreArret : bloque jusqu'à Ctrl+C ou « Quitter » dans l'interface.
func attendreArret() {
	fin := make(chan os.Signal, 1)
	signal.Notify(fin, os.Interrupt, syscall.SIGTERM)
	serveur.SurQuitter = func() { fin <- syscall.SIGTERM }
	<-fin
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Universal Airdrop :", err)
	journal := filepath.Join(config.DossierDonnees(), "erreur.log")
	_ = os.WriteFile(journal, []byte(time.Now().Format(time.RFC3339)+"  "+err.Error()+"\n"), 0o600)
	os.Exit(1)
}

// ---------- accès au démon local ----------

type controle struct {
	Port   int    `json:"port"`
	Secret string `json:"secret"`
	c      *http.Client
}

func lireControle(cfg *config.Config) (*controle, error) {
	var c controle
	ok, err := config.LireJSON(filepath.Join(cfg.Donnees, "controle.json"), &c)
	if !ok || err != nil {
		return nil, errors.New("Universal Airdrop n'est pas lancé")
	}
	c.c = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
	return &c, nil
}

func (c *controle) appeler(methode, chemin string, corps, sortie any) error {
	var r io.Reader
	if corps != nil {
		b, _ := json.Marshal(corps)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(methode, fmt.Sprintf("http://127.0.0.1:%d%s", c.Port, chemin), r)
	req.Header.Set("X-Controle", c.Secret)
	req.Header.Set("Content-Type", "application/json")
	rep, err := c.c.Do(req)
	if err != nil {
		return errors.New("Universal Airdrop ne répond pas : est-il lancé ?")
	}
	defer rep.Body.Close()
	if rep.StatusCode >= 400 {
		var e struct {
			Erreur string `json:"erreur"`
		}
		_ = json.NewDecoder(rep.Body).Decode(&e)
		return errors.New(e.Erreur)
	}
	if sortie != nil {
		return json.NewDecoder(rep.Body).Decode(sortie)
	}
	return nil
}

func (c *controle) actif() bool { return c.appeler("GET", "/controle/etat", nil, nil) == nil }

func (c *controle) urlInterface(chemin string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/%s#k=%s", c.Port, chemin, c.Secret)
}

// source : adresses vues par la découverte du démon local, s'il tourne.
func (c *controle) source() client.Source {
	return func(fp string) map[string]*decouverte.Adresse {
		var vus map[string]decouverte.Pair
		if c == nil || c.appeler("GET", "/controle/vus", nil, &vus) != nil {
			return nil
		}
		return vus[fp].Adresses
	}
}

func ouvrirInterface(chemin string) error {
	cfg, err := config.Charger("")
	if err != nil {
		return err
	}
	ctl, err := lireControle(cfg)
	if err != nil || !ctl.actif() {
		return lancerEnFond()
	}
	return systeme.OuvrirURL(ctl.urlInterface(chemin))
}

// lancerEnFond démarre l'application dans un processus séparé et attend qu'elle réponde.
func lancerEnFond() error {
	cmd := exec.Command(systeme.Executable(), "--arriere-plan")
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

// envoyerVers : appelée par le clic droit > Envoyer vers ; l'interface s'ouvre sur ces fichiers.
func envoyerVers(chemins []string) error {
	cfg, err := config.Charger("")
	if err != nil {
		return err
	}
	ctl, err := lireControle(cfg)
	if err != nil || !ctl.actif() {
		if err := lancerEnFond(); err != nil {
			return err
		}
		for range 50 {
			time.Sleep(200 * time.Millisecond)
			if ctl, err = lireControle(cfg); err == nil && ctl.actif() {
				break
			}
		}
		if ctl == nil || !ctl.actif() {
			return errors.New("l'application n'a pas démarré")
		}
	}
	var abs []string
	for _, c := range chemins {
		if a, err := filepath.Abs(c); err == nil {
			abs = append(abs, a)
		}
	}
	var rep struct {
		URL string `json:"url"`
	}
	if err := ctl.appeler("POST", "/controle/selection", map[string]any{"chemins": abs}, &rep); err != nil {
		return err
	}
	return systeme.OuvrirURL(rep.URL)
}

// ---------- commandes ----------

func ouvrirRegistre() (*config.Config, *appareils.Registre, error) {
	cfg, err := config.Charger("")
	if err != nil {
		return nil, nil, err
	}
	return cfg, appareils.Ouvrir(cfg.Donnees), nil
}

func barre(fait, total int64, debut time.Time) {
	pct := 100.0
	if total > 0 {
		pct = 100 * float64(fait) / float64(total)
	}
	debit := 8 * float64(fait) / time.Since(debut).Seconds() / 1e6
	fmt.Printf("\r  %5.1f %%  %7.0f Mbit/s   ", pct, debit)
}

func envoyer(args []string) error {
	fs := flag.NewFlagSet("envoyer", flag.ExitOnError)
	par := fs.Int("paralleles", 0, "blocs en vol")
	_ = fs.Parse(reordonner(args))
	if fs.NArg() < 2 {
		return errors.New("usage : envoyer CIBLE FICHIERS...")
	}
	cfg, reg, err := ouvrirRegistre()
	if err != nil {
		return err
	}
	ctl, _ := lireControle(cfg)
	o := client.Options{Paralleles: cfg.Paralleles, Source: ctl.source()}
	if *par > 0 {
		o.Paralleles = *par
	}
	var total int64
	for _, c := range fs.Args()[1:] {
		st, err := os.Stat(c)
		if err != nil {
			return err
		}
		total += st.Size()
	}
	o.AvantEnvoi = func(ip string) bool {
		if !reseau.ViaBluetooth(ip) {
			return true
		}
		fmt.Printf("Attention : le lien passe par le Bluetooth (1 à 2 Mbit/s). Durée estimée : %.0f min.\n", reseau.DureeBluetoothS(total)/60)
		if total < 20e6 {
			return true
		}
		fmt.Print("Continuer quand même ? (o/n) ")
		r, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		return strings.TrimSpace(strings.ToLower(r)) == "o"
	}
	debut := time.Now()
	o.Progres = func(f, t int64) { barre(f, t, debut) }
	bilans, err := client.Envoyer(context.Background(), reg, fs.Arg(0), fs.Args()[1:], o)
	fmt.Println()
	for _, b := range bilans {
		if b.Nom == "" {
			continue
		}
		reprise := ""
		if b.BlocsEnvoyes < b.BlocsTotal {
			reprise = fmt.Sprintf(", reprise : %d/%d blocs", b.BlocsEnvoyes, b.BlocsTotal)
		}
		fmt.Printf("  %s -> %s : %s en %.1f s (%.0f Mbit/s, démarrage %d ms%s)\n", b.Nom, b.NomRecu,
			serveur.TailleLisible(b.Taille), b.Duree.Seconds(), b.Mbits(), b.Demarrage.Milliseconds(), reprise)
	}
	return err
}

// reordonner place les options avant les arguments (flag s'arrête au premier argument).
func reordonner(args []string) []string {
	var opts, reste []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			opts = append(opts, args[i])
			if !strings.Contains(args[i], "=") && i+1 < len(args) {
				opts = append(opts, args[i+1])
				i++
			}
		} else {
			reste = append(reste, args[i])
		}
	}
	return append(opts, reste...)
}

func lien(args []string) error {
	if len(args) != 2 {
		return errors.New("usage : lien CIBLE URL")
	}
	cfg, reg, err := ouvrirRegistre()
	if err != nil {
		return err
	}
	ctl, _ := lireControle(cfg)
	ctx, fin := context.WithTimeout(context.Background(), 20*time.Second)
	defer fin()
	if err := client.EnvoyerLien(ctx, reg, args[0], args[1], ctl.source()); err != nil {
		return err
	}
	fmt.Println("Lien transmis.")
	return nil
}

func listerAppareils() error {
	_, reg, err := ouvrirRegistre()
	if err != nil {
		return err
	}
	l := reg.Liste()
	if len(l) == 0 {
		fmt.Println("Aucun appareil de confiance pour l'instant.")
	}
	for _, a := range l {
		fmt.Printf("  [%-9s] %-26s %s\n", a.Type, a.Nom, a.Adresse)
	}
	return nil
}

func listerAdresses() {
	cfg, _ := config.Charger("")
	l := reseau.Classees()
	if len(l) == 0 {
		fmt.Println("Aucune adresse réseau : ce PC n'est connecté à aucun réseau.")
	}
	for _, i := range l {
		fmt.Printf("  %-16s %s  [%s, %d Mbit/s]\n", i.IP, i.Reseau, i.Nom, i.Mbits)
	}
	if cfg != nil {
		fmt.Printf("\nNom Bonjour : %s.local · port %d\n", cfg.NomMdns(), cfg.Port)
	}
}

func identiteLocale(cfg *config.Config) (*identite.Identite, error) {
	return identite.Ouvrir(cfg.Donnees, cfg.Nom, []string{cfg.NomMdns() + ".local"}, func() []string { return reseau.IPs(false) })
}

func empreinte() error {
	cfg, err := config.Charger("")
	if err != nil {
		return err
	}
	id, err := identiteLocale(cfg)
	if err != nil {
		return err
	}
	fmt.Println("Empreinte SHA-256 du certificat de ce PC :\n ", identite.Lisible(id.Empreinte))
	return nil
}

func nommer(args []string) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return errors.New(`usage : nommer "PC de Camil"`)
	}
	cfg, err := config.Charger("")
	if err != nil {
		return err
	}
	cfg.Nom = strings.TrimSpace(args[0])
	if err := cfg.Sauver(); err != nil {
		return err
	}
	fmt.Printf("Ce PC s'appelle « %s » (%s.local). Relance l'application pour l'annoncer.\n", cfg.Nom, cfg.NomMdns())
	return nil
}

func lireOuiNon(question string) bool {
	fmt.Print(question + " (o/n) ")
	r, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(strings.ToLower(r)) == "o"
}

func appairageOuvrir() error {
	cfg, err := config.Charger("")
	if err != nil {
		return err
	}
	ctl, err := lireControle(cfg)
	if err != nil {
		return err
	}
	if err := ctl.appeler("POST", "/controle/appairage/ouvrir", map[string]any{}, nil); err != nil {
		return err
	}
	fmt.Println("En attente d'une demande (2 minutes). Sur l'autre PC : appairer <adresse de ce PC>")
	for _, i := range reseau.Classees() {
		fmt.Printf("   %s   (%s)\n", i.IP, i.Reseau)
	}
	for {
		time.Sleep(500 * time.Millisecond)
		var e map[string]any
		if err := ctl.appeler("GET", "/controle/appairage", nil, &e); err != nil {
			return err
		}
		switch e["etat"] {
		case "a_confirmer":
			code, _ := e["code"].(string)
			fmt.Printf("\nDemande de « %v » depuis %v\n\n   CODE : %s %s\n\n", e["nom"], e["ip"], code[:3], code[3:])
			ok := lireOuiNon("Le même code s'affiche-t-il sur l'autre PC ?")
			if err := ctl.appeler("POST", "/controle/appairage/decision", map[string]bool{"accepter": ok}, nil); err != nil {
				return err
			}
			if !ok {
				return errors.New("appairage refusé")
			}
			fmt.Println("Appairage accepté.")
			return nil
		case "expire", "ferme", "refuse":
			return errors.New("fenêtre fermée sans appairage")
		}
	}
}

func appairer(args []string) error {
	if len(args) != 1 {
		return errors.New("usage : appairer ADRESSE[:PORT]")
	}
	cfg, reg, err := ouvrirRegistre()
	if err != nil {
		return err
	}
	id, err := identiteLocale(cfg)
	if err != nil {
		return err
	}
	hote, port := args[0], cfg.Port
	if h, p, err := net.SplitHostPort(args[0]); err == nil {
		hote = h
		if port, err = strconv.Atoi(p); err != nil {
			return err
		}
	}
	nom, _, err := client.Appairer(context.Background(), client.Moi{Nom: cfg.Nom, Port: cfg.Port, ID: id, Reg: reg}, hote, port,
		func(code, nomB, fpB string) bool {
			fmt.Printf("Pair : « %s »\nEmpreinte : %s\n\n   CODE : %s %s\n\n", nomB, fpB, code[:3], code[3:])
			return lireOuiNon("Le même code s'affiche-t-il sur l'autre PC ?")
		})
	if err != nil {
		return err
	}
	fmt.Printf("Appairé avec « %s ».\n", nom)
	return nil
}

func revoquer(args []string) error {
	if len(args) != 1 {
		return errors.New("usage : revoquer NOM")
	}
	_, reg, err := ouvrirRegistre()
	if err != nil {
		return err
	}
	n, err := reg.Revoquer(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("%d entrée(s) retirée(s).\n", n)
	return nil
}

// ---------- mesure de débit ----------

func portLibre() int {
	for {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			continue
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if l2, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", p+1)); err == nil {
			l2.Close()
			return p
		}
	}
}

func fichierAleatoire(dossier, nom string, n int64) (string, error) {
	p := filepath.Join(dossier, nom)
	f, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = io.CopyN(f, rand.Reader, n)
	return p, err
}

func bench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	local := fs.Bool("local", false, "deux processus sur ce PC : plafond du logiciel seul")
	cible := fs.String("cible", "", "PC appairé à mesurer sur le vrai réseau")
	mo := fs.Int64("mo", 300, "taille du fichier de test en Mo")
	iperf := fs.Float64("iperf", 0, "débit TCP brut iperf3 sur le même lien, en Mbit/s")
	_ = fs.Parse(args)
	tmp, err := os.MkdirTemp("", "ua-bench-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var reg *appareils.Registre
	var src client.Source
	nomCible := *cible
	if *local {
		port := portLibre()
		donneesB := filepath.Join(tmp, "B")
		if err := config.EcrireJSON(filepath.Join(donneesB, "config.json"), map[string]any{"nom": "BENCH-B", "port": port,
			"reception": filepath.Join(tmp, "B-recus"), "decouverte": false, "mdns": false, "ouvrir_liens": false}, false); err != nil {
			return err
		}
		cmd := exec.Command(systeme.Executable(), "demon")
		cmd.Env = append(os.Environ(), "UA_DONNEES="+donneesB)
		if err := cmd.Start(); err != nil {
			return err
		}
		defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
		cfgB := &config.Config{Donnees: donneesB}
		var ctl *controle
		for range 100 {
			time.Sleep(100 * time.Millisecond)
			if ctl, err = lireControle(cfgB); err == nil && ctl.actif() {
				break
			}
		}
		if ctl == nil {
			return errors.New("le démon de test n'a pas démarré")
		}
		cfgA := &config.Config{Donnees: filepath.Join(tmp, "A"), Nom: "BENCH-A", Port: config.PortDefaut}
		_ = os.MkdirAll(cfgA.Donnees, 0o700)
		idA, err := identite.Ouvrir(cfgA.Donnees, "BENCH-A", nil, nil)
		if err != nil {
			return err
		}
		reg = appareils.Ouvrir(cfgA.Donnees)
		if err := ctl.appeler("POST", "/controle/appairage/ouvrir", map[string]any{}, nil); err != nil {
			return err
		}
		if _, _, err := client.Appairer(context.Background(), client.Moi{Nom: "BENCH-A", Port: 1, ID: idA, Reg: reg}, "127.0.0.1", port,
			func(string, string, string) bool {
				return ctl.appeler("POST", "/controle/appairage/decision", map[string]bool{"accepter": true}, nil) == nil
			}); err != nil {
			return err
		}
		nomCible = "BENCH-B"
	} else {
		if nomCible == "" {
			return errors.New("usage : bench --local, ou bench --cible NOM_DU_PC")
		}
		cfg, r, err := ouvrirRegistre()
		if err != nil {
			return err
		}
		reg = r
		ctl, _ := lireControle(cfg)
		src = ctl.source()
	}
	gros, err := fichierAleatoire(tmp, "bench.bin", *mo<<20)
	if err != nil {
		return err
	}
	var photos []string
	for i := range 40 {
		p, err := fichierAleatoire(tmp, fmt.Sprintf("photo_%02d.heic", i), 3<<20)
		if err != nil {
			return err
		}
		photos = append(photos, p)
	}
	fmt.Printf("\n%6s %9s %11s %14s %6s\n", "bloc", "parallèle", "démarrage", "débit", "η")
	for _, e := range []struct {
		bloc int64
		par  int
	}{{1 << 20, 1}, {4 << 20, 1}, {4 << 20, 4}, {8 << 20, 4}} {
		b, err := client.Envoyer(context.Background(), reg, nomCible, []string{gros}, client.Options{Paralleles: e.par, TailleBloc: e.bloc, Source: src})
		if err != nil {
			return err
		}
		eta := "-"
		if *iperf > 0 {
			eta = fmt.Sprintf("%.2f", b[0].Mbits() / *iperf)
		}
		fmt.Printf("%4dMo %9d %9dms %10.0fMbit/s %6s\n", e.bloc>>20, e.par, b[0].Demarrage.Milliseconds(), b[0].Mbits(), eta)
	}
	t0 := time.Now()
	if _, err := client.Envoyer(context.Background(), reg, nomCible, photos, client.Options{Source: src}); err != nil {
		return err
	}
	d := time.Since(t0).Seconds()
	fmt.Printf("\n40 photos de 3 Mo : %.2f s, soit %.0f Mbit/s\n", d, 8*40*3*float64(1<<20)/d/1e6)
	if !*local {
		fmt.Println("Pense à supprimer les fichiers bench*.bin et photo_*.heic reçus sur l'autre PC.")
	}
	return nil
}
