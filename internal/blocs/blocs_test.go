package blocs

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

const B = 64 << 10

func TestRepriseApresRedemarrageEtBlocPerdu(t *testing.T) {
	d := t.TempDir()
	src := make([]byte, 5*B+100)
	rand.Read(src)
	chemin := filepath.Join(d, "src.bin")
	os.WriteFile(chemin, src, 0o644)
	f, _ := os.Open(chemin)
	defer f.Close()
	taille := int64(len(src))
	n := NombreBlocs(taille, B)
	hach := make([]string, n)
	donnees := make([][]byte, n)
	for i := int64(0); i < n; i++ {
		b, h, err := LireEtHacher(f, i, taille, B, make([]byte, B))
		if err != nil {
			t.Fatal(err)
		}
		donnees[i], hach[i] = append([]byte(nil), b...), h
	}
	tmp := filepath.Join(d, "tmp")
	r, err := Ouvrir(tmp, "id1", "src.bin", taille, B, "pc:A")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if raison := r.EcrireBloc(int64(i), donnees[i], hach[i]); raison != "" {
			t.Fatal(raison)
		}
	}
	if raison := r.EcrireBloc(4, append([]byte{'x'}, donnees[4][1:]...), hach[4]); raison == "" {
		t.Fatal("bloc corrompu accepté")
	}
	r.Sauvegarder()
	r.Fermer()
	// coupure de courant simulée : le bloc 2 noté n'a jamais atteint le disque
	p, _ := os.OpenFile(filepath.Join(tmp, "id1.part"), os.O_RDWR, 0)
	p.WriteAt(make([]byte, B), 2*B)
	p.Close()

	r2, _ := Ouvrir(tmp, "id1", "src.bin", taille, B, "pc:A")
	m := r2.Manquants()
	if len(m) != 3 || m[0] != 2 || m[1] != 4 || m[2] != 5 {
		t.Fatalf("manquants = %v, attendu [2 4 5]", m)
	}
	for _, i := range m {
		r2.EcrireBloc(i, donnees[i], hach[i])
	}
	final, raison := r2.Finaliser(filepath.Join(d, "recus"), Racine(hach))
	if raison != "" {
		t.Fatal(raison)
	}
	if got, _ := os.ReadFile(final); !bytes.Equal(got, src) {
		t.Fatal("contenu différent")
	}
}

func TestEtatIncompatible(t *testing.T) {
	tmp := t.TempDir()
	r, _ := Ouvrir(tmp, "id2", "a.bin", 3*B, B, "pc:A")
	r.Sauvegarder()
	r.Fermer()
	autre, _ := Ouvrir(tmp, "id2", "a.bin", 3*B, B, "pc:AUTRE")
	if len(autre.Manquants()) != 3 {
		t.Fatal("un autre appareil ne doit pas reprendre cette réception")
	}
	autre.Abandonner()
}

func TestFichierVide(t *testing.T) {
	d := t.TempDir()
	r, _ := Ouvrir(filepath.Join(d, "tmp"), "v", "vide.txt", 0, B, "x")
	final, raison := r.Finaliser(d, Racine(nil))
	if raison != "" {
		t.Fatal(raison)
	}
	if st, _ := os.Stat(final); st.Size() != 0 {
		t.Fatal("fichier vide attendu")
	}
}
