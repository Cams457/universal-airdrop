"""Tests d'intégration : deux démons réels sur 127.0.0.1, en HTTPS.

Couvre : appairage par code, envoi dans les deux sens, reprise, bloc corrompu, jetons,
épinglage (anti-MITM), révocation à chaud, API iPhone, liens, partage QR à usage unique.
"""

import asyncio
import hashlib
import os
import socket
from pathlib import Path
from urllib.parse import urlsplit

import aiohttp
import pytest

from transfert.appareils import Appareils
from transfert.blocs import lire_et_hacher
from transfert.client import ErreurTransfert, appairer, envoyer, envoyer_fichier, envoyer_lien
from transfert.config import charger_config
from transfert.controle import appeler
from transfert.identite import contexte_client_epingle
from transfert.serveur import Service

PETIT_BLOC = 256 * 1024


def ports_libres():
    """Un port dont le voisin (port + 1, API de contrôle) est aussi libre."""
    while True:
        with socket.socket() as s:
            s.bind(("127.0.0.1", 0))
            p = s.getsockname()[1]
        try:
            with socket.socket() as s1, socket.socket() as s2:
                s1.bind(("0.0.0.0", p))
                s2.bind(("127.0.0.1", p + 1))
            return p
        except OSError:
            continue


def creer(tmp_path: Path, nom: str):
    cfg = charger_config(tmp_path / nom)
    cfg.nom = nom
    cfg.port = ports_libres()
    cfg.reception = str(tmp_path / f"{nom}-recus")
    cfg.decouverte = False
    cfg.mdns = False
    cfg.ouvrir_liens = False
    cfg.paralleles = 3
    return cfg, Service(cfg, verbeux=False)


async def appairer_ab(A, B, accepter_b=True, egal=None):
    """A s'appaire avec B ; on simule l'utilisateur de B via l'API de contrôle."""
    await asyncio.to_thread(appeler, B.cfg, "POST", "/controle/appairage/ouvrir")

    async def confirmer(code, nom_b, empreinte_b):
        etat = await asyncio.to_thread(appeler, B.cfg, "GET", "/controle/appairage")
        assert etat["etat"] == "a_confirmer" and etat["nom"] == A.cfg.nom
        identiques = etat["code"] == code
        await asyncio.to_thread(appeler, B.cfg, "POST", "/controle/appairage/decision",
                                {"accepter": accepter_b and identiques})
        return identiques if egal is None else egal

    return await appairer(A.cfg, A.identite, A.appareils, "127.0.0.1", B.cfg.port, confirmer)


@pytest.fixture
def deux_pc(tmp_path):
    return creer(tmp_path, "PC-A"), creer(tmp_path, "PC-B")


def fichier_aleatoire(chemin: Path, taille: int) -> Path:
    chemin.write_bytes(os.urandom(taille))
    return chemin


def sha(p: Path) -> str:
    return hashlib.sha256(p.read_bytes()).hexdigest()


def test_appairage_et_envoi_dans_les_deux_sens(tmp_path, deux_pc):
    (cfg_a, A), (cfg_b, B) = deux_pc

    async def scenario():
        await A.demarrer()
        await B.demarrer()
        try:
            r = await appairer_ab(A, B)
            assert r["nom"] == "PC-B"
            assert A.appareils.pc("PC-B") and B.appareils.pc("PC-A")

            src = fichier_aleatoire(tmp_path / "video.mp4", 3 * PETIT_BLOC + 12345)
            bilans = await envoyer(cfg_a, A.identite, A.appareils, "PC-B", [src], taille_bloc=PETIT_BLOC)
            assert bilans[0].blocs_envoyes == bilans[0].blocs_total == 4
            assert sha(cfg_b.dossier_reception / "video.mp4") == sha(src)

            src2 = fichier_aleatoire(tmp_path / "doc.pdf", 1000)
            await envoyer(cfg_b, B.identite, B.appareils, "PC-A", [src2], taille_bloc=PETIT_BLOC)
            assert sha(cfg_a.dossier_reception / "doc.pdf") == sha(src2)

            vide = tmp_path / "vide.txt"
            vide.write_bytes(b"")
            await envoyer(cfg_a, A.identite, A.appareils, "PC-B", [vide], taille_bloc=PETIT_BLOC)
            assert (cfg_b.dossier_reception / "vide.txt").read_bytes() == b""

            await envoyer_lien(cfg_a, A.appareils, "PC-B", "https://exemple.fr/")
            with pytest.raises(ErreurTransfert):
                await envoyer_lien(cfg_a, A.appareils, "PC-B", "javascript:alert(1)")
        finally:
            await A.arreter()
            await B.arreter()

    asyncio.run(scenario())


def test_codes_differents_refuse(tmp_path, deux_pc):
    (cfg_a, A), (cfg_b, B) = deux_pc

    async def scenario():
        await A.demarrer()
        await B.demarrer()
        try:
            with pytest.raises(ErreurTransfert):
                await appairer_ab(A, B, accepter_b=False, egal=False)
            assert not A.appareils.pc("PC-B") and not B.appareils.pc("PC-A")
        finally:
            await A.arreter()
            await B.arreter()

    asyncio.run(scenario())


def test_appairage_sans_fenetre_refuse(tmp_path, deux_pc):
    (cfg_a, A), (cfg_b, B) = deux_pc

    async def scenario():
        await B.demarrer()
        try:
            with pytest.raises(ErreurTransfert, match="403"):
                await appairer(cfg_a, A.identite, A.appareils, "127.0.0.1", cfg_b.port, lambda *a: True)
        finally:
            await B.arreter()

    asyncio.run(scenario())


def test_reprise_bloc_corrompu_et_jetons(tmp_path, deux_pc):
    (cfg_a, A), (cfg_b, B) = deux_pc

    async def scenario():
        await A.demarrer()
        await B.demarrer()
        try:
            await appairer_ab(A, B)
            _, info = A.appareils.pc("PC-B")
            ctx = contexte_client_epingle(info["ca_pem"])
            base = f"https://127.0.0.1:{cfg_b.port}"
            src = fichier_aleatoire(tmp_path / "gros.bin", 8 * PETIT_BLOC)
            auth = {"Authorization": f"Bearer {info['jeton_sortant']}"}
            async with aiohttp.ClientSession(connector=aiohttp.TCPConnector(ssl=ctx)) as s:
                # coupure simulée : seuls les blocs 0 à 2 arrivent
                st = src.stat()
                cle = hashlib.sha256(f"{src.resolve()}|{st.st_size}|{st.st_mtime_ns}".encode()).hexdigest()
                async with s.post(f"{base}/api/v1/transfert/preparer", headers=auth, json={
                        "nom": src.name, "taille": st.st_size, "taille_bloc": PETIT_BLOC, "cle": cle}) as r:
                    ident = (await r.json())["id"]
                for i in range(3):
                    donnees, h = lire_et_hacher(src, i, PETIT_BLOC)
                    async with s.put(f"{base}/api/v1/transfert/{ident}/bloc/{i}", data=donnees,
                                     headers={**auth, "X-Bloc-SHA256": h}) as r:
                        assert r.status == 204
                # bloc corrompu : refusé
                donnees, h = lire_et_hacher(src, 3, PETIT_BLOC)
                async with s.put(f"{base}/api/v1/transfert/{ident}/bloc/3", data=b"x" + donnees[1:],
                                 headers={**auth, "X-Bloc-SHA256": h}) as r:
                    assert r.status == 422
                # reprise : seuls les 5 blocs manquants partent
                bilan = await envoyer_fichier(s, base, info["jeton_sortant"], src, 3, PETIT_BLOC)
                assert (bilan.blocs_envoyes, bilan.blocs_total) == (5, 8)
                assert sha(cfg_b.dossier_reception / "gros.bin") == sha(src)

                # mauvais jeton : 401, puis 429 après 20 échecs
                for _ in range(20):
                    async with s.post(f"{base}/api/v1/transfert/preparer",
                                      headers={"Authorization": "Bearer faux"}, json={}) as r:
                        assert r.status == 401
                async with s.post(f"{base}/api/v1/transfert/preparer", headers=auth, json={}) as r:
                    assert r.status == 429
        finally:
            await A.arreter()
            await B.arreter()

    asyncio.run(scenario())


def test_epinglage_refuse_un_autre_serveur(tmp_path, deux_pc):
    """Un client épinglé sur la CA de A refuse de parler à B : c'est la défense anti-MITM."""
    (cfg_a, A), (cfg_b, B) = deux_pc

    async def scenario():
        await B.demarrer()
        try:
            ctx_a = contexte_client_epingle(A.identite.ca_pem)
            async with aiohttp.ClientSession() as s:
                with pytest.raises(aiohttp.ClientConnectorCertificateError):
                    await s.get(f"https://127.0.0.1:{cfg_b.port}/api/v1/info", ssl=ctx_a)
        finally:
            await B.arreter()

    asyncio.run(scenario())


def test_revocation_a_chaud(tmp_path, deux_pc):
    (cfg_a, A), (cfg_b, B) = deux_pc

    async def scenario():
        await A.demarrer()
        await B.demarrer()
        try:
            await appairer_ab(A, B)
            src = fichier_aleatoire(tmp_path / "a.txt", 10)
            await envoyer(cfg_a, A.identite, A.appareils, "PC-B", [src])
            # révocation par la ligne de commande (autre instance du registre), démon toujours lancé
            assert Appareils(cfg_b.donnees).revoquer("PC-A") >= 1
            with pytest.raises(ErreurTransfert, match="401"):
                await envoyer(cfg_a, A.identite, A.appareils, "PC-B", [src])
        finally:
            await A.arreter()
            await B.arreter()

    asyncio.run(scenario())


def test_api_iphone_et_partage(tmp_path, deux_pc):
    (cfg_a, A), (cfg_b, B) = deux_pc

    async def scenario():
        await B.demarrer()
        try:
            jeton = B.appareils.ajouter_iphone("iPhone de test")
            ctx = contexte_client_epingle(B.identite.ca_pem)
            base = f"https://127.0.0.1:{cfg_b.port}"
            auth = {"Authorization": f"Bearer {jeton}"}
            photo = os.urandom(300_000)
            async with aiohttp.ClientSession(connector=aiohttp.TCPConnector(ssl=ctx)) as s:
                # formulaire multipart, comme le Raccourci « Formulaire »
                form = aiohttp.FormData()
                form.add_field("fichier", photo, filename="IMG_0001.HEIC", content_type="image/heic")
                form.add_field("fichier", b"%PDF-1.4", filename="cours.pdf", content_type="application/pdf")
                async with s.post(f"{base}/api/rapide/fichier", data=form, headers=auth) as r:
                    assert r.status == 200, await r.text()
                    assert (await r.json())["recus"] == ["IMG_0001.HEIC", "cours.pdf"]
                assert (cfg_b.dossier_reception / "IMG_0001.HEIC").read_bytes() == photo
                # corps brut + en-tête X-Nom (URL-encodé), comme le Raccourci « Fichier »
                async with s.post(f"{base}/api/rapide/fichier", data=b"video",
                                  headers={**auth, "X-Nom": "vid%C3%A9o.mov"}) as r:
                    assert r.status == 200
                assert (cfg_b.dossier_reception / "vidéo.mov").read_bytes() == b"video"
                # sans jeton : refusé
                async with s.post(f"{base}/api/rapide/fichier", data=b"x") as r:
                    assert r.status == 401
                # liens
                async with s.post(f"{base}/api/rapide/lien", json={"url": "https://exemple.fr"}, headers=auth) as r:
                    assert r.status == 200
                async with s.post(f"{base}/api/rapide/lien", data="file:///C:/Windows/system32",
                                  headers={**auth, "Content-Type": "text/plain"}) as r:
                    assert r.status == 400

                # partage PC -> iPhone, usage unique
                doc = fichier_aleatoire(tmp_path / "notes.pdf", 50_000)
                d = await asyncio.to_thread(appeler, cfg_b, "POST", "/controle/partage", {"chemins": [str(doc)]})
                chemin = urlsplit(d["urls"][0]["url"]).path if d["urls"] else None
                if chemin is None:      # machine sans IP réseau : on récupère le jeton autrement
                    chemin = "/p/" + next(iter(B.partages._p))
                async with s.get(f"{base}{chemin}") as r:
                    assert r.status == 200 and "notes.pdf" in await r.text()
                    assert "default-src 'none'" in r.headers["Content-Security-Policy"]
                async with s.get(f"{base}{chemin}/0") as r:
                    assert r.status == 200 and await r.read() == doc.read_bytes()
                async with s.get(f"{base}{chemin}") as r:
                    assert r.status == 404
                # texte (jeton) à usage unique
                await asyncio.to_thread(appeler, cfg_b, "POST", "/controle/partage", {"texte": "SECRET-123"})
                jt = list(B.partages._p)[-1]
                async with s.get(f"{base}/p/{jt}") as r:
                    assert "SECRET-123" in await r.text()
                async with s.get(f"{base}/p/{jt}") as r:
                    assert r.status == 404
        finally:
            await B.arreter()

    asyncio.run(scenario())


def test_controle_exige_le_secret(tmp_path, deux_pc):
    (cfg_a, A), (cfg_b, B) = deux_pc

    async def scenario():
        await B.demarrer()
        try:
            async with aiohttp.ClientSession() as s:
                async with s.get(f"http://127.0.0.1:{cfg_b.port_controle}/controle/etat") as r:
                    assert r.status == 401
                async with s.get(f"http://127.0.0.1:{cfg_b.port_controle}/controle/etat",
                                 headers={"X-Controle": "devine"}) as r:
                    assert r.status == 401
        finally:
            await B.arreter()

    asyncio.run(scenario())
