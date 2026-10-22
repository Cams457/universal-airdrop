//go:build !windows

package systeme

import (
	"errors"
	"os/exec"
	"runtime"
)

var errNonPris = errors.New("disponible seulement sous Windows")

func ouvrir(cible string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", cible).Start()
	}
	return exec.Command("xdg-open", cible).Start()
}

func OuvrirURL(u string) error     { return ouvrir(u) }
func OuvrirDossier(p string) error { return ouvrir(p) }
func OuvrirFichier(p string) error { return ouvrir(p) }
func PareFeuPresent() bool         { return true }
func AjouterPareFeu() error        { return errNonPris }
func DemarrageActif() bool         { return false }
func Demarrage(bool) error         { return errNonPris }
func EnvoyerVersActif() bool       { return false }
func EnvoyerVers(bool) error       { return errNonPris }
func MettreAJourChemins(string)   {}
func LireEtat() Etat               { return Etat{PareFeu: true, Pris: false} }

// CopierTexte : non pris en charge hors Windows.
func CopierTexte(string) error { return errNonPris }

// Notifier : sans effet hors Windows.
func Notifier(string, string) error { return nil }

// PortService : sans effet hors Windows (pas de règle de pare-feu gérée).
var PortService = 47800
