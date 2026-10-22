"""Tests du moteur de blocs : reprise après redémarrage, revérification sur disque."""

import hashlib
import os

from transfert.blocs import Reception, lire_et_hacher, racine

B = 64 * 1024


def test_reprise_apres_redemarrage_et_bloc_perdu(tmp_path):
    src = tmp_path / "src.bin"
    src.write_bytes(os.urandom(5 * B + 100))
    taille = src.stat().st_size
    tmp, recus = tmp_path / "tmp", tmp_path / "recus"

    r = Reception.ouvrir(tmp, "id1", "src.bin", taille, B, "pc:A")
    hach = [lire_et_hacher(src, i, B)[1] for i in range(6)]
    for i in range(4):
        assert r.ecrire_bloc(i, lire_et_hacher(src, i, B)[0], hach[i]) is None
    r.sauvegarder()                              # arrêt propre du démon
    r._fermer()

    # coupure de courant simulée : le bloc 2 noté dans l'état n'a jamais atteint le disque
    with open(tmp / "id1.part", "r+b") as f:
        f.seek(2 * B)
        f.write(b"\0" * B)

    r2 = Reception.ouvrir(tmp, "id1", "src.bin", taille, B, "pc:A")
    assert r2.manquants() == [2, 4, 5]           # le bloc 2 est redemandé, pas validé à tort
    for i in r2.manquants():
        assert r2.ecrire_bloc(i, lire_et_hacher(src, i, B)[0], hach[i]) is None
    final, raison = r2.finaliser(recus, racine(hach))
    assert raison == "ok"
    assert hashlib.sha256(final.read_bytes()).digest() == hashlib.sha256(src.read_bytes()).digest()
    assert not (tmp / "id1.json").exists()


def test_etat_incompatible_repart_de_zero(tmp_path):
    tmp = tmp_path / "tmp"
    r = Reception.ouvrir(tmp, "id2", "a.bin", 3 * B, B, "pc:A")
    r.ecrire_bloc(0, b"x" * B, hashlib.sha256(b"x" * B).hexdigest())
    r.sauvegarder()
    r._fermer()
    autre = Reception.ouvrir(tmp, "id2", "a.bin", 3 * B, B, "pc:AUTRE")   # autre appareil
    assert autre.manquants() == [0, 1, 2]
