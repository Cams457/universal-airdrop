"""Partages PC -> iPhone (V3) : un lien à jeton, scanné en QR code, valable 10 minutes.

Le jeton fait 128 bits : deviner une URL valide a une probabilité de 2^-128 par essai.
L'URL ne contient que ce jeton, jamais de nom de fichier : rien ne fuit dans un historique.
"""

import secrets
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path

from .config import DUREE_PARTAGE_S


@dataclass
class Partage:
    fichiers: list[Path]
    url: str | None
    expire: float
    texte: str | None = None
    telecharges: set[int] = field(default_factory=set)

    @property
    def complet(self) -> bool:
        return bool(self.fichiers) and len(self.telecharges) == len(self.fichiers)


class Partages:
    def __init__(self):
        self._p: dict[str, Partage] = {}
        self._verrou = threading.Lock()

    def creer(self, fichiers: list[Path], url: str | None = None, duree: float = DUREE_PARTAGE_S,
              texte: str | None = None) -> str:
        jeton = secrets.token_urlsafe(16)
        with self._verrou:
            self._p[jeton] = Partage([Path(f) for f in fichiers], url, time.monotonic() + duree, texte)
        return jeton

    def consommer(self, jeton: str) -> None:
        """Un partage de texte (jeton d'accès) ne s'affiche qu'une seule fois."""
        with self._verrou:
            self._p.pop(jeton, None)

    def obtenir(self, jeton: str) -> Partage | None:
        with self._verrou:
            p = self._p.get(jeton)
            if p and time.monotonic() > p.expire:
                del self._p[jeton]
                return None
            return p

    def marquer(self, jeton: str, i: int) -> None:
        """Après téléchargement complet de chaque fichier, le partage disparaît : usage unique."""
        with self._verrou:
            p = self._p.get(jeton)
            if not p:
                return
            p.telecharges.add(i)
            if p.complet and not p.url:
                del self._p[jeton]

    def nettoyer(self) -> None:
        maintenant = time.monotonic()
        with self._verrou:
            for j in [j for j, p in self._p.items() if maintenant > p.expire]:
                del self._p[j]
