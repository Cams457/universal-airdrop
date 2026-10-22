"""Tests Bonjour / mDNS : nom fixe « pc-de-camil.local », inscrit dans le certificat et résolu sur le réseau."""

import asyncio
import socket

import pytest
from cryptography import x509

from transfert.config import charger_config
from transfert.mdns import TYPE_SERVICE, nom_mdns
from transfert.serveur import Service


def test_nom_mdns():
    assert nom_mdns("PC de Camil") == "pc-de-camil"
    assert nom_mdns("Fixe Éléonore #2") == "fixe-eleonore-2"
    assert nom_mdns("???") == "pc-transfert"


def test_annonce_et_resolution(tmp_path):
    zeroconf = pytest.importorskip("zeroconf")
    from zeroconf.asyncio import AsyncServiceInfo, AsyncZeroconf

    cfg = charger_config(tmp_path / "pc")
    cfg.nom, cfg.decouverte, cfg.reception = "PC de Camil", False, str(tmp_path / "recus")
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        cfg.port = s.getsockname()[1]
    service = Service(cfg, verbeux=False)
    san = x509.load_pem_x509_certificate(service.identite.chemin_serveur.read_bytes()).extensions \
        .get_extension_for_class(x509.SubjectAlternativeName).value
    assert "pc-de-camil.local" in san.get_values_for_type(x509.DNSName)

    async def scenario():
        await service.demarrer()
        if not service.mdns:
            pytest.skip("mDNS indisponible dans cet environnement")
        az = AsyncZeroconf(ip_version=zeroconf.IPVersion.V4Only)
        try:
            info = AsyncServiceInfo(TYPE_SERVICE, f"PC de Camil.{TYPE_SERVICE}")
            trouve = await info.async_request(az.zeroconf, 3000)
            return trouve, info
        finally:
            await az.async_close()
            await service.arreter()

    trouve, info = asyncio.run(scenario())
    if not trouve:
        pytest.skip("aucune réponse mDNS reçue (conteneur sans multicast)")
    assert info.server == "pc-de-camil.local."
    assert info.port == cfg.port
