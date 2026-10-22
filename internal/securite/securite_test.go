package securite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNomSur(t *testing.T) {
	cas := map[string]string{
		"../../windows/system32/evil.dll": "evil.dll",
		`C:\Users\x\photo.jpg`:            "photo.jpg",
		"CON.txt":                         "_CON.txt",
		"a<b>c?.pdf":                      "a_b_c_.pdf",
		"...":                             "fichier",
		"":                                "fichier",
	}
	for entree, attendu := range cas {
		if got := NomSur(entree); got != attendu {
			t.Errorf("NomSur(%q) = %q, attendu %q", entree, got, attendu)
		}
	}
	long := NomSur(strings.Repeat("é", 500) + ".mp4")
	if utf8.RuneCountInString(long) > 200 || !strings.HasSuffix(long, ".mp4") {
		t.Errorf("nom long mal tronqué : %d caractères", utf8.RuneCountInString(long))
	}
}

func TestCheminLibre(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "a.txt"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(d, "a (1).txt"), []byte("2"), 0o644)
	if c, _ := CheminLibre(d, "a.txt"); filepath.Base(c) != "a (2).txt" {
		t.Errorf("obtenu %s", c)
	}
}

func TestIPAutorisee(t *testing.T) {
	for _, ip := range []string{"192.168.1.10", "10.0.0.2", "172.20.10.3", "192.168.137.1", "127.0.0.1", "::1", "fe80::1", "192.168.1.2:5000", "100.101.102.103", "100.64.0.1"} {
		if !IPAutorisee(ip) {
			t.Errorf("%s devrait être autorisée", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2001:4860::8888", "", "pas-une-ip", "100.128.0.1", "100.63.255.255"} {
		if IPAutorisee(ip) {
			t.Errorf("%s devrait être refusée", ip)
		}
	}
}

func TestURLAutorisee(t *testing.T) {
	if !URLAutorisee("https://exemple.fr/page?a=1") || !URLAutorisee("http://192.168.1.5:8080/") {
		t.Fatal("liens http(s) valides refusés")
	}
	for _, u := range []string{"javascript:alert(1)", "file:///C:/Windows", "ms-settings:privacy", "https://", "", "https://a.fr/\x00", "data:text/html,x"} {
		if URLAutorisee(u) {
			t.Errorf("%q devrait être refusé", u)
		}
	}
}

func TestEngagementEtCode(t *testing.T) {
	fa, fb := strings.Repeat("a", 64), strings.Repeat("b", 64)
	ra, rb, rb2 := Alea(32), Alea(32), Alea(32)
	if Engagement(fb, fa, rb) == Engagement(fb, fa, rb2) {
		t.Fatal("l'engagement doit dépendre de r_B")
	}
	code := CodeVerification(fa, fb, ra, rb)
	if len(code) != 6 {
		t.Fatalf("code %q", code)
	}
	if code == CodeVerification(fa, strings.Repeat("c", 64), ra, rb) {
		t.Fatal("une CA substituée doit changer le code (quasi sûrement)")
	}
	if string(h([]byte("ab"), []byte("c"))) == string(h([]byte("a"), []byte("bc"))) {
		t.Fatal("le préfixe de longueur doit séparer les champs")
	}
}

func TestJetons(t *testing.T) {
	j := NouveauJeton()
	if len(j) != 43 || HacherJeton(j) == j || len(HacherJeton(j)) != 64 {
		t.Fatal("jeton mal formé")
	}
	if len(JetonCourt()) != 22 {
		t.Fatal("jeton court mal formé")
	}
}
