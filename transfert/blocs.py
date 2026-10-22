"""Moteur de blocs : découpage, vérification et reprise (V2).

Un fichier de N octets est découpé en n = ceil(N / B) blocs de taille B (le dernier peut être plus court).
Chaque bloc porte son SHA-256. Le récepteur tient un fichier d'état listant les blocs validés :
après une coupure, seuls les blocs manquants sont renvoyés.

Intégrité globale : racine = SHA-256(h_0 || h_1 || ... || h_{n-1}). L'émetteur et le récepteur la
calculent chacun de leur côté ; l'égalité prouve que tous les blocs sont les bons et dans le bon ordre.
L'émetteur n'a pas à relire le fichier entier avant d'envoyer : le premier bloc part tout de suite,
ce qui tient la latence de démarrage sous 2 s même pour une grosse vidéo.
"""

import hashlib
import math
import os
import threading
from pathlib import Path

from .config import ecrire_json, lire_json
from .securite import chemin_libre


def nombre_blocs(taille: int, taille_bloc: int) -> int:
    return math.ceil(taille / taille_bloc) if taille else 0


def longueur_bloc(i: int, taille: int, taille_bloc: int) -> int:
    return min(taille_bloc, taille - i * taille_bloc)


def lire_et_hacher(chemin: Path, i: int, taille_bloc: int) -> tuple[bytes, str]:
    """Lit le bloc i et calcule son SHA-256 (appelé dans un thread : hashlib libère le GIL)."""
    with open(chemin, "rb") as f:
        f.seek(i * taille_bloc)
        donnees = f.read(taille_bloc)
    return donnees, hashlib.sha256(donnees).hexdigest()


def racine(hachages: list[str]) -> str:
    h = hashlib.sha256()
    for x in hachages:
        h.update(bytes.fromhex(x))
    return h.hexdigest()


class Reception:
    """État d'une réception en cours, persistant sur disque pour survivre à une coupure."""

    def __init__(self, dossier_tmp: Path, ident: str, meta: dict):
        self.dossier_tmp = dossier_tmp
        self.ident = ident
        self.meta = meta
        self.chemin_part = dossier_tmp / f"{ident}.part"
        self.chemin_etat = dossier_tmp / f"{ident}.json"
        self._verrou = threading.Lock()

    @classmethod
    def ouvrir(cls, dossier_tmp: Path, ident: str, nom: str, taille: int, taille_bloc: int,
               appareil: str) -> "Reception":
        dossier_tmp.mkdir(parents=True, exist_ok=True)
        etat = lire_json(dossier_tmp / f"{ident}.json", None)
        part = dossier_tmp / f"{ident}.part"
        compatible = (etat and etat["taille"] == taille and etat["taille_bloc"] == taille_bloc
                      and etat["appareil"] == appareil and part.exists())
        if not compatible:
            etat = {"nom": nom, "taille": taille, "taille_bloc": taille_bloc, "appareil": appareil,
                    "n": nombre_blocs(taille, taille_bloc), "recus": {}}
            with open(part, "wb") as f:
                f.truncate(taille)       # réserve la taille (fichier creux sur NTFS et ext4)
            ecrire_json(dossier_tmp / f"{ident}.json", etat)
        return cls(dossier_tmp, ident, etat)

    def manquants(self) -> list[int]:
        recus = self.meta["recus"]
        return [i for i in range(self.meta["n"]) if str(i) not in recus]

    def ecrire_bloc(self, i: int, donnees: bytes, sha_annonce: str) -> str | None:
        """Vérifie puis écrit le bloc i. Renvoie None si tout va bien, sinon la raison du refus."""
        n, taille, b = self.meta["n"], self.meta["taille"], self.meta["taille_bloc"]
        if not 0 <= i < n:
            return "indice de bloc hors limites"
        if len(donnees) != longueur_bloc(i, taille, b):
            return "longueur de bloc incorrecte"
        sha = hashlib.sha256(donnees).hexdigest()
        if sha != sha_annonce.lower():
            return "empreinte du bloc incorrecte"
        with self._verrou:
            with open(self.chemin_part, "r+b") as f:
                f.seek(i * b)
                f.write(donnees)
            self.meta["recus"][str(i)] = sha
            ecrire_json(self.chemin_etat, self.meta)
        return None

    def finaliser(self, dossier_reception: Path, racine_emetteur: str) -> tuple[Path | None, str]:
        if self.manquants():
            return None, f"{len(self.manquants())} bloc(s) manquant(s)"
        recus = self.meta["recus"]
        if racine([recus[str(i)] for i in range(self.meta["n"])]) != racine_emetteur.lower():
            return None, "empreinte racine différente"
        with self._verrou:
            final = chemin_libre(dossier_reception, self.meta["nom"])
            os.replace(self.chemin_part, final)
            self.chemin_etat.unlink(missing_ok=True)
        return final, "ok"

    def abandonner(self) -> None:
        self.chemin_part.unlink(missing_ok=True)
        self.chemin_etat.unlink(missing_ok=True)
