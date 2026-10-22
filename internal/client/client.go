// Package client : côté émetteur. Appairage (côté initiateur A) et envoi PC -> PC par blocs.
//
// Vitesse : plusieurs blocs en vol (connexions HTTP/1.1 parallèles, plus robustes aux pertes Wi-Fi
// qu'une connexion HTTP/2 unique), plusieurs fichiers en vol pour les lots de photos, et lecture
// disque + SHA-256 d'un bloc pendant que le réseau transporte les autres. Le premier bloc part sans
// attendre le hachage du fichier entier : la latence de démarrage reste faible.
package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Cams457/universal-airdrop/internal/appareils"
	"github.com/Cams457/universal-airdrop/internal/blocs"
	"github.com/Cams457/universal-airdrop/internal/config"
	"github.com/Cams457/universal-airdrop/internal/decouverte"
	"github.com/Cams457/universal-airdrop/internal/identite"
	"github.com/Cams457/universal-airdrop/internal/reseau"
	"github.com/Cams457/universal-airdrop/internal/securite"
)

const essais = 4

// Transport HTTP/1.1 vers un pair, TLS épinglé.
func Transport(tlsCfg *tls.Config, conns int) *http.Transport {
	return &http.Transport{
		TLSClientConfig:       tlsCfg,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxConnsPerHost:       conns,
		MaxIdleConnsPerHost:   conns,
		ForceAttemptHTTP2:     false,
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{}, // pas de HTTP/2
		ResponseHeaderTimeout: 2 * time.Minute,
		WriteBufferSize:       256 << 10,
		ReadBufferSize:        256 << 10,
		Proxy:                 nil, // réseau local : jamais de proxy
	}
}

// ErreurDistante : réponse d'erreur du pair.
type ErreurDistante struct {
	Statut  int
	Message string
}

func (e *ErreurDistante) Error() string { return fmt.Sprintf("%d : %s", e.Statut, e.Message) }

func verifier(r *http.Response) error {
	if r.StatusCode < 400 {
		return nil
	}
	var m struct {
		Erreur string `json:"erreur"`
	}
	brut, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	if json.Unmarshal(brut, &m) != nil || m.Erreur == "" {
		m.Erreur = http.StatusText(r.StatusCode)
	}
	return &ErreurDistante{Statut: r.StatusCode, Message: m.Erreur}
}

func posterJSON(ctx context.Context, c *http.Client, url string, entetes map[string]string, corps, sortie any) error {
	brut, _ := json.Marshal(corps)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(brut))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range entetes {
		req.Header.Set(k, v)
	}
	r, err := c.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if err := verifier(r); err != nil {
		return err
	}
	if sortie != nil {
		return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(sortie)
	}
	_, _ = io.Copy(io.Discard, r.Body)
	return nil
}

// ---------- appairage ----------

// Moi : identité de ce PC, telle que présentée aux pairs.
type Moi struct {
	Nom  string
	Port int
	ID   *identite.Identite
	Reg  *appareils.Registre
}

// Appairer appaire ce PC (A) avec le PC distant (B), qui doit avoir ouvert une fenêtre d'appairage.
// confirmer(code, nomB, empreinteB) affiche le code et renvoie true si l'utilisateur confirme
// qu'il est identique à celui de l'autre écran.
func Appairer(ctx context.Context, moi Moi, hote string, port int, confirmer func(code, nomB, fpB string) bool) (string, string, error) {
	base := "https://" + net.JoinHostPort(hote, strconv.Itoa(port))
	premier := &http.Client{Transport: Transport(identite.ConfigClientPremierContact(), 1), Timeout: 15 * time.Second}
	// 1. Premier contact : chiffré mais pas encore authentifié. B renvoie sa CA et s'engage sur r_B.
	var d struct {
		Session, Nom, CAPEM, Engagement string
		Port                            int
	}
	var brut struct {
		Session    string `json:"session"`
		Nom        string `json:"nom"`
		CAPEM      string `json:"ca_pem"`
		Engagement string `json:"engagement"`
		Port       int    `json:"port"`
	}
	if err := posterJSON(ctx, premier, base+"/api/v1/appairage/debut", nil,
		map[string]any{"nom": moi.Nom, "ca_pem": moi.ID.CAPEM, "port": moi.Port}, &brut); err != nil {
		return "", "", err
	}
	d.Session, d.Nom, d.CAPEM, d.Engagement, d.Port = brut.Session, brut.Nom, brut.CAPEM, brut.Engagement, brut.Port
	fpB, _, err := identite.EmpreintePEM(d.CAPEM)
	if err != nil {
		return "", "", err
	}
	// 2. À partir d'ici, seul le détenteur de cette CA peut répondre : c'est elle que le code authentifie.
	tlsB, err := identite.ConfigClientEpingle(d.CAPEM)
	if err != nil {
		return "", "", err
	}
	epingle := &http.Client{Transport: Transport(tlsB, 1), Timeout: time.Duration(config.DureeAppairageS+20) * time.Second}
	aleaA := securite.Alea(32)
	var rev struct {
		AleaB string `json:"alea_b"`
	}
	if err := posterJSON(ctx, epingle, base+"/api/v1/appairage/revele", nil,
		map[string]any{"session": d.Session, "alea_a": hex.EncodeToString(aleaA)}, &rev); err != nil {
		return "", "", err
	}
	aleaB, err := hex.DecodeString(rev.AleaB)
	if err != nil || len(aleaB) != 32 {
		return "", "", errors.New("aléa du pair invalide")
	}
	if !securite.EgalTempsConstant(securite.Engagement(fpB, moi.ID.Empreinte, aleaB), d.Engagement) {
		return "", "", errors.New("engagement non respecté par le pair : attaque possible, appairage annulé")
	}
	code := securite.CodeVerification(moi.ID.Empreinte, fpB, aleaA, aleaB)
	if !confirmer(code, d.Nom, identite.Lisible(fpB)) {
		return "", "", errors.New("appairage annulé : les codes ne correspondent pas")
	}
	// 3. Confirmation : chacun remet à l'autre le jeton que l'autre présentera pour lui envoyer.
	jetonPourB := securite.NouveauJeton()
	var conf struct {
		JetonPourA string `json:"jeton_pour_a"`
	}
	if err := posterJSON(ctx, epingle, base+"/api/v1/appairage/confirmer", nil,
		map[string]any{"session": d.Session, "jeton_pour_b": jetonPourB}, &conf); err != nil {
		return "", "", err
	}
	if err := moi.Reg.AjouterPC(fpB, d.Nom, d.CAPEM, hote, d.Port, conf.JetonPourA, jetonPourB); err != nil {
		return "", "", err
	}
	return d.Nom, fpB, nil
}

// ---------- choix du chemin ----------

type Candidat struct {
	IP   string
	Port int
}

// ClasserChemins trie les adresses d'un pair du chemin le plus rapide au plus lent.
// Capacité d'un chemin = min(débit de notre interface de sortie, débit annoncé par le pair).
func ClasserChemins(adresses map[string]*decouverte.Adresse, capacite func(ip string, l reseau.Lien, mbits int) int) []Candidat {
	if capacite == nil {
		capacite = reseau.CapaciteVers
	}
	type note struct {
		c Candidat
		v int
	}
	var notes []note
	for ip, a := range adresses {
		notes = append(notes, note{Candidat{ip, a.Port}, capacite(ip, a.Lien, a.Mbits)})
	}
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].v != notes[j].v {
			return notes[i].v > notes[j].v
		}
		return notes[i].c.IP < notes[j].c.IP
	})
	var l []Candidat
	for _, n := range notes {
		l = append(l, n.c)
	}
	return l
}

// Source : adresses connues d'un pair (fournies par la découverte du démon local).
type Source func(fp string) map[string]*decouverte.Adresse

func candidats(source Source, fp string, pc appareils.PC) []Candidat {
	var l []Candidat
	if source != nil {
		if a := source(fp); len(a) > 0 {
			l = ClasserChemins(a, nil)
		}
	}
	vus := map[string]bool{}
	for _, c := range l {
		vus[c.IP] = true
	}
	ajouter := func(ip string) {
		if ip != "" && !vus[ip] {
			vus[ip] = true
			l = append(l, Candidat{ip, pc.Port})
		}
	}
	ajouter(pc.Adresse)
	// adresses mémorisées : réseau local d'abord, Tailscale ensuite (il marche aussi à distance)
	for _, ip := range pc.Adresses {
		if !reseau.EstTailscale(ip) {
			ajouter(ip)
		}
	}
	for _, ip := range pc.Adresses {
		if reseau.EstTailscale(ip) {
			ajouter(ip)
		}
	}
	return l
}

// joindre essaie les adresses candidates presque en même temps (départs décalés de 300 ms, dans
// l'ordre de préférence) : hors de chez soi, les adresses du réseau local ne répondent pas, et
// attendre leur délai l'une après l'autre retarderait l'adresse Tailscale de plusieurs secondes.
// Renvoie l'adresse qui a répondu et la description du pair (/api/v1/info).
func joindre(ctx context.Context, c *http.Client, cands []Candidat) (string, []byte, error) {
	type resultat struct {
		i     int
		base  string
		corps []byte
		err   error
	}
	if len(cands) == 0 {
		return "", nil, errors.New("aucune adresse connue pour ce PC")
	}
	cctx, fin := context.WithCancel(ctx)
	defer fin()
	res := make(chan resultat, len(cands))
	for i, cand := range cands {
		go func() {
			select {
			case <-time.After(time.Duration(i) * 300 * time.Millisecond):
			case <-cctx.Done():
				res <- resultat{i: i, err: cctx.Err()}
				return
			}
			base := "https://" + net.JoinHostPort(cand.IP, strconv.Itoa(cand.Port))
			rctx, rfin := context.WithTimeout(cctx, 2500*time.Millisecond)
			defer rfin()
			req, _ := http.NewRequestWithContext(rctx, http.MethodGet, base+"/api/v1/info", nil)
			r, err := c.Do(req)
			if err != nil {
				res <- resultat{i: i, err: err}
				return
			}
			corps, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
			r.Body.Close()
			if r.StatusCode != http.StatusOK {
				res <- resultat{i: i, err: fmt.Errorf("statut %d", r.StatusCode)}
				return
			}
			res <- resultat{i: i, base: base, corps: corps}
		}()
	}
	echecs := make([]string, len(cands))
	for range cands {
		x := <-res
		if x.err == nil {
			return x.base, x.corps, nil
		}
		echecs[x.i] = cands[x.i].IP + " (" + x.err.Error() + ")"
	}
	return "", nil, fmt.Errorf("pair injoignable : %s. Vérifie qu'il est allumé, que Universal Airdrop y tourne, et qu'il est sur le même réseau ou joignable par Tailscale",
		strings.Join(echecs, ", "))
}

// ---------- envoi ----------

// Bilan d'un fichier envoyé.
type Bilan struct {
	Nom          string        `json:"nom"`
	Taille       int64         `json:"taille"`
	Duree        time.Duration `json:"duree"`
	Demarrage    time.Duration `json:"demarrage"`
	BlocsEnvoyes int           `json:"blocs_envoyes"`
	BlocsTotal   int           `json:"blocs_total"`
	NomRecu      string        `json:"nom_recu"`
}

func (b Bilan) Mbits() float64 {
	if b.Duree <= 0 {
		return 0
	}
	return 8 * float64(b.Taille) / b.Duree.Seconds() / 1e6
}

// Options d'un envoi.
type Options struct {
	Paralleles    int
	FichiersEnVol int
	TailleBloc    int64
	Progres       func(fait, total int64)
	AvantEnvoi    func(ipPair string) bool
	Source        Source
	// De : appareil d'origine quand ce PC relaie (ex. « iPhone de Camil »), affiché par le receveur.
	De string
	// Pour : téléphones du PC distant à qui remettre le fichier (sa boîte d'envoi), au lieu de le
	// garder lui-même.
	Pour []string
}

func (o *Options) normaliser() {
	if o.Paralleles < 1 {
		o.Paralleles = config.BlocsEnVol
	}
	if o.FichiersEnVol < 1 {
		o.FichiersEnVol = config.FichiersEnVol
	}
	if o.TailleBloc <= 0 {
		o.TailleBloc = config.TailleBloc
	}
}

// Connexion ouverte vers un pair appairé.
type Connexion struct {
	Base, Jeton, Nom string
	Client           *http.Client
	De               string
	Pour             []string
}

// Ouvrir résout l'adresse d'un pair appairé et vérifie qu'il répond, avec sa CA épinglée.
func Ouvrir(ctx context.Context, reg *appareils.Registre, cible string, o Options) (*Connexion, error) {
	o.normaliser()
	fp, pc, ok := reg.TrouverPC(cible)
	if !ok {
		return nil, fmt.Errorf("« %s » n'est pas un PC appairé : appaire-le d'abord", cible)
	}
	tlsCfg, err := identite.ConfigClientEpingle(pc.CAPEM)
	if err != nil {
		return nil, err
	}
	c := &http.Client{Transport: Transport(tlsCfg, o.Paralleles+o.FichiersEnVol)}
	base, corps, err := joindre(ctx, c, candidats(o.Source, fp, pc))
	if err != nil {
		return nil, err
	}
	var info struct {
		IPs []string `json:"ips"`
	}
	if json.Unmarshal(corps, &info) == nil && len(info.IPs) > 0 {
		reg.MemoriserAdresses(fp, info.IPs)
	}
	return &Connexion{Base: base, Jeton: pc.JetonSortant, Nom: pc.Nom, Client: c, De: o.De, Pour: o.Pour}, nil
}

func (c *Connexion) auth() map[string]string {
	h := map[string]string{"Authorization": "Bearer " + c.Jeton}
	if c.De != "" {
		h["X-De"] = url.QueryEscape(c.De)
	}
	if len(c.Pour) > 0 {
		h["X-Pour"] = url.QueryEscape(strings.Join(c.Pour, "|"))
	}
	return h
}

// IP du pair joint.
func (c *Connexion) IP() string {
	h := strings.TrimPrefix(c.Base, "https://")
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

// Envoyer envoie des fichiers vers un PC appairé.
func Envoyer(ctx context.Context, reg *appareils.Registre, cible string, chemins []string, o Options) ([]Bilan, error) {
	o.normaliser()
	cx, err := Ouvrir(ctx, reg, cible, o)
	if err != nil {
		return nil, err
	}
	if o.AvantEnvoi != nil && !o.AvantEnvoi(cx.IP()) {
		return nil, errors.New("envoi annulé")
	}
	return cx.EnvoyerFichiers(ctx, chemins, o)
}

// EnvoyerFichiers : plusieurs fichiers en vol ; enVol borne le total de blocs simultanés.
func (cx *Connexion) EnvoyerFichiers(ctx context.Context, chemins []string, o Options) ([]Bilan, error) {
	o.normaliser()
	var total int64
	for _, ch := range chemins {
		st, err := os.Stat(ch)
		if err != nil {
			return nil, err
		}
		total += st.Size()
	}
	var fait atomic.Int64
	ajout := func(n int64) {
		v := fait.Add(n)
		if o.Progres != nil {
			o.Progres(v, total)
		}
	}
	ctx, annuler := context.WithCancel(ctx)
	defer annuler()
	enVol := make(chan struct{}, o.Paralleles)
	fichiers := make(chan struct{}, o.FichiersEnVol)
	bilans := make([]Bilan, len(chemins))
	var (
		wg       sync.WaitGroup
		muErr    sync.Mutex
		premiere error
	)
	for k, ch := range chemins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fichiers <- struct{}{}
			defer func() { <-fichiers }()
			if ctx.Err() != nil {
				return
			}
			b, err := cx.envoyerFichier(ctx, ch, o, enVol, ajout)
			if err != nil {
				muErr.Lock()
				if premiere == nil {
					premiere = fmt.Errorf("%s : %w", filepath.Base(ch), err)
					annuler()
				}
				muErr.Unlock()
				return
			}
			bilans[k] = b
		}()
	}
	wg.Wait()
	return bilans, premiere
}

func (cx *Connexion) envoyerFichier(ctx context.Context, chemin string, o Options, enVol chan struct{}, ajout func(int64)) (Bilan, error) {
	t0 := time.Now()
	f, err := os.Open(chemin)
	if err != nil {
		return Bilan{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Bilan{}, err
	}
	abs, _ := filepath.Abs(chemin)
	s := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", abs, st.Size(), st.ModTime().UnixNano())))
	var prep struct {
		ID        string  `json:"id"`
		Manquants []int64 `json:"manquants"`
	}
	if err := posterJSON(ctx, cx.Client, cx.Base+"/api/v1/transfert/preparer", cx.auth(), map[string]any{
		"nom": filepath.Base(chemin), "taille": st.Size(), "taille_bloc": o.TailleBloc, "cle": hex.EncodeToString(s[:]),
	}, &prep); err != nil {
		return Bilan{}, err
	}
	taille, b := st.Size(), o.TailleBloc
	n := blocs.NombreBlocs(taille, b)
	manquant := make(map[int64]bool, len(prep.Manquants))
	for _, i := range prep.Manquants {
		manquant[i] = true
	}
	hach := make([]string, n)
	indices := make(chan int64, n)
	for i := int64(0); i < n; i++ {
		indices <- i
	}
	close(indices)
	var (
		premier atomic.Int64
		wg      sync.WaitGroup
		muErr   sync.Mutex
		erreur  error
	)
	ouvriers := int(min(int64(o.Paralleles), max(n, 1)))
	for range ouvriers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tampon := make([]byte, b)
			for i := range indices {
				if ctx.Err() != nil {
					return
				}
				donnees, h, err := blocs.LireEtHacher(f, i, taille, b, tampon)
				if err != nil {
					muErr.Lock()
					erreur = err
					muErr.Unlock()
					return
				}
				hach[i] = h
				if manquant[i] {
					if err := cx.envoyerBloc(ctx, prep.ID, i, donnees, h, enVol); err != nil {
						muErr.Lock()
						if erreur == nil {
							erreur = err
						}
						muErr.Unlock()
						return
					}
					premier.CompareAndSwap(0, int64(time.Since(t0)))
				}
				ajout(int64(len(donnees)))
			}
		}()
	}
	wg.Wait()
	if ctx.Err() != nil { // annulé : le receveur efface le fichier partiel
		cx.abandonner(prep.ID)
		return Bilan{}, ctx.Err()
	}
	if erreur != nil {
		return Bilan{}, erreur
	}
	var fin struct {
		Nom string `json:"nom"`
	}
	if err := posterJSON(ctx, cx.Client, cx.Base+"/api/v1/transfert/"+prep.ID+"/terminer", cx.auth(),
		map[string]any{"racine": blocs.Racine(hach)}, &fin); err != nil {
		return Bilan{}, err
	}
	duree := time.Since(t0)
	dem := time.Duration(premier.Load())
	if dem == 0 {
		dem = duree
	}
	return Bilan{Nom: filepath.Base(chemin), Taille: taille, Duree: duree, Demarrage: dem,
		BlocsEnvoyes: len(prep.Manquants), BlocsTotal: int(n), NomRecu: fin.Nom}, nil
}

// abandonner prévient le receveur d'une annulation (au mieux : 3 secondes maximum).
func (cx *Connexion) abandonner(ident string) {
	ctx, fin := context.WithTimeout(context.Background(), 3*time.Second)
	defer fin()
	req, _ := http.NewRequestWithContext(ctx, "DELETE", cx.Base+"/api/v1/transfert/"+ident, nil)
	for k, v := range cx.auth() {
		req.Header.Set(k, v)
	}
	if r, err := cx.Client.Do(req); err == nil {
		r.Body.Close()
	}
}

func (cx *Connexion) envoyerBloc(ctx context.Context, ident string, i int64, donnees []byte, h string, enVol chan struct{}) error {
	var derniere error
	for essai := range essais {
		err := func() error {
			select {
			case enVol <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			defer func() { <-enVol }()
			req, err := http.NewRequestWithContext(ctx, http.MethodPut,
				fmt.Sprintf("%s/api/v1/transfert/%s/bloc/%d", cx.Base, ident, i), bytes.NewReader(donnees))
			if err != nil {
				return err
			}
			req.ContentLength = int64(len(donnees))
			req.Header.Set("Content-Type", "application/octet-stream")
			req.Header.Set("Authorization", "Bearer "+cx.Jeton)
			req.Header.Set("X-Bloc-SHA256", h)
			r, err := cx.Client.Do(req)
			if err != nil {
				return err
			}
			defer r.Body.Close()
			_, _ = io.Copy(io.Discard, r.Body)
			return verifier(r)
		}()
		if err == nil {
			return nil
		}
		var ed *ErreurDistante
		if errors.As(err, &ed) && ed.Statut < 500 && ed.Statut != http.StatusTooManyRequests {
			return err // refus définitif (jeton, bloc invalide) : inutile de réessayer
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		derniere = err
		time.Sleep(time.Duration(500*(1<<essai)) * time.Millisecond)
	}
	return fmt.Errorf("bloc %d : %w ; relance l'envoi pour reprendre", i, derniere)
}

// EnvoyerLien envoie un lien ou un texte à un PC appairé : il y est copié dans le presse-papiers
// (et un lien http(s) y est ouvert si ce PC l'autorise).
func EnvoyerLien(ctx context.Context, reg *appareils.Registre, cible, texte string, source Source) error {
	return EnvoyerTexte(ctx, reg, cible, texte, Options{Source: source})
}

// EnvoyerTexte : comme EnvoyerLien, avec relais possible (o.De, o.Pour).
func EnvoyerTexte(ctx context.Context, reg *appareils.Registre, cible, texte string, o Options) error {
	texte = strings.TrimSpace(texte)
	if texte == "" {
		return errors.New("rien à envoyer")
	}
	cx, err := Ouvrir(ctx, reg, cible, o)
	if err != nil {
		return err
	}
	corps := map[string]string{"texte": texte}
	if securite.URLAutorisee(texte) {
		corps = map[string]string{"url": texte}
	}
	return posterJSON(ctx, cx.Client, cx.Base+"/api/rapide/lien", cx.auth(), corps, nil)
}

// Telephones demande à un PC appairé la liste de ses téléphones (pour les joindre par relais).
func Telephones(ctx context.Context, reg *appareils.Registre, cible string, source Source) ([]string, error) {
	cx, err := Ouvrir(ctx, reg, cible, Options{Source: source})
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", cx.Base+"/api/v1/telephones", nil)
	for k, v := range cx.auth() {
		req.Header.Set(k, v)
	}
	r, err := cx.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if err := verifier(r); err != nil {
		return nil, err
	}
	var c struct {
		Telephones []string `json:"telephones"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&c); err != nil {
		return nil, err
	}
	return c.Telephones, nil
}
