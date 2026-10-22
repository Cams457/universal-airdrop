//go:build windows

package main

import (
	_ "embed"
	"os"
	"strings"

	"fyne.io/systray"
	"golang.org/x/sys/windows"

	"github.com/Cams457/universal-airdrop/internal/serveur"
	"github.com/Cams457/universal-airdrop/internal/systeme"
)

//go:embed icone.ico
var iconeICO []byte

// attacherConsole : l'exécutable est une application graphique (pas de fenêtre noire au
// double-clic) ; lancé depuis cmd avec des arguments, il réutilise la console de cmd pour écrire.
func attacherConsole() {
	k := windows.NewLazySystemDLL("kernel32.dll")
	if r, _, _ := k.NewProc("AttachConsole").Call(uintptr(^uint32(0))); r == 0 { // ATTACH_PARENT_PROCESS
		return
	}
	if h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil && h != 0 {
		os.Stdout = os.NewFile(uintptr(h), "stdout")
	}
	if h, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE); err == nil && h != 0 {
		os.Stderr = os.NewFile(uintptr(h), "stderr")
	}
	if h, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE); err == nil && h != 0 {
		os.Stdin = os.NewFile(uintptr(h), "stdin")
	}
}

// icone : l'icône près de l'horloge, avec son menu. Bloque jusqu'à « Quitter ».
func icone(s *serveur.Service) {
	ouvrirUI := func(chemin string) { _ = systeme.OuvrirURL(s.URLInterface(chemin)) }
	serveur.SurQuitter = systray.Quit
	s.SurEvenement = func(e serveur.Evenement) {
		if e.Type == "recu" || e.Type == "lien" {
			t := e.Texte
			if len(t) > 120 {
				t = t[:120] + "…"
			}
			systray.SetTooltip("Universal Airdrop\n" + t)
		}
	}
	systray.Run(func() {
		systray.SetIcon(iconeICO)
		systray.SetTitle("Universal Airdrop")
		systray.SetTooltip("Universal Airdrop · " + strings.TrimSpace(s.Cfg.Nom))
		systray.SetOnTapped(func() { ouvrirUI("") })
		ouvrir := systray.AddMenuItem("Ouvrir Universal Airdrop", "")
		recevoir := systray.AddMenuItem("Recevoir depuis un téléphone…", "Affiche un QR code à scanner")
		dossier := systray.AddMenuItem("Ouvrir le dossier de réception", "")
		guide := systray.AddMenuItem("Guide d'installation…", "Installer, relier l'iPhone, Raccourcis")
		systray.AddSeparator()
		quitter := systray.AddMenuItem("Quitter", "")
		go func() {
			for {
				select {
				case <-ouvrir.ClickedCh:
					ouvrirUI("")
				case <-recevoir.ClickedCh:
					ouvrirUI("?action=recevoir")
				case <-guide.ClickedCh:
					ouvrirUI("?action=guide")
				case <-dossier.ClickedCh:
					_ = os.MkdirAll(s.Cfg.DossierReception(), 0o755)
					_ = systeme.OuvrirDossier(s.Cfg.DossierReception())
				case <-quitter.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})
}
