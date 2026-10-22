// Package systeme : intégration au système d'exploitation (ouvrir un lien ou un dossier, pare-feu,
// démarrage automatique, entrée « Envoyer vers » du clic droit).
package systeme

import "os"

// Executable : chemin absolu du programme en cours.
func Executable() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// Etat de l'intégration, pour l'interface.
type Etat struct {
	PareFeu     bool `json:"pare_feu"`
	Demarrage   bool `json:"demarrage"`
	EnvoyerVers bool `json:"envoyer_vers"`
	Pris        bool `json:"pris_en_charge"` // false hors Windows
}
