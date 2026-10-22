package identite

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestIdentiteEtReemission(t *testing.T) {
	d := t.TempDir()
	ips := []string{"192.168.1.20"}
	id, err := Ouvrir(d, "PC de Camil", []string{"pc-de-camil.local"}, func() []string { return ips })
	if err != nil {
		t.Fatal(err)
	}
	if len(id.Empreinte) != 64 {
		t.Fatal("empreinte")
	}
	f := id.feuille
	if !slices.Contains(f.DNSNames, "pc-de-camil.local") || f.NotAfter.Sub(f.NotBefore) > 825*24*time.Hour {
		t.Fatal("certificat serveur non conforme aux exigences iOS")
	}
	if err := f.CheckSignatureFrom(id.CA); err != nil {
		t.Fatal(err)
	}
	// partage de connexion activé : nouvelle IP -> réémission, CA inchangée
	ips = append(ips, "172.20.10.4")
	if neuf, err := id.VerifierServeur(); err != nil || !neuf {
		t.Fatal("aurait dû réémettre")
	}
	trouve := false
	for _, a := range id.feuille.IPAddresses {
		trouve = trouve || a.Equal(net.ParseIP("172.20.10.4"))
	}
	if !trouve {
		t.Fatal("nouvelle IP absente du SAN")
	}
	if neuf, _ := id.VerifierServeur(); neuf {
		t.Fatal("ne devait pas réémettre")
	}
	id2, _ := Ouvrir(d, "PC de Camil", nil, func() []string { return ips })
	if id2.Empreinte != id.Empreinte {
		t.Fatal("la CA doit rester stable")
	}
}

func TestEpinglage(t *testing.T) {
	a, _ := Ouvrir(t.TempDir(), "A", nil, nil)
	b, _ := Ouvrir(t.TempDir(), "B", nil, nil)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	srv.Listener = tls.NewListener(srv.Listener, b.ConfigServeur()) // notre configuration seule, comme le démon
	srv.Start()
	defer srv.Close()
	srv.URL = "https" + srv.URL[len("http"):]

	bon, _ := ConfigClientEpingle(b.CAPEM)
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: bon}}
	if r, err := c.Get(srv.URL); err != nil {
		t.Fatal(err)
	} else {
		r.Body.Close()
	}
	mauvais, _ := ConfigClientEpingle(a.CAPEM) // épinglé sur A : doit refuser B (MITM)
	c = &http.Client{Transport: &http.Transport{TLSClientConfig: mauvais}}
	if _, err := c.Get(srv.URL); err == nil {
		t.Fatal("un client épinglé sur A ne doit pas accepter B")
	}
}
