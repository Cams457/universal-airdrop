package boite

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBoite(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(t.TempDir(), "photo.jpg")
	os.WriteFile(src, []byte("jpeg"), 0o644)
	b := Ouvrir(d)
	e, err := b.DeposerFichier("iPhone de Camil", "PC fixe", src, false)
	if err != nil {
		t.Fatal(err)
	}
	b.DeposerTexte("IPHONE DE CAMIL", "PC fixe", "https://exemple.fr", true)
	if _, err := os.Stat(src); err != nil {
		t.Fatal("la source ne doit pas être déplacée sans le demander")
	}
	// rechargement depuis le disque : même nom, casse différente
	b2 := Ouvrir(d)
	l := b2.Lister("iphone de camil")
	if len(l) != 2 || l[0].Type != "fichier" || l[1].Type != "lien" {
		t.Fatalf("boîte rechargée : %+v", l)
	}
	_, chemin, ok := b2.Fichier("iPhone de Camil", e.ID)
	if got, _ := os.ReadFile(chemin); !ok || string(got) != "jpeg" {
		t.Fatal("fichier en attente illisible")
	}
	b2.Retirer("iPhone de Camil", e.ID)
	if _, err := os.Stat(chemin); err == nil {
		t.Fatal("fichier récupéré non supprimé")
	}
	if b2.Nombre("iPhone de Camil") != 1 {
		t.Fatal("compte faux")
	}
	// expiration
	b2.mu.Lock()
	b2.b["iphone de camil"][0].Cree = time.Now().Add(-Duree - time.Hour)
	b2.mu.Unlock()
	b2.Nettoyer()
	if b2.Nombre("iPhone de Camil") != 0 {
		t.Fatal("élément expiré conservé")
	}
}

// Ce que le téléphone a déjà récupéré n'est plus « en attente » ; l'annulation ne retire que le reste.
func TestEnAttente(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(t.TempDir(), "a.pdf")
	os.WriteFile(src, []byte("pdf"), 0o644)
	b := Ouvrir(d)
	f, _ := b.DeposerFichier("iPhone", "PC", src, false)
	x := b.DeposerTexte("iPhone", "PC", "bonjour", false)
	b.DeposerTexte("iPhone", "PC", "pas encore lu", false)
	if n := b.EnAttente("iPhone"); n != 3 {
		t.Fatalf("en attente : %d, attendu 3", n)
	}
	b.Remettre("iphone", f.ID, x.ID)
	if n := Ouvrir(d).EnAttente("iPhone"); n != 1 {
		t.Fatalf("après remise (rechargé) : %d, attendu 1", n)
	}
	if n := b.AnnulerEnAttente("iPhone"); n != 1 {
		t.Fatalf("annulés : %d, attendu 1", n)
	}
	if l := b.Lister("iPhone"); len(l) != 2 || b.EnAttente("iPhone") != 0 {
		t.Fatalf("après annulation : %+v", l)
	}
}
