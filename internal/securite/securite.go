// Package securite regroupe les primitives de sécurité : noms de fichiers sûrs, adresses autorisées,
// liens autorisés, jetons, engagement et code de vérification.
//
// Aucune primitive cryptographique n'est réimplémentée : tout repose sur crypto/sha256,
// crypto/rand et crypto/subtle de la bibliothèque standard.
package securite

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	caracteresInterdits = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f\x7f]`)
	nomsReserves        = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
)

func init() {
	for i := 1; i <= 9; i++ {
		nomsReserves[fmt.Sprintf("COM%d", i)] = true
		nomsReserves[fmt.Sprintf("LPT%d", i)] = true
	}
}

// NomSur réduit un nom reçu du réseau à un nom de fichier simple et sans danger.
// Il défend contre la traversée de répertoire (../, chemins absolus, C:\), les noms réservés
// de Windows (CON, NUL…), les caractères interdits et les noms vides ou démesurés.
func NomSur(nom string) string {
	nom = norm.NFC.String(strings.ToValidUTF8(nom, "_"))
	if i := strings.LastIndexAny(nom, `/\`); i >= 0 {
		nom = nom[i+1:]
	}
	nom = caracteresInterdits.ReplaceAllString(nom, "_")
	nom = strings.Trim(nom, " .") // Windows ignore les points et espaces finaux
	if nom == "" {
		nom = "fichier"
	}
	racine := strings.ToUpper(strings.SplitN(nom, ".", 2)[0])
	if nomsReserves[racine] {
		nom = "_" + nom
	}
	if utf8.RuneCountInString(nom) > 200 {
		ext := filepath.Ext(nom)
		if utf8.RuneCountInString(ext) > 10 {
			ext = ""
		}
		r := []rune(strings.TrimSuffix(nom, ext))
		nom = string(r[:200-utf8.RuneCountInString(ext)]) + ext
	}
	return nom
}

// CheminLibre renvoie un chemin qui n'écrase jamais un fichier existant : « photo (1).jpg »…
func CheminLibre(dossier, nom string) (string, error) {
	if err := os.MkdirAll(dossier, 0o755); err != nil {
		return "", err
	}
	cible := filepath.Join(dossier, nom)
	if _, err := os.Lstat(cible); os.IsNotExist(err) {
		return cible, nil
	}
	ext := filepath.Ext(nom)
	base := strings.TrimSuffix(nom, ext)
	for i := 1; ; i++ {
		c := filepath.Join(dossier, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Lstat(c); os.IsNotExist(err) {
			return c, nil
		}
	}
}

// IPAutorisee n'accepte que les adresses du réseau local : privées, boucle locale, lien local.
func IPAutorisee(ip string) bool {
	if h, _, err := net.SplitHostPort(ip); err == nil {
		ip = h
	}
	if i := strings.IndexByte(ip, '%'); i >= 0 {
		ip = ip[:i]
	}
	a := net.ParseIP(ip)
	if a == nil {
		return false
	}
	return a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast()
}

// URLAutorisee : seuls http et https sont ouverts, jamais file:, javascript:, ms-settings:…
func URLAutorisee(lien string) bool {
	if lien == "" || len(lien) > 2048 {
		return false
	}
	for _, c := range lien {
		if c < 32 || c == 0x7f {
			return false
		}
	}
	u, err := url.Parse(strings.TrimSpace(lien))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// NouveauJeton : 256 bits d'aléa cryptographique, encodés en base64 URL (43 caractères).
func NouveauJeton() string {
	return base64.RawURLEncoding.EncodeToString(Alea(32))
}

// JetonCourt : 128 bits, pour les liens de partage à usage unique.
func JetonCourt() string {
	return base64.RawURLEncoding.EncodeToString(Alea(16))
}

// Alea renvoie n octets d'aléa cryptographique.
func Alea(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("générateur aléatoire indisponible : " + err.Error())
	}
	return b
}

// HacherJeton : le serveur ne stocke que le hachage, un vol du fichier ne livre pas les jetons.
func HacherJeton(jeton string) string {
	s := sha256.Sum256([]byte(jeton))
	return hex.EncodeToString(s[:])
}

// EgalTempsConstant compare deux chaînes sans fuite d'information par le temps de calcul.
func EgalTempsConstant(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// h calcule SHA-256 d'une concaténation préfixée par les longueurs (4 octets gros-boutiste).
// Sans le préfixe, H(ab || c) = H(a || bc) : on pourrait déplacer des octets d'un champ à l'autre.
func h(morceaux ...[]byte) []byte {
	s := sha256.New()
	var l [4]byte
	for _, m := range morceaux {
		binary.BigEndian.PutUint32(l[:], uint32(len(m)))
		s.Write(l[:])
		s.Write(m)
	}
	return s.Sum(nil)
}

// Engagement : c_B = H(fp_B || fp_A || r_B). B fixe r_B avant de connaître r_A.
func Engagement(empreinteB, empreinteA string, aleaB []byte) string {
	return hex.EncodeToString(h([]byte("engagement"), []byte(empreinteB), []byte(empreinteA), aleaB))
}

// CodeVerification : code à 6 chiffres affiché sur les deux écrans.
// On réduit 64 bits modulo 10^6 : le biais, de l'ordre de 10^6 / 2^64 ≈ 5e-14, est négligeable.
func CodeVerification(empreinteA, empreinteB string, aleaA, aleaB []byte) string {
	d := h([]byte("code"), []byte(empreinteA), []byte(empreinteB), aleaA, aleaB)
	return fmt.Sprintf("%06d", binary.BigEndian.Uint64(d[:8])%1_000_000)
}
