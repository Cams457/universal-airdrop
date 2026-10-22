"""Annonce Bonjour / mDNS : le PC répond au nom fixe « pc-de-camil.local ».

Pourquoi : le Raccourci iOS n'a alors qu'UNE adresse à connaître, quel que soit le réseau.
Via la box, le nom se résout en 192.168.x.y ; via le partage de connexion de l'iPhone, en
172.20.10.x. iOS résout nativement les noms .local (Bonjour), sans rien installer.

Le nom est aussi inscrit dans le certificat du PC (SAN), pour que iOS accepte le HTTPS.
"""

import re
import socket
import unicodedata

from .reseau import ips_locales

TYPE_SERVICE = "_transfert._tcp.local."


def nom_mdns(nom: str) -> str:
    """« PC de Camil » -> « pc-de-camil » : minuscules, sans accents, tirets (règles DNS)."""
    ascii_ = unicodedata.normalize("NFKD", nom).encode("ascii", "ignore").decode()
    slug = re.sub(r"[^a-z0-9]+", "-", ascii_.lower()).strip("-")
    return (slug or "pc-transfert")[:63]


class AnnonceMdns:
    def __init__(self, nom: str, port: int, empreinte: str):
        self.nom, self.port, self.empreinte = nom, port, empreinte
        self.hote = nom_mdns(nom)
        self._az = None
        self._info = None
        self._ips: set[str] = set()

    def _construire(self):
        from zeroconf.asyncio import AsyncServiceInfo
        self._ips = ips_locales(avec_boucle=False)
        return AsyncServiceInfo(
            TYPE_SERVICE, f"{self.nom[:60]}.{TYPE_SERVICE}",
            addresses=[socket.inet_aton(ip) for ip in sorted(self._ips)],
            port=self.port, server=f"{self.hote}.local.",
            properties={"fp": self.empreinte[:16], "v": "1"})

    async def demarrer(self) -> None:
        from zeroconf import IPVersion
        from zeroconf.asyncio import AsyncZeroconf
        self._az = AsyncZeroconf(ip_version=IPVersion.V4Only)
        self._info = self._construire()
        await self._az.async_register_service(self._info, allow_name_change=True)

    async def actualiser(self) -> bool:
        """Nouvelle IP (partage de connexion activé) : on met à jour l'annonce. Renvoie True si changé."""
        if not self._az or ips_locales(avec_boucle=False) == self._ips:
            return False
        nouvelle = self._construire()
        nouvelle.name = self._info.name
        await self._az.async_update_service(nouvelle)
        self._info = nouvelle
        return True

    async def arreter(self) -> None:
        if self._az:
            try:
                await self._az.async_unregister_all_services()
            finally:
                await self._az.async_close()
