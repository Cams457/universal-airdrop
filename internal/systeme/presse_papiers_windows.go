//go:build windows

package systeme

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	pOpenClipboard    = user32.NewProc("OpenClipboard")
	pCloseClipboard   = user32.NewProc("CloseClipboard")
	pEmptyClipboard   = user32.NewProc("EmptyClipboard")
	pSetClipboardData = user32.NewProc("SetClipboardData")
	pGetClipboardData = user32.NewProc("GetClipboardData")
	pGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	pGlobalFree       = kernel32.NewProc("GlobalFree")
	pGlobalLock       = kernel32.NewProc("GlobalLock")
	pGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	pGlobalSize       = kernel32.NewProc("GlobalSize")
	pMoveMemory       = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfUnicodeText = 13     // CF_UNICODETEXT
	gmemMoveable  = 0x0002 // GMEM_MOVEABLE
)

// ouvrirPressePapiers : le presse-papiers est un verrou global ; une autre application peut le
// tenir quelques millisecondes, d'où les essais répétés.
func ouvrirPressePapiers() error {
	for i := 0; i < 40; i++ {
		if r, _, _ := pOpenClipboard.Call(0); r != 0 {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return errors.New("presse-papiers occupé par une autre application")
}

// CopierTexte place le texte dans le presse-papiers de Windows (Ctrl+V pour le coller).
func CopierTexte(t string) error {
	runtime.LockOSThread() // OpenClipboard/CloseClipboard doivent venir du même thread
	defer runtime.UnlockOSThread()
	u := utf16.Encode([]rune(strings.ReplaceAll(t, "\x00", "")))
	u = append(u, 0)
	taille := uintptr(len(u) * 2)
	if err := ouvrirPressePapiers(); err != nil {
		return err
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	h, _, err := pGlobalAlloc.Call(gmemMoveable, taille)
	if h == 0 {
		return err
	}
	p, _, err := pGlobalLock.Call(h)
	if p == 0 {
		pGlobalFree.Call(h)
		return err
	}
	pMoveMemory.Call(p, uintptr(unsafe.Pointer(&u[0])), taille)
	pGlobalUnlock.Call(h)
	if r, _, err := pSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		pGlobalFree.Call(h) // sinon le système en devient propriétaire
		return err
	}
	return nil
}

// LireTexte lit le texte du presse-papiers (utilisé par les tests).
func LireTexte() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ouvrirPressePapiers(); err != nil {
		return "", err
	}
	defer pCloseClipboard.Call()
	h, _, err := pGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", err
	}
	p, _, err := pGlobalLock.Call(h)
	if p == 0 {
		return "", err
	}
	defer pGlobalUnlock.Call(h)
	n, _, _ := pGlobalSize.Call(h)
	u := make([]uint16, n/2)
	if len(u) == 0 {
		return "", nil
	}
	pMoveMemory.Call(uintptr(unsafe.Pointer(&u[0])), p, uintptr(len(u)*2))
	for i, c := range u {
		if c == 0 {
			u = u[:i]
			break
		}
	}
	return string(utf16.Decode(u)), nil
}

// scriptNotification affiche une notification Windows. Le titre et le texte passent par des
// variables d'environnement, jamais dans le script : aucune injection possible.
const scriptNotification = `$ErrorActionPreference='Stop'
[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime]
$x=[Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$t=$x.GetElementsByTagName('text')
[void]$t.Item(0).AppendChild($x.CreateTextNode($env:UA_TITRE))
[void]$t.Item(1).AppendChild($x.CreateTextNode($env:UA_TEXTE))
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe').Show([Windows.UI.Notifications.ToastNotification]::new($x))`

// Notifier affiche une notification Windows (en bas à droite de l'écran).
func Notifier(titre, texte string) error {
	c := sansFenetre(exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", scriptNotification))
	c.Env = append(os.Environ(), "UA_TITRE="+titre, "UA_TEXTE="+texte)
	return c.Start()
}
