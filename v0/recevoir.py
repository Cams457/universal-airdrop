"""V0 : reçoit des fichiers envoyés par envoyer.py.

Utilisation (invite cmd) :  python recevoir.py --dossier "%USERPROFILE%\\Downloads\\Transfert"
Attention : V0 est en clair. À n'utiliser que sur ton réseau domestique, le temps des tests.
"""

import argparse
import hashlib
import ipaddress
import os
import shutil
import socket
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from protocole import (BLOC, ENTETE, SIGNATURE, TAILLE, TAILLE_MAX_NOM,  # noqa: E402
                       recevoir_exactement)
from transfert.securite import chemin_libre, nom_sur  # noqa: E402

PORT = 47790


def repondre(conn, code: int, message: str) -> None:
    conn.sendall(bytes([code]) + message.encode("utf-8"))


def traiter(conn, dossier: Path) -> Path | None:
    signature, longueur = ENTETE.unpack(recevoir_exactement(conn, ENTETE.size))
    if signature != SIGNATURE or longueur > TAILLE_MAX_NOM:
        repondre(conn, 2, "en-tête invalide")
        return None
    nom = nom_sur(recevoir_exactement(conn, longueur).decode("utf-8", "replace"))
    (taille,) = TAILLE.unpack(recevoir_exactement(conn, TAILLE.size))
    attendu = recevoir_exactement(conn, 32)

    dossier.mkdir(parents=True, exist_ok=True)
    if shutil.disk_usage(dossier).free < taille:
        repondre(conn, 2, "espace disque insuffisant")
        return None

    temporaire = dossier / f".{nom}.part"
    h = hashlib.sha256()
    reste = taille
    with open(temporaire, "wb") as f:
        while reste:
            morceau = conn.recv(min(reste, BLOC))
            if not morceau:
                f.close()
                temporaire.unlink(missing_ok=True)
                raise ConnectionError("connexion coupée pendant le contenu")
            h.update(morceau)
            f.write(morceau)
            reste -= len(morceau)
    if h.digest() != attendu:
        temporaire.unlink(missing_ok=True)
        repondre(conn, 1, "empreinte SHA-256 différente : fichier rejeté")
        return None
    final = chemin_libre(dossier, nom)
    os.replace(temporaire, final)
    repondre(conn, 0, f"reçu sous {final.name}, SHA-256 vérifié")
    return final


def servir(dossier: Path, port: int = PORT, une_fois: bool = False) -> None:
    with socket.create_server(("0.0.0.0", port)) as serveur:
        serveur.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4 * 1024 * 1024)
        print(f"En attente sur le port {port}, réception dans {dossier}")
        while True:
            conn, (ip, _) = serveur.accept()
            with conn:
                if not ipaddress.ip_address(ip).is_private:
                    repondre(conn, 2, "adresse hors réseau local refusée")
                    continue
                try:
                    final = traiter(conn, dossier)
                    print(f"{ip} : {final.name if final else 'refusé'}")
                except (ConnectionError, OSError) as e:
                    print(f"{ip} : erreur ({e})")
            if une_fois:
                return


def main():
    p = argparse.ArgumentParser(description="V0 : réception TCP de fichiers")
    p.add_argument("--dossier", type=Path, default=Path.home() / "Downloads" / "Transfert")
    p.add_argument("--port", type=int, default=PORT)
    a = p.parse_args()
    servir(a.dossier, a.port)


if __name__ == "__main__":
    main()
