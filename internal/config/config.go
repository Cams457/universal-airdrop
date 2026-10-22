// Package config gère les dossiers, ports et réglages, stockés dans config.json.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"unicode"
)

const (
	PortDefaut        = 47800 // serveur HTTPS sur le réseau local
	DecalageControle  = 1     // interface locale sur 127.0.0.1:(port + 1)
	GroupeMulticast   = "239.255.47.80"
	PortMulticast     = 47802
	TailleBloc        = 4 << 20 // 4 Mio
	BlocsEnVol        = 4
	FichiersEnVol     = 4
	DureePartageS     = 600 // lien QR valable 10 min
	DureeAppairageS   = 120
	DureeEnvoiMobileS = 900 // page d'envoi pour téléphone : 15 min
)

// Config regroupe les réglages modifiables par l'utilisateur.
type Config struct {
	Donnees       string `json:"-"`
	Nom           string `json:"nom"`
	Port          int    `json:"port"`
	Reception     string `json:"reception"`
	OuvrirLiens   bool   `json:"ouvrir_liens"`
	Paralleles    int    `json:"paralleles"`
	Decouverte    bool   `json:"decouverte"`
	Mdns          bool   `json:"mdns"`
	Configure     bool   `json:"configure"` // assistant de premier lancement terminé
	LienRaccourci string `json:"lien_raccourci"`
}

// DossierDonnees : %APPDATA%\UniversalAirdrop sous Windows, ~/.config/universal-airdrop ailleurs.
func DossierDonnees() string {
	if d := os.Getenv("UA_DONNEES"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		if a := os.Getenv("APPDATA"); a != "" {
			return filepath.Join(a, "UniversalAirdrop")
		}
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "universal-airdrop")
	}
	return filepath.Join(os.TempDir(), "universal-airdrop")
}

func receptionDefaut() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, "Downloads", "Universal Airdrop")
	}
	return filepath.Join(os.TempDir(), "Universal Airdrop")
}

func nomDefaut() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "Mon PC"
}

// Charger lit config.json (ou le crée avec les valeurs par défaut).
func Charger(donnees string) (*Config, error) {
	if donnees == "" {
		donnees = DossierDonnees()
	}
	if err := os.MkdirAll(donnees, 0o700); err != nil {
		return nil, err
	}
	c := &Config{Donnees: donnees, Nom: nomDefaut(), Port: PortDefaut, OuvrirLiens: true,
		Paralleles: BlocsEnVol, Decouverte: true, Mdns: true}
	brut, err := os.ReadFile(filepath.Join(donnees, "config.json"))
	if err == nil {
		if err := json.Unmarshal(brut, c); err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		if err := c.Sauver(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if c.Paralleles < 1 {
		c.Paralleles = BlocsEnVol
	}
	return c, nil
}

func (c *Config) Sauver() error { return EcrireJSON(filepath.Join(c.Donnees, "config.json"), c, false) }

func (c *Config) DossierReception() string {
	if c.Reception != "" {
		return c.Reception
	}
	return receptionDefaut()
}

func (c *Config) PortControle() int { return c.Port + DecalageControle }

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// NomMdns : « PC de Camil » -> « pc-de-camil » (minuscules, sans accents, règles DNS).
func NomMdns(nom string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	s, _, _ := transform.String(t, nom)
	s = strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if s == "" {
		s = "pc-airdrop"
	}
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

func (c *Config) NomMdns() string { return NomMdns(c.Nom) }

// EcrireJSON écrit de façon atomique : fichier temporaire puis remplacement.
func EcrireJSON(chemin string, v any, secret bool) error {
	if err := os.MkdirAll(filepath.Dir(chemin), 0o700); err != nil {
		return err
	}
	brut, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if secret {
		mode = 0o600
	}
	tmp := chemin + ".tmp"
	if err := os.WriteFile(tmp, brut, mode); err != nil {
		return err
	}
	return os.Rename(tmp, chemin)
}

// LireJSON lit un fichier JSON ; renvoie false s'il n'existe pas.
func LireJSON(chemin string, v any) (bool, error) {
	brut, err := os.ReadFile(chemin)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(brut, v)
}
