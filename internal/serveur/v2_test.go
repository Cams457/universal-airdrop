package serveur

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func attendre(t *testing.T, quoi string, cond func() bool) {
	t.Helper()
	for range 150 {
		if cond() {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("délai dépassé : %s", quoi)
}

// v2 : l'iPhone choisit ses destinataires, le PC relaie ; boîte d'envoi des téléphones ;
// relais vers le téléphone d'un autre PC ; ajout d'un iPhone sans QR code.
func TestV2DestinationsRelaisEtBoite(t *testing.T) {
	a, b := creer(t, "PC-A"), creer(t, "PC-B")
	if err := appairerAB(t, a, b, true, true); err != nil {
		t.Fatal(err)
	}
	jIphone, _ := a.Reg.AjouterTelephone("iPhone de Camil")
	jIpad, _ := a.Reg.AjouterTelephone("iPad")
	b.Reg.AjouterTelephone("Tablette")
	var mu sync.Mutex
	var copiesB []string
	b.CopierTexte = func(x string) error { mu.Lock(); copiesB = append(copiesB, x); mu.Unlock(); return nil }

	c := clientEpingle(t, a.ID.CAPEM)
	base := fmt.Sprintf("https://127.0.0.1:%d", a.Cfg.Port)
	iphone := func(extra map[string]string) map[string]string {
		h := map[string]string{"Authorization": "Bearer " + jIphone}
		for k, v := range extra {
			h[k] = v
		}
		return h
	}

	// 1. liste proposée dans le Raccourci : ce PC d'abord, sans l'iPhone lui-même
	_, rep := faire(t, c, "GET", base+"/api/rapide/destinations", iphone(nil), nil)
	var d struct{ Destinations []string }
	json.Unmarshal(rep, &d)
	if strings.Join(d.Destinations, "|") != "PC-A|PC-B|iPad" {
		t.Fatalf("destinations : %v", d.Destinations)
	}

	// 2. photo vers PC-B et l'iPad, pas vers PC-A (en-tête encodé comme le ferait un Raccourci)
	ct, corps := formulaire(t, map[string][]byte{"IMG_0042.HEIC": []byte("photo-heic")})
	code, rep := faire(t, c, "POST", base+"/api/rapide/fichier", iphone(map[string]string{"Content-Type": ct, "X-Destinations": "PC-B%7CiPad"}), corps)
	if code != 200 {
		t.Fatalf("envoi avec destinations : %d %s", code, rep)
	}
	recuB := filepath.Join(b.Cfg.DossierReception(), "IMG_0042.HEIC")
	attendre(t, "relais vers PC-B", func() bool { _, err := os.Stat(recuB); return err == nil })
	if _, err := os.Stat(filepath.Join(a.Cfg.DossierReception(), "IMG_0042.HEIC")); err == nil {
		t.Fatal("PC-A n'était pas destinataire")
	}
	trouve := false
	b.mu.Lock()
	for _, e := range b.evenements {
		trouve = trouve || strings.Contains(e.Texte, "iPhone de Camil (via PC-A)")
	}
	b.mu.Unlock()
	if !trouve {
		t.Fatal("PC-B doit afficher l'appareil d'origine")
	}

	// 3. lien vers PC-B (presse-papiers) et l'iPad (boîte)
	if code, rep := faire(t, c, "POST", base+"/api/rapide/lien", iphone(map[string]string{"Content-Type": "text/plain", "X-Destinations": "PC-B|iPad"}), []byte("https://exemple.fr/v2")); code != 200 {
		t.Fatalf("lien : %d %s", code, rep)
	}
	attendre(t, "lien copié sur PC-B", func() bool { mu.Lock(); defer mu.Unlock(); return len(copiesB) == 1 })

	// 4. l'iPad récupère sa boîte : le lien dans la réponse, la photo par son URL (une seule fois)
	ipad := map[string]string{"Authorization": "Bearer " + jIpad}
	_, rep = faire(t, c, "GET", base+"/api/rapide/boite", ipad, nil)
	var boite struct {
		Nombre   int
		Texte    string
		Fichiers []struct {
			URL, Nom, Destination string
			Album                 bool
		}
	}
	json.Unmarshal(rep, &boite)
	if boite.Nombre != 2 || boite.Texte != "https://exemple.fr/v2" || len(boite.Fichiers) != 1 || !boite.Fichiers[0].Album || boite.Fichiers[0].Destination != "photos" {
		t.Fatalf("boîte de l'iPad : %s", rep)
	}
	u := boite.Fichiers[0].URL // construite à partir de l'hôte de la requête
	if code, contenu := faire(t, c, "GET", u, ipad, nil); code != 200 || string(contenu) != "photo-heic" {
		t.Fatalf("téléchargement : %d", code)
	}
	if code, _ := faire(t, c, "GET", u, ipad, nil); code != 404 {
		t.Fatal("un fichier récupéré doit disparaître")
	}
	if code, _ := faire(t, c, "GET", base+"/api/rapide/boite", map[string]string{"Authorization": "Bearer " + b.Secret}, nil); code != 401 {
		t.Fatal("boîte accessible sans jeton de téléphone")
	}

	// 5. depuis l'interface de PC-A : un fichier pour l'iPhone (boîte locale) et pour la tablette
	// de PC-B (relais), découverte comme téléphone distant
	a.rafraichirDistants(context.Background())
	if dd := a.telephonesDistants(); len(dd) != 1 || dd[0].Nom != "Tablette" || dd[0].Via != "PC-B" {
		t.Fatalf("téléphones distants : %+v", dd)
	}
	h := &http.Client{Transport: &http.Transport{Proxy: nil}}
	url := fmt.Sprintf("http://127.0.0.1:%d", a.Cfg.PortControle())
	ok := map[string]string{"X-Controle": a.Secret}
	ct, corps = formulaire(t, map[string][]byte{"cours.pdf": []byte("%PDF")})
	_, rep = faire(t, h, "POST", url+"/controle/televerser", map[string]string{"X-Controle": a.Secret, "Content-Type": ct}, corps)
	var sel struct{ Selection string }
	json.Unmarshal(rep, &sel)
	j, _ := json.Marshal(map[string]any{"cibles": []string{"iPhone de Camil"}, "distants": []distant{{Nom: "Tablette", Via: "PC-B"}}, "selection": sel.Selection})
	if code, rep := faire(t, h, "POST", url+"/controle/envoyer", ok, j); code != 200 {
		t.Fatalf("envoi interface : %d %s", code, rep)
	}
	if a.Boites.Nombre("iPhone de Camil") != 1 {
		t.Fatal("rien en attente pour l'iPhone")
	}
	attendre(t, "relais vers la tablette de PC-B", func() bool { return b.Boites.Nombre("Tablette") == 1 })
	if l := b.Boites.Lister("Tablette"); l[0].Nom != "cours.pdf" {
		t.Fatalf("boîte de la tablette : %+v", l)
	}

	// 6. retirer l'iPad vide sa boîte
	a.Boites.DeposerTexte("iPad", "test", "x", false)
	jr, _ := json.Marshal(map[string]string{"nom": "iPad"})
	faire(t, h, "POST", url+"/controle/revoquer", ok, jr)
	if a.Boites.Nombre("iPad") != 0 {
		t.Fatal("boîte d'un appareil retiré conservée")
	}
}

func TestAjoutIphoneSansQR(t *testing.T) {
	a := creer(t, "PC-A")
	c := clientEpingle(t, a.ID.CAPEM)
	base := fmt.Sprintf("https://127.0.0.1:%d", a.Cfg.Port)
	if code, _ := faire(t, c, "POST", base+"/api/v1/telephone/demande", nil, []byte(`{"nom":"a|b"}`)); code != 400 {
		t.Fatal("nom avec séparateur accepté")
	}
	_, rep := faire(t, c, "POST", base+"/api/v1/telephone/demande", nil, []byte(`{"nom":"iPhone 15"}`))
	var dem struct{ ID, Code string }
	json.Unmarshal(rep, &dem)
	if len(dem.Code) != 6 || dem.ID == "" {
		t.Fatalf("demande : %s", rep)
	}
	if _, rep := faire(t, c, "GET", base+"/api/v1/telephone/demande/"+dem.ID, nil, nil); !strings.Contains(string(rep), `"attente"`) || strings.Contains(string(rep), "jeton") {
		t.Fatalf("aucun jeton avant acceptation : %s", rep)
	}
	l := a.demandesEnAttente()
	if len(l) != 1 || l[0].Code != dem.Code || l[0].Nom != "iPhone 15" {
		t.Fatalf("demandes côté PC : %+v", l)
	}
	h := &http.Client{Transport: &http.Transport{Proxy: nil}}
	j, _ := json.Marshal(map[string]any{"id": l[0].Court, "accepter": true})
	if code, rep := faire(t, h, "POST", fmt.Sprintf("http://127.0.0.1:%d/controle/telephone/demande", a.Cfg.PortControle()), map[string]string{"X-Controle": a.Secret}, j); code != 200 {
		t.Fatalf("acceptation : %d %s", code, rep)
	}
	_, rep = faire(t, c, "GET", base+"/api/v1/telephone/demande/"+dem.ID, nil, nil)
	var fin struct{ Etat, Jeton, Adresse string }
	json.Unmarshal(rep, &fin)
	if fin.Etat != "acceptee" || fin.Jeton == "" || !strings.Contains(fin.Adresse, ".local:") {
		t.Fatalf("après acceptation : %s", rep)
	}
	if acc, ok := a.Reg.Verifier(fin.Jeton); !ok || acc.Nom != "iPhone 15" || acc.Type != "telephone" {
		t.Fatal("jeton remis invalide")
	}
	if code, _ := faire(t, c, "GET", base+"/api/v1/telephone/demande/"+dem.ID, nil, nil); code != 404 {
		t.Fatal("le jeton ne doit être remis qu'une fois")
	}
}
