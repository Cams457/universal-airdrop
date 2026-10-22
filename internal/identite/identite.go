// Package identite gère l'identité cryptographique du PC : une autorité de certification (CA)
// locale et un certificat serveur signé par elle.
//
// La CA est l'identité stable du PC : son empreinte SHA-256 est ce que les autres PC épinglent
// et ce que l'iPhone installe dans un profil. Le certificat serveur porte les IP actuelles (SAN)
// et le nom Bonjour ; il est réémis quand l'IP change, sans casser la confiance.
// iOS exige pour un certificat serveur : un SAN, l'usage serverAuth et au plus 825 jours de validité.
package identite

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	validiteCA      = 10 * 365 * 24 * time.Hour
	validiteServeur = 800 * 24 * time.Hour
	marge           = 30 * 24 * time.Hour
)

// Identite : CA + certificat serveur courant.
type Identite struct {
	dossier   string
	Nom       string
	NomsDNS   []string
	CAPEM     string
	CA        *x509.Certificate
	cleCA     *ecdsa.PrivateKey
	Empreinte string

	mu      sync.RWMutex
	serveur *tls.Certificate
	feuille *x509.Certificate
	// IPsLocales est remplaçable par les tests.
	IPsLocales func() []string
}

// Ouvrir charge (ou crée) l'identité stockée dans dossier/identite.
func Ouvrir(dossier, nom string, nomsDNS []string, ips func() []string) (*Identite, error) {
	d := filepath.Join(dossier, "identite")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return nil, err
	}
	id := &Identite{dossier: d, Nom: nom, NomsDNS: nomsDNS, IPsLocales: ips}
	if _, err := os.Stat(id.chemin("ca.pem")); os.IsNotExist(err) {
		if err := id.creerCA(); err != nil {
			return nil, err
		}
	}
	if err := id.chargerCA(); err != nil {
		return nil, err
	}
	if _, err := id.VerifierServeur(); err != nil {
		return nil, err
	}
	return id, nil
}

func (id *Identite) chemin(n string) string { return filepath.Join(id.dossier, n) }

func ecrirePEM(chemin, type_ string, der []byte, secret bool) error {
	mode := os.FileMode(0o644)
	if secret {
		mode = 0o600
	}
	return os.WriteFile(chemin, pem.EncodeToMemory(&pem.Block{Type: type_, Bytes: der}), mode)
}

func numeroSerie() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	return n
}

func (id *Identite) creerCA() error {
	cle, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	nom := "Universal Airdrop CA - " + id.Nom
	if len(nom) > 64 {
		nom = nom[:64]
	}
	modele := &x509.Certificate{
		SerialNumber:          numeroSerie(),
		Subject:               pkix.Name{CommonName: nom},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(validiteCA),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, modele, modele, &cle.PublicKey, cle)
	if err != nil {
		return err
	}
	cleDER, err := x509.MarshalPKCS8PrivateKey(cle)
	if err != nil {
		return err
	}
	if err := ecrirePEM(id.chemin("ca_cle.pem"), "PRIVATE KEY", cleDER, true); err != nil {
		return err
	}
	return ecrirePEM(id.chemin("ca.pem"), "CERTIFICATE", der, false)
}

func lirePEM(chemin string) ([]byte, error) {
	brut, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	b, _ := pem.Decode(brut)
	if b == nil {
		return nil, fmt.Errorf("%s : PEM invalide", chemin)
	}
	return b.Bytes, nil
}

func (id *Identite) chargerCA() error {
	brut, err := os.ReadFile(id.chemin("ca.pem"))
	if err != nil {
		return err
	}
	id.CAPEM = string(brut)
	der, err := lirePEM(id.chemin("ca.pem"))
	if err != nil {
		return err
	}
	if id.CA, err = x509.ParseCertificate(der); err != nil {
		return err
	}
	cleDER, err := lirePEM(id.chemin("ca_cle.pem"))
	if err != nil {
		return err
	}
	cle, err := x509.ParsePKCS8PrivateKey(cleDER)
	if err != nil {
		return err
	}
	var ok bool
	if id.cleCA, ok = cle.(*ecdsa.PrivateKey); !ok {
		return errors.New("clé de CA inattendue")
	}
	id.Empreinte = EmpreinteDER(der)
	return nil
}

// EmpreinteDER : SHA-256 d'un certificat, en hexadécimal.
func EmpreinteDER(der []byte) string {
	s := sha256.Sum256(der)
	return hex.EncodeToString(s[:])
}

// EmpreintePEM : empreinte d'un certificat fourni en PEM ; vérifie au passage que c'est une CA.
func EmpreintePEM(p string) (string, *x509.Certificate, error) {
	b, _ := pem.Decode([]byte(p))
	if b == nil || b.Type != "CERTIFICATE" {
		return "", nil, errors.New("certificat PEM invalide")
	}
	c, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return "", nil, err
	}
	if !c.IsCA {
		return "", nil, errors.New("le certificat n'est pas une autorité de certification")
	}
	return EmpreinteDER(b.Bytes), c, nil
}

// Lisible : format affiché à l'humain, comme iOS (groupes de 2 caractères hexadécimaux).
func Lisible(fp string) string {
	var b strings.Builder
	for i := 0; i < len(fp); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strings.ToUpper(fp[i:min(i+2, len(fp))]))
	}
	return b.String()
}

// DERCA : la CA au format DER (pour le profil iOS).
func (id *Identite) DERCA() []byte { return id.CA.Raw }

// DefinirNomsDNS change les noms inscrits dans le certificat (renommage du PC) ; le prochain
// VerifierServeur réémet le certificat.
func (id *Identite) DefinirNomsDNS(noms []string) {
	id.mu.Lock()
	id.NomsDNS = append([]string(nil), noms...)
	id.mu.Unlock()
}

func (id *Identite) nomsDNS() []string {
	id.mu.RLock()
	defer id.mu.RUnlock()
	return append([]string(nil), id.NomsDNS...)
}

func (id *Identite) ips() []string {
	ips := []string{"127.0.0.1"}
	if id.IPsLocales != nil {
		ips = append(ips, id.IPsLocales()...)
	}
	slices.Sort(ips)
	return slices.Compact(ips)
}

// VerifierServeur réémet le certificat serveur s'il manque, expire bientôt, ou ne couvre plus
// les IP ou noms actuels. Renvoie true si un nouveau certificat a été émis.
func (id *Identite) VerifierServeur() (bool, error) {
	ips := id.ips()
	if id.feuille == nil {
		_ = id.chargerServeur()
	}
	id.mu.RLock()
	f := id.feuille
	id.mu.RUnlock()
	if f != nil && time.Until(f.NotAfter) > marge {
		couvertes := map[string]bool{}
		for _, a := range f.IPAddresses {
			couvertes[a.String()] = true
		}
		ok := true
		for _, ip := range ips {
			ok = ok && couvertes[ip]
		}
		for _, n := range id.nomsDNS() {
			ok = ok && slices.Contains(f.DNSNames, n)
		}
		if ok {
			return false, nil
		}
	}
	return true, id.creerServeur(ips)
}

func (id *Identite) chargerServeur() error {
	cert, err := tls.LoadX509KeyPair(id.chemin("serveur_chaine.pem"), id.chemin("serveur_cle.pem"))
	if err != nil {
		return err
	}
	f, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return err
	}
	id.mu.Lock()
	id.serveur, id.feuille = &cert, f
	id.mu.Unlock()
	return nil
}

func (id *Identite) creerServeur(ips []string) error {
	cle, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	hote, _ := os.Hostname()
	dns := []string{"localhost"}
	if hote != "" {
		dns = append(dns, hote, hote+".local")
	}
	for _, n := range id.nomsDNS() {
		if !slices.Contains(dns, n) {
			dns = append(dns, n)
		}
	}
	var adresses []net.IP
	for _, ip := range ips {
		if a := net.ParseIP(ip); a != nil {
			adresses = append(adresses, a)
		}
	}
	cn := id.Nom
	if len(cn) > 64 {
		cn = cn[:64]
	}
	modele := &x509.Certificate{
		SerialNumber: numeroSerie(),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().Add(validiteServeur),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dns,
		IPAddresses:  adresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, modele, id.CA, &cle.PublicKey, id.cleCA)
	if err != nil {
		return err
	}
	cleDER, err := x509.MarshalPKCS8PrivateKey(cle)
	if err != nil {
		return err
	}
	if err := ecrirePEM(id.chemin("serveur_cle.pem"), "PRIVATE KEY", cleDER, true); err != nil {
		return err
	}
	chaine := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), []byte(id.CAPEM)...)
	if err := os.WriteFile(id.chemin("serveur_chaine.pem"), chaine, 0o644); err != nil {
		return err
	}
	return id.chargerServeur()
}

// ConfigServeur : TLS 1.2 minimum ; GetCertificate sert toujours le certificat le plus récent,
// d'où un rechargement à chaud quand l'IP change (partage de connexion activé).
func (id *Identite) ConfigServeur() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			id.mu.RLock()
			defer id.mu.RUnlock()
			return id.serveur, nil
		},
	}
}

// ConfigClientEpingle : client qui n'accepte QUE la CA du pair (épinglage).
// La vérification de nom d'hôte est remplacée par mieux : la chaîne doit remonter à CETTE CA,
// et les IP du pair peuvent changer.
func ConfigClientEpingle(caPEMPair string) (*tls.Config, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEMPair)) {
		return nil, errors.New("CA du pair illisible")
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // remplacé par VerifyPeerCertificate ci-dessous, plus strict
		VerifyPeerCertificate: func(bruts [][]byte, _ [][]*x509.Certificate) error {
			if len(bruts) == 0 {
				return errors.New("aucun certificat présenté")
			}
			feuille, err := x509.ParseCertificate(bruts[0])
			if err != nil {
				return err
			}
			inter := x509.NewCertPool()
			for _, b := range bruts[1:] {
				if c, err := x509.ParseCertificate(b); err == nil {
					inter.AddCert(c)
				}
			}
			_, err = feuille.Verify(x509.VerifyOptions{Roots: pool, Intermediates: inter,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
			if err != nil {
				return fmt.Errorf("certificat du pair refusé (CA épinglée différente) : %w", err)
			}
			return nil
		},
	}, nil
}

// ConfigClientPremierContact : uniquement pour le tout premier message d'appairage, chiffré mais
// non authentifié. L'authentification vient ensuite du code comparé par les humains.
func ConfigClientPremierContact() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}
}
