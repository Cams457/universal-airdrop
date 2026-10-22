"""Tests V0 : envoi TCP en boucle locale, fichier corrompu, nom piégé."""

import hashlib
import os
import socket
import sys
import threading
from pathlib import Path

RACINE = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(RACINE / "v0"))
sys.path.insert(0, str(RACINE))

import envoyer  # noqa: E402
import recevoir  # noqa: E402
from protocole import ENTETE, SIGNATURE, TAILLE, recevoir_exactement  # noqa: E402


def port_libre():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def test_envoi_normal(tmp_path):
    src = tmp_path / "video.bin"
    src.write_bytes(os.urandom(5 * 1024 * 1024 + 123))
    dossier = tmp_path / "recus"
    port = port_libre()
    t = threading.Thread(target=recevoir.servir, args=(dossier, port, True), daemon=True)
    t.start()
    threading.Event().wait(0.2)
    code, message, _ = envoyer.envoyer("127.0.0.1", src, port)
    t.join(5)
    assert code == 0, message
    assert (dossier / "video.bin").read_bytes() == src.read_bytes()


def test_empreinte_fausse_rejetee(tmp_path):
    dossier = tmp_path / "recus"
    port = port_libre()
    t = threading.Thread(target=recevoir.servir, args=(dossier, port, True), daemon=True)
    t.start()
    threading.Event().wait(0.2)
    contenu = b"bonjour"
    nom = b"a.txt"
    with socket.create_connection(("127.0.0.1", port)) as s:
        s.sendall(ENTETE.pack(SIGNATURE, len(nom)) + nom + TAILLE.pack(len(contenu))
                  + hashlib.sha256(b"autre chose").digest() + contenu)
        code = recevoir_exactement(s, 1)[0]
    t.join(5)
    assert code == 1
    assert not any(dossier.iterdir())


def test_nom_piege_neutralise(tmp_path):
    src = tmp_path / "x.txt"
    src.write_bytes(b"contenu")
    dossier = tmp_path / "recus"
    port = port_libre()
    t = threading.Thread(target=recevoir.servir, args=(dossier, port, True), daemon=True)
    t.start()
    threading.Event().wait(0.2)
    nom = "../../evil.txt".encode()
    with socket.create_connection(("127.0.0.1", port)) as s:
        s.sendall(ENTETE.pack(SIGNATURE, len(nom)) + nom + TAILLE.pack(7)
                  + hashlib.sha256(b"contenu").digest() + b"contenu")
        code = recevoir_exactement(s, 1)[0]
    t.join(5)
    assert code == 0
    assert (dossier / "evil.txt").exists()
    assert not (tmp_path.parent / "evil.txt").exists()


def test_pas_d_ecrasement(tmp_path):
    src = tmp_path / "doc.pdf"
    src.write_bytes(b"v2")
    dossier = tmp_path / "recus"
    dossier.mkdir()
    (dossier / "doc.pdf").write_bytes(b"v1")
    port = port_libre()
    t = threading.Thread(target=recevoir.servir, args=(dossier, port, True), daemon=True)
    t.start()
    threading.Event().wait(0.2)
    code, _, _ = envoyer.envoyer("127.0.0.1", src, port)
    t.join(5)
    assert code == 0
    assert (dossier / "doc.pdf").read_bytes() == b"v1"
    assert (dossier / "doc (1).pdf").read_bytes() == b"v2"
