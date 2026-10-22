package serveur

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Cams457/universal-airdrop/internal/securite"
)

// tri : le port public ne parle que HTTPS, mais Safari ouvre « pc.local:47800/iphone » en http://
// quand on tape l'adresse sans https://. Le premier octet d'une connexion TLS vaut 0x16 (en-tête
// d'enregistrement « handshake ») ; une requête HTTP en clair commence par une lettre (« GET »).
// On regarde ce premier octet : TLS est servi normalement, HTTP reçoit une redirection vers https://.
type tri struct {
	net.Listener
	tls    chan net.Conn
	fin    chan struct{}
	err    error
	unique sync.Once
	hotes  func() []string // noms acceptés pour la redirection (anti redirection ouverte)
}

func nouveauTri(l net.Listener, hotes func() []string) *tri {
	t := &tri{Listener: l, tls: make(chan net.Conn), fin: make(chan struct{}), hotes: hotes}
	go t.boucle()
	return t
}

func (t *tri) boucle() {
	for {
		c, err := t.Listener.Accept()
		if err != nil {
			t.unique.Do(func() { t.err = err; close(t.fin) })
			return
		}
		go t.trier(c)
	}
}

// connLue : connexion dont le début a déjà été lu (dans le tampon).
type connLue struct {
	net.Conn
	r *bufio.Reader
}

func (c *connLue) Read(b []byte) (int, error) { return c.r.Read(b) }

func (t *tri) trier(c net.Conn) {
	ip, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	if !securite.IPAutorisee(ip) {
		c.Close()
		return
	}
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(c)
	premier, err := br.Peek(1)
	if err != nil {
		c.Close()
		return
	}
	c.SetReadDeadline(time.Time{})
	lue := &connLue{Conn: c, r: br}
	if premier[0] == 0x16 {
		select {
		case t.tls <- lue:
		case <-t.fin:
			c.Close()
		}
		return
	}
	t.rediriger(lue, br)
}

func (t *tri) rediriger(c net.Conn, br *bufio.Reader) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	hote, port, err := net.SplitHostPort(req.Host)
	if err != nil {
		hote, port = req.Host, ""
	}
	connu := net.ParseIP(hote) != nil && securite.IPAutorisee(hote)
	for _, h := range t.hotes() {
		connu = connu || strings.EqualFold(hote, h)
	}
	if !connu || port == "" {
		corps := "Tape https:// devant l'adresse de ce PC.\r\n"
		fmt.Fprintf(c, "HTTP/1.1 400 Bad Request\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s", len(corps), corps)
		return
	}
	cible := "https://" + net.JoinHostPort(hote, port) + req.URL.RequestURI()
	corps := "Redirection vers " + cible + "\r\n"
	fmt.Fprintf(c, "HTTP/1.1 301 Moved Permanently\r\nLocation: %s\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s", cible, len(corps), corps)
}

// Accept renvoie les connexions TLS seulement.
func (t *tri) Accept() (net.Conn, error) {
	select {
	case c := <-t.tls:
		return c, nil
	case <-t.fin:
		return nil, t.err
	}
}
