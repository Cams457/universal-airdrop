"""Registre des appareils de confiance : PC appairés (CA épinglée) et jetons d'accès (iPhone et PC).

Structure de appareils.json :
- "pcs"    : empreinte de CA -> {nom, ca_pem, adresse, port, jeton_sortant}
             jeton_sortant = le jeton que CE PC présente quand il envoie vers ce pair ;
- "jetons" : SHA-256 du jeton -> {nom, type, empreinte}
             jetons que CE PC accepte. Seul le hachage est stocké.

Limite connue : jeton_sortant est stocké en clair dans le profil utilisateur (comme un mot de passe
enregistré). Un logiciel malveillant déjà présent sur le PC pourrait le lire : c'est hors du modèle
de menace. Extension possible : chiffrement par DPAPI sous Windows.
"""

import datetime as dt
import threading
from pathlib import Path

from .config import ecrire_json, lire_json
from .securite import egal_temps_constant, hacher_jeton, nouveau_jeton


class Appareils:
    def __init__(self, dossier: Path):
        self.chemin = Path(dossier) / "appareils.json"
        self._verrou = threading.RLock()
        self._mtime = None
        self.donnees = {"pcs": {}, "jetons": {}}
        self._rafraichir()

    def _mtime_fichier(self):
        try:
            return self.chemin.stat().st_mtime_ns
        except FileNotFoundError:
            return None

    def _rafraichir(self) -> None:
        """Relit le fichier s'il a changé : une révocation faite en ligne de commande s'applique
        immédiatement au démon, sans redémarrage."""
        with self._verrou:
            m = self._mtime_fichier()
            if m != self._mtime:
                self.donnees = lire_json(self.chemin, {"pcs": {}, "jetons": {}})
                self._mtime = m

    def _sauver(self) -> None:
        ecrire_json(self.chemin, self.donnees, secret=True)
        self._mtime = self._mtime_fichier()

    def recharger(self) -> None:
        with self._verrou:
            self._mtime = None
            self._rafraichir()

    # --- PC appairés ----------------------------------------------------------

    def ajouter_pc(self, empreinte: str, nom: str, ca_pem: str, adresse: str, port: int,
                   jeton_sortant: str, jeton_entrant: str) -> None:
        with self._verrou:
            self._rafraichir()
            self._retirer_jetons_de(empreinte)
            self.donnees["pcs"][empreinte] = {
                "nom": nom, "ca_pem": ca_pem, "adresse": adresse, "port": port,
                "jeton_sortant": jeton_sortant, "ajoute": dt.datetime.now().isoformat(timespec="seconds"),
            }
            self.donnees["jetons"][hacher_jeton(jeton_entrant)] = {
                "nom": nom, "type": "pc", "empreinte": empreinte}
            self._sauver()

    def mettre_a_jour_adresse(self, empreinte: str, adresse: str, port: int) -> None:
        with self._verrou:
            self._rafraichir()
            pc = self.donnees["pcs"].get(empreinte)
            if pc and (pc["adresse"], pc["port"]) != (adresse, port):
                pc["adresse"], pc["port"] = adresse, port
                self._sauver()

    def pc(self, cible: str) -> tuple[str, dict] | None:
        """Retrouve un PC par nom (insensible à la casse), empreinte, ou adresse."""
        self._rafraichir()
        pcs = self.donnees["pcs"]
        if cible in pcs:
            return cible, pcs[cible]
        for fp, info in pcs.items():
            if info["nom"].lower() == cible.lower() or info["adresse"] == cible.split(":")[0]:
                return fp, info
        return None

    # --- jetons ---------------------------------------------------------------

    def ajouter_iphone(self, nom: str) -> str:
        jeton = nouveau_jeton()
        with self._verrou:
            self._rafraichir()
            self.donnees["jetons"][hacher_jeton(jeton)] = {
                "nom": nom, "type": "iphone", "empreinte": None,
                "cree": dt.datetime.now().isoformat(timespec="seconds")}
            self._sauver()
        return jeton

    def verifier_jeton(self, jeton: str | None) -> dict | None:
        """Renvoie l'appareil associé au jeton, ou None.

        La recherche porte sur le hachage du jeton : un attaquant ne peut rien déduire du temps
        de recherche, puisqu'il faudrait trouver une préimage de SHA-256.
        """
        if not jeton or len(jeton) > 256:
            return None
        h = hacher_jeton(jeton)
        with self._verrou:
            self._rafraichir()
            for cle, info in self.donnees["jetons"].items():
                if egal_temps_constant(cle, h):
                    return dict(info)
        return None

    def revoquer(self, nom: str) -> int:
        """Supprime tous les jetons et appairages portant ce nom. Renvoie le nombre d'entrées retirées."""
        with self._verrou:
            self._rafraichir()
            n = 0
            for cle in [k for k, v in self.donnees["jetons"].items() if v["nom"].lower() == nom.lower()]:
                del self.donnees["jetons"][cle]
                n += 1
            for fp in [k for k, v in self.donnees["pcs"].items() if v["nom"].lower() == nom.lower()]:
                del self.donnees["pcs"][fp]
                n += 1
            self._sauver()
            return n

    def _retirer_jetons_de(self, empreinte: str) -> None:
        for cle in [k for k, v in self.donnees["jetons"].items() if v.get("empreinte") == empreinte]:
            del self.donnees["jetons"][cle]

    def liste(self) -> list[dict]:
        self._rafraichir()
        sortie = [{"nom": v["nom"], "type": "pc", "adresse": f'{v["adresse"]}:{v["port"]}', "empreinte": fp}
                  for fp, v in self.donnees["pcs"].items()]
        sortie += [{"nom": v["nom"], "type": "iphone", "adresse": "", "empreinte": ""}
                   for v in self.donnees["jetons"].values() if v["type"] == "iphone"]
        return sortie
