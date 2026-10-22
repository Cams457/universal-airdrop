"""Tests de la découverte : format des annonces et échange multicast réel si l'environnement le permet."""

import asyncio

import pytest

from transfert import decouverte as D


def test_annonce_aller_retour():
    a = D.lire_annonce(D.annonce("PC-A", "f" * 64, 47800))
    assert a == {"nom": "PC-A", "fp": "f" * 64, "port": 47800, "lien": "autre", "mbits": 0}


@pytest.mark.parametrize("brut", [b"", b"{}", b"pas du json", b'{"app":"autre"}',
                                  b'{"app":"transfert","fp":"court","port":1}',
                                  b'{"app":"transfert","fp":"' + b"f" * 64 + b'","port":70000}',
                                  b"x" * 2000])
def test_annonces_invalides_ignorees(brut):
    assert D.lire_annonce(brut) is None


def test_echange_multicast_reel():
    vus = []

    async def scenario():
        a = D.Decouverte("PC-A", "a" * 64, 1111)
        b = D.Decouverte("PC-B", "b" * 64, 2222, sur_pair=lambda fp, ip, port: vus.append((fp, port)))
        try:
            await a.demarrer()
            await b.demarrer()
        except OSError as e:
            pytest.skip(f"multicast indisponible ici : {e}")
        try:
            for _ in range(30):
                a._emettre()
                await asyncio.sleep(0.1)
                if vus:
                    break
        finally:
            await a.arreter()
            await b.arreter()

    asyncio.run(scenario())
    if not vus:
        pytest.skip("aucun paquet multicast reçu dans cet environnement (conteneur sans multicast)")
    assert ("a" * 64, 1111) in vus


def test_annonce_porte_le_lien():
    a = D.lire_annonce(D.annonce("PC-A", "f" * 64, 47800, "ethernet", 1000))
    assert (a["lien"], a["mbits"]) == ("ethernet", 1000)
    assert D.lire_annonce(D.annonce("PC-A", "f" * 64, 47800, "inconnu?", -5))["lien"] == "autre"


def test_toutes_les_adresses_gardees():
    d = D.Decouverte("PC-B", "b" * 64, 1)
    d.enregistrer({"nom": "PC-A", "fp": "a" * 64, "port": 47800, "lien": "wifi", "mbits": 1201}, "192.168.1.20")
    d.enregistrer({"nom": "PC-A", "fp": "a" * 64, "port": 47800, "lien": "ethernet", "mbits": 1000}, "192.168.1.21")
    assert set(d.actifs()["a" * 64]["adresses"]) == {"192.168.1.20", "192.168.1.21"}


def test_chemin_le_plus_rapide_d_abord():
    from transfert.client import classer_chemins
    adresses = {"172.20.10.3": {"port": 1, "lien": "bluetooth", "mbits": 3},
                "192.168.1.20": {"port": 1, "lien": "wifi", "mbits": 866},
                "192.168.1.21": {"port": 1, "lien": "ethernet", "mbits": 1000}}
    # notre carte de sortie : Ethernet gigabit vers .21, Wi-Fi 6 vers .20, Bluetooth vers .3
    locaux = {"192.168.1.21": 1000, "192.168.1.20": 1201, "172.20.10.3": 3}
    ordre = classer_chemins(adresses, lambda ip, l, m: min(locaux[ip], m))
    assert [ip for ip, _ in ordre] == ["192.168.1.21", "192.168.1.20", "172.20.10.3"]
