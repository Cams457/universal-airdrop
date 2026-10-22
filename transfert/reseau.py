"""Adresses IP du PC, interface par interface.

Indispensable pour le partage de connexion : un PC peut avoir à la fois une IP de box
(192.168.1.x), une IP fournie par le partage de l'iPhone (172.20.10.x) et, s'il héberge
lui-même un point d'accès mobile Windows, l'IP 192.168.137.1.
"""

import socket


def _ips_psutil() -> set[str] | None:
    try:
        import psutil
    except ImportError:
        return None
    ips = set()
    for adresses in psutil.net_if_addrs().values():
        for a in adresses:
            if a.family == socket.AF_INET:
                ips.add(a.address)
    return ips


def _ips_socket() -> set[str]:
    ips = set()
    try:
        ips.update(socket.gethostbyname_ex(socket.gethostname())[2])
    except OSError:
        pass
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("192.0.2.1", 9))   # aucun paquet envoyé : sert à lire l'IP de la route par défaut
            ips.add(s.getsockname()[0])
    except OSError:
        pass
    return ips


def ips_locales(avec_boucle: bool = True) -> set[str]:
    ips = _ips_psutil() or _ips_socket()
    ips = {ip for ip in ips if not ip.startswith("169.254.")}
    if avec_boucle:
        ips.add("127.0.0.1")
    else:
        ips = {ip for ip in ips if not ip.startswith("127.")}
    return ips


def type_reseau(ip: str) -> str:
    if ip.startswith("172.20.10."):
        return "partage de connexion de l'iPhone"
    if ip.startswith("192.168.137."):
        return "point d'accès mobile Windows (ce PC est l'hôte)"
    if ip.startswith("127."):
        return "boucle locale"
    return "réseau local (box ou autre)"


def _priorite(ip: str) -> int:
    """Ordre de préférence pour le QR code destiné à l'iPhone.

    Si le PC est sur le partage de l'iPhone, l'iPhone le joint forcément par 172.20.10.x.
    S'il héberge un point d'accès, l'iPhone y est probablement connecté.
    """
    if ip.startswith("172.20.10."):
        return 0
    if ip.startswith("192.168.137."):
        return 1
    return 2


def ips_classees() -> list[tuple[str, str]]:
    return [(ip, type_reseau(ip)) for ip in sorted(ips_locales(avec_boucle=False), key=lambda i: (_priorite(i), i))]
