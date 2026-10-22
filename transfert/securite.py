"""Primitives de sécurité : noms de fichiers, IP autorisées, URL, jetons, engagement et code de vérification.

Aucune primitive cryptographique n'est réimplémentée : tout repose sur hashlib, hmac et secrets.
"""

import hashlib
import hmac
import ipaddress
import re
import secrets
import unicodedata
from pathlib import Path
from urllib.parse import urlsplit

NOMS_RESERVES_WINDOWS = {"CON", "PRN", "AUX", "NUL",
                         *(f"COM{i}" for i in range(1, 10)), *(f"LPT{i}" for i in range(1, 10))}
CARACTERES_INTERDITS = re.compile(r'[<>:"/\\|?*\x00-\x1f\x7f]')


# --- Fichiers -----------------------------------------------------------------

def nom_sur(nom: str) -> str:
    """Réduit un nom reçu du réseau à un nom de fichier simple et sans danger.

    Défend contre : traversée de répertoire (../, chemins absolus, C:\\), noms réservés
    Windows (CON, NUL…), caractères interdits, noms vides ou démesurés.
    """
    nom = unicodedata.normalize("NFC", nom or "")
    nom = re.split(r"[\\/]", nom)[-1]           # garde seulement la dernière composante
    nom = CARACTERES_INTERDITS.sub("_", nom)
    nom = nom.strip(" .")                        # Windows ignore les points et espaces finaux
    if not nom:
        nom = "fichier"
    racine = nom.split(".")[0].upper()
    if racine in NOMS_RESERVES_WINDOWS:
        nom = "_" + nom
    if len(nom) > 200:
        base, point, ext = nom.rpartition(".")
        nom = (base[:190] + "." + ext[:9]) if point and len(ext) <= 9 else nom[:200]
    return nom


def chemin_libre(dossier: Path, nom: str) -> Path:
    """Chemin qui n'écrase jamais un fichier existant : « photo (1).jpg », « photo (2).jpg »…"""
    dossier.mkdir(parents=True, exist_ok=True)
    cible = dossier / nom
    if not cible.exists():
        return cible
    base, ext = Path(nom).stem, Path(nom).suffix
    i = 1
    while (dossier / f"{base} ({i}){ext}").exists():
        i += 1
    return dossier / f"{base} ({i}){ext}"


# --- Réseau -------------------------------------------------------------------

def ip_autorisee(ip: str | None) -> bool:
    """N'accepte que les adresses du réseau local (privées, boucle locale, lien local)."""
    if not ip:
        return False
    try:
        adr = ipaddress.ip_address(ip.split("%")[0])
    except ValueError:
        return False
    if isinstance(adr, ipaddress.IPv6Address) and adr.ipv4_mapped:
        adr = adr.ipv4_mapped
    return adr.is_private or adr.is_loopback or adr.is_link_local


def url_autorisee(url: str) -> bool:
    """Seuls http et https sont ouverts : jamais file:, javascript:, ms-settings:, etc."""
    if not url or len(url) > 2048 or any(ord(c) < 32 for c in url):
        return False
    try:
        morceaux = urlsplit(url.strip())
    except ValueError:
        return False
    return morceaux.scheme in ("http", "https") and bool(morceaux.netloc)


# --- Jetons -------------------------------------------------------------------

def nouveau_jeton() -> str:
    """256 bits d'aléa cryptographique."""
    return secrets.token_urlsafe(32)


def hacher_jeton(jeton: str) -> str:
    """Le serveur ne stocke que le hachage : un vol du fichier ne livre pas les jetons."""
    return hashlib.sha256(jeton.encode()).hexdigest()


def egal_temps_constant(a: str, b: str) -> bool:
    return hmac.compare_digest(a.encode(), b.encode())


# --- Engagement et code de vérification (appairage) ---------------------------

def _h(*morceaux: bytes) -> bytes:
    """SHA-256 d'une concaténation préfixée par les longueurs.

    Le préfixe de longueur rend le découpage non ambigu : sans lui, H(ab || c) = H(a || bc)
    et un attaquant pourrait déplacer des octets d'un champ à l'autre.
    """
    h = hashlib.sha256()
    for m in morceaux:
        h.update(len(m).to_bytes(4, "big"))
        h.update(m)
    return h.digest()


def nouvel_alea() -> bytes:
    return secrets.token_bytes(32)


def engagement(empreinte_b: str, empreinte_a: str, alea_b: bytes) -> str:
    """c_B = H(fp_B || fp_A || r_B) : B fixe r_B avant de connaître r_A."""
    return _h(b"engagement", empreinte_b.encode(), empreinte_a.encode(), alea_b).hex()


def code_verification(empreinte_a: str, empreinte_b: str, alea_a: bytes, alea_b: bytes) -> str:
    """Code à 6 chiffres affiché sur les deux écrans.

    On réduit 64 bits modulo 10^6 : le biais est de l'ordre de 10^6 / 2^64 ≈ 5e-14, négligeable.
    """
    d = _h(b"code", empreinte_a.encode(), empreinte_b.encode(), alea_a, alea_b)
    return f"{int.from_bytes(d[:8], 'big') % 10**6:06d}"
