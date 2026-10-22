"""Accès de la ligne de commande à l'API de contrôle du démon (127.0.0.1 uniquement)."""

import json
import urllib.error
import urllib.request
from pathlib import Path

from .config import Config, lire_json

# Le contrôle est local : on ignore tout proxy configuré dans l'environnement.
_OUVREUR = urllib.request.build_opener(urllib.request.ProxyHandler({}))


class DemonAbsent(Exception):
    pass


def appeler(cfg: Config, methode: str, chemin: str, corps: dict | None = None, timeout: float = 10):
    info = lire_json(Path(cfg.donnees) / "controle.json", None)
    if not info:
        raise DemonAbsent("le démon ne tourne pas : lance d'abord « python -m transfert demon »")
    requete = urllib.request.Request(
        f"http://127.0.0.1:{info['port']}{chemin}", method=methode,
        data=json.dumps(corps).encode() if corps is not None else None,
        headers={"X-Controle": info["secret"], "Content-Type": "application/json"})
    try:
        with _OUVREUR.open(requete, timeout=timeout) as r:
            return json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        try:
            message = json.loads(e.read()).get("erreur", e.reason)
        except ValueError:
            message = e.reason
        raise RuntimeError(f"{e.code} : {message}") from None
    except (urllib.error.URLError, ConnectionError, TimeoutError) as e:
        raise DemonAbsent(f"le démon ne répond pas ({e}) : est-il lancé ?") from None


def demon_actif(cfg: Config) -> bool:
    try:
        appeler(cfg, "GET", "/controle/etat", timeout=2)
        return True
    except (DemonAbsent, RuntimeError):
        return False
