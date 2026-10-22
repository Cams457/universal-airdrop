"""V0 : envoie un fichier à un PC qui exécute recevoir.py.

Utilisation (invite cmd) :  python envoyer.py 192.168.1.20 "C:\\chemin\\video.mp4"
"""

import argparse
import socket
import sys
import time
from pathlib import Path

from protocole import ENTETE, SIGNATURE, TAILLE, recevoir_exactement, sha256_fichier

PORT = 47790


def envoyer(hote: str, chemin: Path, port: int = PORT) -> tuple[int, str, float]:
    taille = chemin.stat().st_size
    empreinte = sha256_fichier(chemin)
    nom = chemin.name.encode("utf-8")
    t0 = time.perf_counter()
    with socket.create_connection((hote, port), timeout=30) as s:
        s.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 4 * 1024 * 1024)
        s.sendall(ENTETE.pack(SIGNATURE, len(nom)) + nom + TAILLE.pack(taille) + empreinte)
        with open(chemin, "rb") as f:
            s.sendfile(f)    # copie noyau quand c'est possible (TransmitFile sous Windows)
        s.shutdown(socket.SHUT_WR)
        code = recevoir_exactement(s, 1)[0]
        message = s.recv(4096).decode("utf-8", "replace")
    duree = time.perf_counter() - t0
    return code, message, duree


def main():
    p = argparse.ArgumentParser(description="V0 : envoi TCP d'un fichier")
    p.add_argument("hote")
    p.add_argument("fichier", type=Path)
    p.add_argument("--port", type=int, default=PORT)
    a = p.parse_args()
    if not a.fichier.is_file():
        sys.exit(f"Fichier introuvable : {a.fichier}")
    code, message, duree = envoyer(a.hote, a.fichier, a.port)
    taille = a.fichier.stat().st_size
    debit = 8 * taille / duree / 1e6 if duree > 0 else 0
    print(f"{'Succès' if code == 0 else 'Échec'} : {message}")
    print(f"{taille / 1e6:.1f} Mo en {duree:.2f} s, soit {debit:.0f} Mbit/s")
    sys.exit(code)


if __name__ == "__main__":
    main()
