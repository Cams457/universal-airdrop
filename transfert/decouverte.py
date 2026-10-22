"""Découverte automatique des PC par multicast UDP, sur CHAQUE interface réseau.

Envoyer seulement sur la route par défaut ne suffit pas : si le PC héberge un point d'accès
mobile Windows, ses voisins sont sur l'interface du point d'accès, pas sur celle d'Internet.

Sécurité : une annonce n'est qu'un indice d'adresse. Une annonce falsifiée peut au pire
rediriger une connexion vers une mauvaise IP, où elle échouera, car TLS exige la CA épinglée.
"""

import asyncio
import json
import socket
import struct
import time

from .config import GROUPE_MULTICAST, PORT_MULTICAST
from .reseau import ips_locales
from .securite import ip_autorisee

PERIODE_S = 5
OUBLI_S = 30


def annonce(nom: str, empreinte: str, port: int) -> bytes:
    return json.dumps({"app": "transfert", "v": 1, "nom": nom[:64], "fp": empreinte, "port": port}).encode()


def lire_annonce(donnees: bytes) -> dict | None:
    if len(donnees) > 1024:
        return None
    try:
        a = json.loads(donnees)
    except (ValueError, UnicodeDecodeError):
        return None
    if not isinstance(a, dict) or a.get("app") != "transfert":
        return None
    fp, port = a.get("fp"), a.get("port")
    if not (isinstance(fp, str) and len(fp) == 64 and isinstance(port, int) and 0 < port < 65536):
        return None
    return {"nom": str(a.get("nom", ""))[:64], "fp": fp, "port": port}


class _Protocole(asyncio.DatagramProtocol):
    def __init__(self, decouverte):
        self.d = decouverte

    def datagram_received(self, data, addr):
        ip = addr[0]
        if not ip_autorisee(ip):
            return
        a = lire_annonce(data)
        if a and a["fp"] != self.d.empreinte:
            self.d.enregistrer(a, ip)


class Decouverte:
    def __init__(self, nom: str, empreinte: str, port: int, sur_pair=None):
        self.nom, self.empreinte, self.port = nom, empreinte, port
        self.sur_pair = sur_pair            # rappel (empreinte, ip, port) pour mettre à jour les adresses
        self.vus: dict[str, dict] = {}
        self._transport = None
        self._taches = []
        self._interfaces_jointes: set[str] = set()
        self._sock = None

    def enregistrer(self, a: dict, ip: str) -> None:
        self.vus[a["fp"]] = {"nom": a["nom"], "ip": ip, "port": a["port"], "vu": time.time()}
        if self.sur_pair:
            self.sur_pair(a["fp"], ip, a["port"])

    def actifs(self) -> dict[str, dict]:
        limite = time.time() - OUBLI_S
        return {fp: v for fp, v in self.vus.items() if v["vu"] >= limite}

    def _joindre(self) -> None:
        for ip in ips_locales(avec_boucle=True) - self._interfaces_jointes:
            mreq = struct.pack("4s4s", socket.inet_aton(GROUPE_MULTICAST), socket.inet_aton(ip))
            try:
                self._sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
                self._interfaces_jointes.add(ip)
            except OSError:
                pass    # interface sans multicast (VPN, par exemple)

    def _emettre(self) -> None:
        message = annonce(self.nom, self.empreinte, self.port)
        for ip in ips_locales(avec_boucle=True):
            try:
                with socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP) as s:
                    s.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_TTL, 1)   # reste sur le lien local
                    s.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_IF, socket.inet_aton(ip))
                    s.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_LOOP, 1)
                    s.sendto(message, (GROUPE_MULTICAST, PORT_MULTICAST))
            except OSError:
                pass

    async def _boucle(self) -> None:
        while True:
            self._joindre()             # nouvelle interface (partage activé) : on la rejoint
            self._emettre()
            await asyncio.sleep(PERIODE_S)

    async def demarrer(self) -> None:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind(("", PORT_MULTICAST))
        s.setblocking(False)
        self._sock = s
        loop = asyncio.get_running_loop()
        self._transport, _ = await loop.create_datagram_endpoint(lambda: _Protocole(self), sock=s)
        self._taches.append(asyncio.create_task(self._boucle()))

    async def arreter(self) -> None:
        for t in self._taches:
            t.cancel()
        if self._transport:
            self._transport.close()
