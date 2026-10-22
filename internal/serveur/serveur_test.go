package serveur

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cams457/universal-airdrop/internal/appareils"
	"github.com/Cams457/universal-airdrop/internal/blocs"
	"github.com/Cams457/universal-airdrop/internal/client"
	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/identite"
	"github.com/Cams457/universal-airdrop/internal/partage"
)

const petitBloc = 256 << 10

func portLibre(t *testing.T) int {
	for range 50 {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		a, err1 := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", p))
		b, err2 := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", p+1))
		if a != nil {
			a.Close()
		}
		if b != nil {
			b.Close()
		}
		if err1 == nil && err2 == nil {
			return p
		}
	}
	t.Fatal("aucun port libre")
	return 0
}

func creer(t *testing.T, nom string) *Service {
	d := t.TempDir()
	cfg := &config.Config{Donnees: d, Nom: nom, Port: portLibre(t), Reception: filepath.Join(d, "recus"),
		Paralleles: 3, Decouverte: false, Mdns: false}
	s, err := Nouveau(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s.Verbeux = false
	if err := s.Demarrer(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Arreter)
	return s
}

// appairerAB : A s'appaire avec B ; l'utilisateur de B est simulé.
func appairerAB(t *testing.T, a, b *Service, accepterB, egal bool) error {
	b.OuvrirAppairage()
	_, _, err := client.Appairer(context.Background(), a.moi(), "127.0.0.1", b.Cfg.Port, func(code, nomB, fpB string) bool {
		b.mu.Lock()
		codeB := b.appairageB.Code
		b.mu.Unlock()
		if err := b.DecisionAppairage(accepterB && codeB == code); err != nil {
			t.Error(err)
		}
		return egal && codeB == code
	})
	return err
}

func aleatoire(t *testing.T, dossier, nom string, n int) string {
	p := filepath.Join(dossier, nom)
	b := make([]byte, n)
	rand.Read(b)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func memeContenu(t *testing.T, a, b string) {
	t.Helper()
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	if err1 != nil || err2 != nil || sha256.Sum256(x) != sha256.Sum256(y) {
		t.Fatalf("%s et %s diffèrent (%v %v)", a, b, err1, err2)
	}
}

func TestAppairageEtEnvoiDansLesDeuxSens(t *testing.T) {
	a, b := creer(t, "PC-A"), creer(t, "PC-B")
	if err := appairerAB(t, a, b, true, true); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := a.Reg.TrouverPC("PC-B"); !ok {
		t.Fatal("A ne connaît pas B")
	}
	if _, _, ok := b.Reg.TrouverPC("PC-A"); !ok {
		t.Fatal("B ne connaît pas A")
	}
	src := aleatoire(t, t.TempDir(), "video.mp4", 3*petitBloc+12345)
	bilans, err := client.Envoyer(context.Background(), a.Reg, "PC-B", []string{src}, client.Options{TailleBloc: petitBloc})
	if err != nil {
		t.Fatal(err)
	}
	if bilans[0].BlocsEnvoyes != 4 || bilans[0].BlocsTotal != 4 {
		t.Fatalf("bilan inattendu : %+v", bilans[0])
	}
	memeContenu(t, src, filepath.Join(b.Cfg.DossierReception(), "video.mp4"))

	// sens inverse, plusieurs fichiers dont un vide
	d := t.TempDir()
	var lot []string
	for i := range 6 {
		lot = append(lot, aleatoire(t, d, fmt.Sprintf("photo_%d.heic", i), 100_000+i))
	}
	vide := filepath.Join(d, "vide.txt")
	os.WriteFile(vide, nil, 0o644)
	lot = append(lot, vide)
	if _, err := client.Envoyer(context.Background(), b.Reg, "PC-A", lot, client.Options{TailleBloc: petitBloc}); err != nil {
		t.Fatal(err)
	}
	for _, f := range lot {
		memeContenu(t, f, filepath.Join(a.Cfg.DossierReception(), filepath.Base(f)))
	}

	ouverts := make(chan string, 1)
	b.OuvrirLien = func(u string) error { ouverts <- u; return nil }
	b.Cfg.OuvrirLiens = true
	if err := client.EnvoyerLien(context.Background(), a.Reg, "PC-B", "https://exemple.fr/", nil); err != nil {
		t.Fatal(err)
	}
	if u := <-ouverts; u != "https://exemple.fr/" {
		t.Fatalf("lien ouvert : %s", u)
	}
	if err := client.EnvoyerLien(context.Background(), a.Reg, "PC-B", "javascript:alert(1)", nil); err == nil {
		t.Fatal("lien javascript: accepté")
	}
}

func TestCodesDifferentsRefuses(t *testing.T) {
	a, b := creer(t, "PC-A"), creer(t, "PC-B")
	if err := appairerAB(t, a, b, false, false); err == nil {
		t.Fatal("l'appairage aurait dû échouer")
	}
	if len(a.Reg.Liste()) != 0 || len(b.Reg.Liste()) != 0 {
		t.Fatal("aucun appareil ne doit être enregistré")
	}
}

func TestAppairageSansFenetreRefuse(t *testing.T) {
	a, b := creer(t, "PC-A"), creer(t, "PC-B")
	_, _, err := client.Appairer(context.Background(), a.moi(), "127.0.0.1", b.Cfg.Port, func(string, string, string) bool { return true })
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("attendu 403, obtenu %v", err)
	}
}

func clientEpingle(t *testing.T, caPEM string) *http.Client {
	cfg, err := identite.ConfigClientEpingle(caPEM)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: client.Transport(cfg, 4)}
}

func faire(t *testing.T, c *http.Client, methode, url string, entetes map[string]string, corps []byte) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(methode, url, bytes.NewReader(corps))
	for k, v := range entetes {
		req.Header.Set(k, v)
	}
	r, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, b
}

func TestRepriseBlocCorrompuEtJetons(t *testing.T) {
	a, b := creer(t, "PC-A"), creer(t, "PC-B")
	if err := appairerAB(t, a, b, true, true); err != nil {
		t.Fatal(err)
	}
	_, pc, _ := a.Reg.TrouverPC("PC-B")
	c := clientEpingle(t, pc.CAPEM)
	base := fmt.Sprintf("https://127.0.0.1:%d", b.Cfg.Port)
	auth := map[string]string{"Authorization": "Bearer " + pc.JetonSortant, "Content-Type": "application/json"}
	src := aleatoire(t, t.TempDir(), "gros.bin", 8*petitBloc)
	st, _ := os.Stat(src)
	abs, _ := filepath.Abs(src)
	cle := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", abs, st.Size(), st.ModTime().UnixNano())))
	corps, _ := json.Marshal(map[string]any{"nom": "gros.bin", "taille": st.Size(), "taille_bloc": petitBloc, "cle": fmt.Sprintf("%x", cle)})
	code, rep := faire(t, c, "POST", base+"/api/v1/transfert/preparer", auth, corps)
	if code != 200 {
		t.Fatalf("preparer : %d %s", code, rep)
	}
	var prep struct{ ID string }
	json.Unmarshal(rep, &prep)
	f, _ := os.Open(src)
	defer f.Close()
	tampon := make([]byte, petitBloc)
	for i := range 3 { // coupure simulée : seuls les blocs 0 à 2 arrivent
		d, h, _ := blocs.LireEtHacher(f, int64(i), st.Size(), petitBloc, tampon)
		if code, _ := faire(t, c, "PUT", fmt.Sprintf("%s/api/v1/transfert/%s/bloc/%d", base, prep.ID, i),
			map[string]string{"Authorization": auth["Authorization"], "X-Bloc-SHA256": h}, d); code != 204 {
			t.Fatalf("bloc %d : %d", i, code)
		}
	}
	d, h, _ := blocs.LireEtHacher(f, 3, st.Size(), petitBloc, tampon)
	corrompu := append([]byte{d[0] ^ 1}, d[1:]...)
	if code, _ := faire(t, c, "PUT", fmt.Sprintf("%s/api/v1/transfert/%s/bloc/3", base, prep.ID),
		map[string]string{"Authorization": auth["Authorization"], "X-Bloc-SHA256": h}, corrompu); code != 422 {
		t.Fatalf("bloc corrompu : %d, attendu 422", code)
	}
	// reprise : seuls les 5 blocs manquants partent
	bilans, err := client.Envoyer(context.Background(), a.Reg, "PC-B", []string{src}, client.Options{TailleBloc: petitBloc})
	if err != nil {
		t.Fatal(err)
	}
	if bilans[0].BlocsEnvoyes != 5 || bilans[0].BlocsTotal != 8 {
		t.Fatalf("reprise : %d/%d blocs, attendu 5/8", bilans[0].BlocsEnvoyes, bilans[0].BlocsTotal)
	}
	memeContenu(t, src, filepath.Join(b.Cfg.DossierReception(), "gros.bin"))
	for range 20 {
		if code, _ := faire(t, c, "POST", base+"/api/v1/transfert/preparer", map[string]string{"Authorization": "Bearer faux"}, []byte("{}")); code != 401 {
			t.Fatalf("mauvais jeton : %d", code)
		}
	}
	if code, _ := faire(t, c, "POST", base+"/api/v1/transfert/preparer", auth, []byte("{}")); code != 429 {
		t.Fatalf("après 20 échecs : %d, attendu 429", code)
	}
}

func TestRevocationAChaud(t *testing.T) {
	a, b := creer(t, "PC-A"), creer(t, "PC-B")
	if err := appairerAB(t, a, b, true, true); err != nil {
		t.Fatal(err)
	}
	src := aleatoire(t, t.TempDir(), "a.txt", 10)
	if _, err := client.Envoyer(context.Background(), a.Reg, "PC-B", []string{src}, client.Options{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond) // l'horodatage du fichier doit changer
	if n, _ := appareils.Ouvrir(b.Cfg.Donnees).Revoquer("PC-A"); n < 1 {
		t.Fatal("révocation sans effet")
	}
	if _, err := client.Envoyer(context.Background(), a.Reg, "PC-B", []string{src}, client.Options{}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("attendu 401 après révocation, obtenu %v", err)
	}
}

func formulaire(t *testing.T, fichiers map[string][]byte) (string, []byte) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for nom, contenu := range fichiers {
		p, _ := w.CreateFormFile("fichier", nom)
		p.Write(contenu)
	}
	w.Close()
	return w.FormDataContentType(), buf.Bytes()
}

func TestTelephoneEtPartages(t *testing.T) {
	b := creer(t, "PC-B")
	jeton, _ := b.Reg.AjouterTelephone("iPhone de test")
	c := clientEpingle(t, b.ID.CAPEM)
	base := fmt.Sprintf("https://127.0.0.1:%d", b.Cfg.Port)
	photo := make([]byte, 300_000)
	rand.Read(photo)
	ct, corps := formulaire(t, map[string][]byte{"IMG_0001.HEIC": photo})
	if code, rep := faire(t, c, "POST", base+"/api/rapide/fichier", map[string]string{"Authorization": "Bearer " + jeton, "Content-Type": ct}, corps); code != 200 {
		t.Fatalf("Raccourci multipart : %d %s", code, rep)
	}
	if got, _ := os.ReadFile(filepath.Join(b.Cfg.DossierReception(), "IMG_0001.HEIC")); !bytes.Equal(got, photo) {
		t.Fatal("photo différente")
	}
	if code, _ := faire(t, c, "POST", base+"/api/rapide/fichier", map[string]string{"Authorization": "Bearer " + jeton, "X-Nom": "vid%C3%A9o.mov"}, []byte("video")); code != 200 {
		t.Fatal("corps brut refusé")
	}
	if got, _ := os.ReadFile(filepath.Join(b.Cfg.DossierReception(), "vidéo.mov")); string(got) != "video" {
		t.Fatal("nom URL-encodé mal décodé")
	}
	if code, _ := faire(t, c, "POST", base+"/api/rapide/fichier", nil, []byte("x")); code != 401 {
		t.Fatal("envoi sans jeton accepté")
	}
	if code, _ := faire(t, c, "POST", base+"/api/rapide/lien", map[string]string{"Authorization": "Bearer " + jeton, "Content-Type": "text/plain"}, []byte("file:///C:/Windows")); code != 400 {
		t.Fatal("lien file: accepté")
	}

	// page d'envoi pour n'importe quel téléphone
	je := b.Partages.Creer(&partage.Partage{Type: partage.Envoi}, time.Minute)
	code, page := faire(t, c, "GET", base+"/envoyer/"+je, nil, nil)
	if code != 200 || !strings.Contains(string(page), "Envoyer à PC-B") {
		t.Fatalf("page d'envoi : %d", code)
	}
	ct, corps = formulaire(t, map[string][]byte{"depuis-android.jpg": []byte("jpeg")})
	if code, _ := faire(t, c, "POST", base+"/envoyer/"+je, map[string]string{"Content-Type": ct}, corps); code != 200 {
		t.Fatal("envoi depuis la page refusé")
	}
	if code, _ := faire(t, c, "POST", base+"/envoyer/jetoninexistant00", map[string]string{"Content-Type": ct}, corps); code != 404 {
		t.Fatal("jeton de page inconnu accepté")
	}

	// partage PC -> téléphone : usage unique ; texte à usage unique
	doc := aleatoire(t, t.TempDir(), "notes.pdf", 50_000)
	jp := b.Partages.Creer(&partage.Partage{Type: partage.Fichiers, Fichiers: []string{doc}}, time.Minute)
	if code, page := faire(t, c, "GET", base+"/p/"+jp, nil, nil); code != 200 || !strings.Contains(string(page), "notes.pdf") {
		t.Fatal("page de partage")
	}
	if code, contenu := faire(t, c, "GET", base+"/p/"+jp+"/0", nil, nil); code != 200 || len(contenu) != 50_000 {
		t.Fatal("téléchargement")
	}
	if code, _ := faire(t, c, "GET", base+"/p/"+jp, nil, nil); code != 404 {
		t.Fatal("le partage doit disparaître après téléchargement")
	}
	jt := b.Partages.Creer(&partage.Partage{Type: partage.Texte, Texte: "SECRET-123"}, time.Minute)
	if _, page := faire(t, c, "GET", base+"/p/"+jt, nil, nil); !strings.Contains(string(page), "SECRET-123") {
		t.Fatal("texte absent")
	}
	if code, _ := faire(t, c, "GET", base+"/p/"+jt, nil, nil); code != 404 {
		t.Fatal("le texte doit être à usage unique")
	}
}

func TestControleLocal(t *testing.T) {
	a, b := creer(t, "PC-A"), creer(t, "PC-B")
	if err := appairerAB(t, a, b, true, true); err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Transport: &http.Transport{Proxy: nil}}
	url := fmt.Sprintf("http://127.0.0.1:%d", a.Cfg.PortControle())
	if code, page := faire(t, c, "GET", url+"/", nil, nil); code != 200 || !strings.Contains(string(page), "Universal Airdrop") {
		t.Fatal("interface non servie")
	}
	if code, _ := faire(t, c, "GET", url+"/controle/etat", nil, nil); code != 401 {
		t.Fatal("API accessible sans secret")
	}
	req, _ := http.NewRequest("GET", url+"/controle/etat", nil)
	req.Host = "attaquant.exemple" // DNS rebinding
	req.Header.Set("X-Controle", a.Secret)
	if r, err := c.Do(req); err != nil || r.StatusCode != 403 {
		t.Fatal("hôte étranger accepté")
	}
	ok := map[string]string{"X-Controle": a.Secret}
	// glisser-déposer dans l'interface, puis envoi vers B
	ct, corps := formulaire(t, map[string][]byte{"depose.txt": []byte("bonjour")})
	code, rep := faire(t, c, "POST", url+"/controle/televerser", map[string]string{"X-Controle": a.Secret, "Content-Type": ct}, corps)
	if code != 200 {
		t.Fatalf("téléverser : %d %s", code, rep)
	}
	var sel struct{ Selection string }
	json.Unmarshal(rep, &sel)
	corpsEnvoi, _ := json.Marshal(map[string]string{"cible": "PC-B", "selection": sel.Selection})
	if code, rep := faire(t, c, "POST", url+"/controle/envoyer", ok, corpsEnvoi); code != 200 {
		t.Fatalf("envoyer : %d %s", code, rep)
	}
	recu := filepath.Join(b.Cfg.DossierReception(), "depose.txt")
	for range 100 {
		if _, err := os.Stat(recu); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got, _ := os.ReadFile(recu); string(got) != "bonjour" {
		t.Fatal("fichier déposé non reçu par B")
	}
	// ajout d'un téléphone : profil + jeton, avec QR codes
	code, rep = faire(t, c, "POST", url+"/controle/telephone/ajouter", ok, []byte(`{"nom":"iPhone"}`))
	var tel struct {
		Profil, Jeton []struct{ URL, QR string }
		Empreinte     string
	}
	json.Unmarshal(rep, &tel)
	if code != 200 || len(tel.Empreinte) != 95 {
		t.Fatalf("ajout de téléphone : %d %s", code, rep)
	}
	if len(tel.Profil) > 0 && !strings.HasPrefix(tel.Profil[0].QR, "data:image/png;base64,") {
		t.Fatal("QR code absent")
	}
}

func TestRenommageAChaud(t *testing.T) {
	a := creer(t, "PC-A")
	if err := a.Renommer("PC de Camil"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Charger(a.Cfg.Donnees)
	if cfg.Nom != "PC de Camil" {
		t.Fatal("nom non sauvegardé")
	}
	c := clientEpingle(t, a.ID.CAPEM)
	_, rep := faire(t, c, "GET", fmt.Sprintf("https://127.0.0.1:%d/api/v1/info", a.Cfg.Port), nil, nil)
	if !strings.Contains(string(rep), "PC de Camil") {
		t.Fatal("le nouveau nom n'est pas servi")
	}
}
