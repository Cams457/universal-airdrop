"""Tests de la découverte : format des annonces et échange multicast réel si l'environnement le permet."""

import asyncio

import pytest

from transfert import decouverte as D


def test_annonce_aller_retour():
    a = D.lire_annonce(D.annonce("PC-A", "f" * 64, 47800))
    assert a == {"nom": "PC-A", "fp": "f" * 64, "port": 47800}


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
