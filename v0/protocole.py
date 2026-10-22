"""V0 : protocole TCP minimal, en clair (étape d'apprentissage, remplacée par HTTPS dès la V1).

Framing d'un envoi :
    "TRF0"  (4 octets, signature)
    L       (2 octets, big-endian) longueur du nom en UTF-8
    nom     (L octets)
    taille  (8 octets, big-endian)
    sha256  (32 octets) empreinte du fichier entier
    contenu (taille octets)
Réponse du récepteur : 1 octet (0 = OK, 1 = empreinte fausse, 2 = refus) puis un message UTF-8.

Pourquoi un framing : TCP transporte un flux d'octets sans frontières. Sans longueurs explicites,
le récepteur ne saurait pas où finit le nom et où commence le contenu.
"""

import hashlib
import struct

SIGNATURE = b"TRF0"
ENTETE = struct.Struct(">4sH")
TAILLE = struct.Struct(">Q")
BLOC = 1024 * 1024
TAILLE_MAX_NOM = 1024


def sha256_fichier(chemin) -> bytes:
    h = hashlib.sha256()
    with open(chemin, "rb") as f:
        while morceau := f.read(BLOC):
            h.update(morceau)
    return h.digest()


def recevoir_exactement(sock, n: int) -> bytes:
    """recv() peut rendre moins que demandé : on boucle jusqu'à avoir exactement n octets."""
    morceaux, reste = [], n
    while reste:
        m = sock.recv(min(reste, BLOC))
        if not m:
            raise ConnectionError("connexion fermée avant la fin du message")
        morceaux.append(m)
        reste -= len(m)
    return b"".join(morceaux)
