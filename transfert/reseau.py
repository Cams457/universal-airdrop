"""Adresses IP du PC, interface par interface.

Un PC peut avoir en même temps :
- une IP de box (192.168.1.x) en Wi-Fi ou Ethernet ;
- une IP du partage de connexion de l'iPhone (172.20.10.x), reçue en Wi-Fi, en USB ou en Bluetooth ;
- l'IP 192.168.137.1 s'il héberge lui-même un point d'accès mobile Windows.

Le Bluetooth (PAN) se reconnaît au nom de l'interface Windows (« Connexion réseau Bluetooth »).
Son débit utile est de l'ordre de 1 à 2 Mbit/s : on le signale et on le place en dernier choix.
"""

import socket

DEBIT_BLUETOOTH_MBITS = 1.5     # estimation, à mesurer avec bench.py


def _interfaces_psutil() -> list[tuple[str, str]] | None:
    try:
        import psutil
    except ImportError:
        return None
    sortie = []
    for nom, adresses in psutil.net_if_addrs().items():
        for a in adresses:
            if a.family == socket.AF_INET:
                sortie.append((a.address, nom))
    return sortie


def _interfaces_socket() -> list[tuple[str, str]]:
    ips = set()
    try:
        ips.update(socket.gethostbyname_ex(socket.gethostname())[2])
    except OSError:
        pass
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("192.0.2.1", 9))   # aucun paquet envoyé : lit l'IP de la route par défaut
            ips.add(s.getsockname()[0])
    except OSError:
        pass
    return [(ip, "") for ip in ips]


def interfaces() -> list[tuple[str, str]]:
    """Liste (ip, nom de l'interface), sans les adresses auto-attribuées 169.254.x.x."""
    liste = _interfaces_psutil() or _interfaces_socket()
    return [(ip, nom) for ip, nom in liste if not ip.startswith("169.254.")]


def ips_locales(avec_boucle: bool = True) -> set[str]:
    ips = {ip for ip, _ in interfaces()}
    if avec_boucle:
        ips.add("127.0.0.1")
    else:
        ips = {ip for ip in ips if not ip.startswith("127.")}
    return ips


def est_bluetooth(nom_interface: str) -> bool:
    return "bluetooth" in (nom_interface or "").lower()


def type_reseau(ip: str, nom_interface: str = "") -> str:
    if est_bluetooth(nom_interface):
        return "Bluetooth, partage de connexion de l'iPhone (1 à 2 Mbit/s : petits fichiers)"
    if ip.startswith("172.20.10."):
        return "partage de connexion de l'iPhone (Wi-Fi ou USB)"
    if ip.startswith("192.168.137."):
        return "point d'accès mobile Windows (ce PC est l'hôte)"
    if ip.startswith("127."):
        return "boucle locale"
    return "réseau local (box ou autre)"


def _priorite(ip: str, nom_interface: str) -> int:
    """Ordre de préférence pour le QR code destiné à l'iPhone : le lien le plus rapide d'abord.

    Sur le partage de l'iPhone, l'iPhone joint forcément le PC par 172.20.10.x ; s'il héberge un
    point d'accès, l'iPhone y est probablement connecté ; le Bluetooth vient en dernier (lent).
    """
    if est_bluetooth(nom_interface):
        return 3
    if ip.startswith("172.20.10."):
        return 0
    if ip.startswith("192.168.137."):
        return 1
    return 2


def ips_classees() -> list[tuple[str, str]]:
    liste = [(ip, nom) for ip, nom in interfaces() if not ip.startswith("127.")]
    liste.sort(key=lambda x: (_priorite(*x), x[0]))
    return [(ip, type_reseau(ip, nom)) for ip, nom in liste]


def ip_locale_vers(ip_distante: str) -> str | None:
    """IP locale que le système utiliserait pour joindre ip_distante (aucun paquet n'est envoyé)."""
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect((ip_distante, 9))
            return s.getsockname()[0]
    except OSError:
        return None


def via_bluetooth(ip_distante: str) -> bool:
    locale = ip_locale_vers(ip_distante)
    return any(ip == locale and est_bluetooth(nom) for ip, nom in interfaces())


def duree_estimee_bluetooth(octets: int) -> float:
    return 8 * octets / (DEBIT_BLUETOOTH_MBITS * 1e6)
