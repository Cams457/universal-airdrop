//go:build windows

package systeme

import "testing"

func TestPressePapiers(t *testing.T) {
	const texte = "https://exemple.fr/é?a=1&b=2 — ✓"
	if err := CopierTexte(texte); err != nil {
		t.Skipf("presse-papiers indisponible (session sans bureau ?) : %v", err)
	}
	lu, err := LireTexte()
	if err != nil {
		t.Skipf("lecture impossible : %v", err)
	}
	if lu != texte {
		t.Fatalf("lu %q, attendu %q", lu, texte)
	}
}
