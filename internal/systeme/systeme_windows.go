//go:build windows

package systeme

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	nomRegle         = "Universal Airdrop"
	cleDemarrage     = `Software\Microsoft\Windows\CurrentVersion\Run`
	valDemarrage     = "UniversalAirdrop"
	creerSansFenetre = 0x08000000 // CREATE_NO_WINDOW
)

func sansFenetre(c *exec.Cmd) *exec.Cmd {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: creerSansFenetre}
	return c
}

// OuvrirURL ouvre un lien http(s) déjà validé dans le navigateur par défaut, sans passer par un shell.
func OuvrirURL(u string) error {
	return sansFenetre(exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", u)).Start()
}

// OuvrirDossier ouvre un dossier dans l'Explorateur.
func OuvrirDossier(p string) error { return exec.Command("explorer.exe", p).Start() }

// OuvrirFichier montre le fichier sélectionné dans l'Explorateur.
func OuvrirFichier(p string) error { return exec.Command("explorer.exe", "/select,", p).Start() }

// PortService : port TCP du service (fixé au démarrage), couvert par la règle « par port ».
var PortService = 47800

func regleTCP() string { return fmt.Sprintf("%s (TCP %d)", nomRegle, PortService) }

const regleUDP = nomRegle + " (UDP decouverte)" // ASCII : passe par cmd en administrateur

func montrerRegle(nom string) string {
	sortie, err := sansFenetre(exec.Command("netsh", "advfirewall", "firewall", "show", "rule",
		"name="+nom, "verbose")).Output()
	if err != nil {
		return ""
	}
	return strings.ToLower(string(sortie))
}

// PareFeuPresent : les autres appareils peuvent-ils joindre ce PC ? Il suffit de la règle « par
// port » (indépendante de l'emplacement de l'exe) ou de la règle « par programme » pour CET exe ;
// dans les deux cas, elle doit inclure la plage Tailscale (sinon : règle d'une ancienne version).
func PareFeuPresent() bool {
	if t := montrerRegle(regleTCP()); strings.Contains(t, "100.64.0.0") {
		return true
	}
	t := montrerRegle(nomRegle)
	return strings.Contains(t, strings.ToLower(Executable())) && strings.Contains(t, "100.64.0.0")
}

// AjouterPareFeu ajoute les règles avec élévation (UAC) : Windows affiche « Voulez-vous autoriser… ».
//   - par programme (CET exe), tous protocoles ;
//   - par port (TCP du service, UDP 47802 et 5353 pour la découverte et Bonjour), qui reste valable
//     si l'exe est renommé ou déplacé (« UniversalAirdrop (1).exe » dans Téléchargements…).
//
// Tous profils (l'interface d'un partage de connexion est souvent « Public »), mais UNIQUEMENT
// depuis les sous-réseaux directement connectés et Tailscale (remoteip=localsubnet,100.64.0.0/10).
func AjouterPareFeu() error {
	exe := Executable()
	// La commande s'exécute en administrateur : un chemin contenant un caractère spécial de cmd
	// (& | ^ < > " %) permettrait d'injecter une autre commande. On refuse plutôt que d'échapper.
	if exe == "" || strings.ContainsAny(exe, "&|^<>\"%!") {
		return fmt.Errorf("chemin du programme non pris en charge pour la règle de pare-feu : %s", exe)
	}
	const distants = "remoteip=localsubnet,100.64.0.0/10"
	nf := "netsh advfirewall firewall "
	args := "/c " + strings.Join([]string{
		nf + fmt.Sprintf(`delete rule name="%s"`, nomRegle),
		nf + fmt.Sprintf(`delete rule name="%s"`, regleTCP()),
		nf + fmt.Sprintf(`delete rule name="%s"`, regleUDP),
		nf + fmt.Sprintf(`add rule name="%s" dir=in action=allow program="%s" enable=yes profile=any %s`, nomRegle, exe, distants),
		nf + fmt.Sprintf(`add rule name="%s" dir=in action=allow protocol=TCP localport=%d enable=yes profile=any %s`, regleTCP(), PortService, distants),
		nf + fmt.Sprintf(`add rule name="%s" dir=in action=allow protocol=UDP localport=47802,5353 enable=yes profile=any %s`, regleUDP, distants),
	}, " & ")
	verbe, _ := windows.UTF16PtrFromString("runas")
	prog, _ := windows.UTF16PtrFromString("cmd.exe")
	params, _ := windows.UTF16PtrFromString(args)
	return windows.ShellExecute(0, verbe, prog, params, nil, windows.SW_HIDE)
}

// DemarrageActif : lancement automatique à l'ouverture de session (sans droits administrateur).
func DemarrageActif() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, cleDemarrage, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(valDemarrage)
	return err == nil && strings.Contains(strings.ToLower(v), strings.ToLower(Executable()))
}

func Demarrage(actif bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, cleDemarrage, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !actif {
		if err := k.DeleteValue(valDemarrage); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	return k.SetStringValue(valDemarrage, fmt.Sprintf(`"%s" --arriere-plan`, Executable()))
}

func cheminEnvoyerVers() string {
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "SendTo", "Universal Airdrop.lnk")
}

func EnvoyerVersActif() bool {
	_, err := os.Stat(cheminEnvoyerVers())
	return err == nil
}

// EnvoyerVers crée (ou retire) le raccourci dans le menu clic droit > Envoyer vers.
func EnvoyerVers(actif bool) error {
	c := cheminEnvoyerVers()
	if !actif {
		if err := os.Remove(c); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe := strings.ReplaceAll(Executable(), "'", "''")
	lnk := strings.ReplaceAll(c, "'", "''")
	script := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut('%s');$s.TargetPath='%s';$s.Arguments='envoyer-vers';$s.IconLocation='%s,0';$s.Save()`, lnk, exe, exe)
	return sansFenetre(exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)).Run()
}

// MettreAJourChemins : une nouvelle version lancée depuis un autre dossier (téléchargement) devient
// la cible du démarrage automatique et d'« Envoyer vers » ; sinon l'ancien exe reviendrait au
// prochain démarrage de Windows. Le dernier chemin connu est noté dans dossier\exe.txt, pour ne
// rien refaire quand il n'a pas changé.
func MettreAJourChemins(dossier string) {
	exe := Executable()
	note := filepath.Join(dossier, "exe.txt")
	if b, err := os.ReadFile(note); err == nil && strings.EqualFold(strings.TrimSpace(string(b)), exe) {
		return
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, cleDemarrage, registry.QUERY_VALUE); err == nil {
		v, _, err := k.GetStringValue(valDemarrage)
		k.Close()
		if err == nil && v != "" && !strings.Contains(strings.ToLower(v), strings.ToLower(exe)) {
			_ = Demarrage(true)
		}
	}
	if EnvoyerVersActif() {
		_ = EnvoyerVers(true)
	}
	_ = os.WriteFile(note, []byte(exe), 0o600)
}

func LireEtat() Etat {
	return Etat{PareFeu: PareFeuPresent(), Demarrage: DemarrageActif(), EnvoyerVers: EnvoyerVersActif(), Pris: true}
}
