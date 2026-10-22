package mdns

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestAnnonceEtResolution(t *testing.T) {
	a := Nouvelle("PC de Camil", "pc-de-camil", strings.Repeat("f", 64), 47800)
	if err := a.Demarrer(); err != nil {
		t.Skipf("mDNS indisponible : %v", err)
	}
	defer a.Arreter()
	r, err := zeroconf.NewResolver(zeroconf.SelectIPTraffic(zeroconf.IPv4))
	if err != nil {
		t.Skip(err)
	}
	entrees := make(chan *zeroconf.ServiceEntry, 4)
	ctx, fin := context.WithTimeout(context.Background(), 3*time.Second)
	defer fin()
	if err := r.Lookup(ctx, "PC de Camil", Service, "local.", entrees); err != nil {
		t.Skip(err)
	}
	select {
	case e := <-entrees:
		if e.HostName != "pc-de-camil.local." || e.Port != 47800 {
			t.Fatalf("annonce inattendue : %s:%d", e.HostName, e.Port)
		}
	case <-ctx.Done():
		t.Skip("aucune réponse mDNS (environnement sans multicast)")
	}
}
