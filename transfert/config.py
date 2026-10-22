"""Configuration : dossiers, ports et réglages, stockés dans config.json."""

import json
import os
import socket
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path

PORT_DEFAUT = 47800            # serveur HTTPS (réseau local)
DECALAGE_CONTROLE = 1          # API de contrôle locale sur 127.0.0.1:(port + 1)
GROUPE_MULTICAST = "239.255.47.80"
PORT_MULTICAST = 47801
TAILLE_BLOC = 4 * 1024 * 1024  # 4 Mio : voir la section V2 de la feuille de route
CONNEXIONS_PARALLELES = 3
DUREE_PARTAGE_S = 600          # lien QR valable 10 min
DUREE_APPAIRAGE_S = 120


def dossier_donnees() -> Path:
    env = os.environ.get("TRANSFERT_DONNEES")
    if env:
        return Path(env)
    if sys.platform == "win32":
        return Path(os.environ.get("APPDATA", str(Path.home()))) / "Transfert"
    return Path.home() / ".config" / "transfert"


def dossier_reception_defaut() -> Path:
    return Path.home() / "Downloads" / "Transfert"


@dataclass
class Config:
    donnees: Path
    nom: str = field(default_factory=socket.gethostname)
    port: int = PORT_DEFAUT
    reception: str = ""
    ouvrir_liens: bool = True
    paralleles: int = CONNEXIONS_PARALLELES
    convertir_heic: bool = False
    convertir_hevc: bool = False
    decouverte: bool = True
    mdns: bool = True

    @property
    def dossier_reception(self) -> Path:
        return Path(self.reception) if self.reception else dossier_reception_defaut()

    @property
    def nom_mdns(self) -> str:
        from .mdns import nom_mdns
        return nom_mdns(self.nom)

    @property
    def port_controle(self) -> int:
        return self.port + DECALAGE_CONTROLE

    def sauver(self) -> None:
        d = asdict(self)
        d.pop("donnees")
        ecrire_json(self.donnees / "config.json", d)


def charger_config(donnees: Path | None = None) -> Config:
    donnees = Path(donnees) if donnees else dossier_donnees()
    donnees.mkdir(parents=True, exist_ok=True)
    chemin = donnees / "config.json"
    cfg = Config(donnees=donnees)
    if chemin.exists():
        valeurs = json.loads(chemin.read_text(encoding="utf-8"))
        for cle, val in valeurs.items():
            if hasattr(cfg, cle) and cle != "donnees":
                setattr(cfg, cle, val)
    else:
        cfg.sauver()
    return cfg


def ecrire_json(chemin: Path, donnees, secret: bool = False) -> None:
    """Écriture atomique : fichier temporaire puis remplacement, pour ne jamais laisser un JSON tronqué."""
    chemin.parent.mkdir(parents=True, exist_ok=True)
    tmp = chemin.with_suffix(chemin.suffix + ".tmp")
    tmp.write_text(json.dumps(donnees, indent=2, ensure_ascii=False), encoding="utf-8")
    if secret and os.name == "posix":
        os.chmod(tmp, 0o600)
    os.replace(tmp, chemin)


def lire_json(chemin: Path, defaut):
    if not chemin.exists():
        return defaut
    return json.loads(chemin.read_text(encoding="utf-8"))
