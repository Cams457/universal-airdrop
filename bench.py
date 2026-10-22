"""Mesure de vitesse : latence de démarrage et efficacité η par rapport au débit brut.

Deux modes (invite cmd) :
  python bench.py --local
      deux démons dans ce PC, en boucle locale : mesure le plafond du LOGICIEL seul (TLS, SHA-256,
      disque). Il doit être très au-dessus du débit Wi-Fi, sinon c'est le logiciel qui limite.
  python bench.py --cible PC-B --iperf 310
      vers un PC appairé, sur le vrai réseau. --iperf = débit TCP brut mesuré par iperf3 sur le même
      lien, en Mbit/s, pour calculer η = débit utile / débit iperf3 (cible : η >= 0,8).
"""

import argparse
import asyncio
import os
import socket
import sys
import tempfile
from pathlib import Path

from transfert.appareils import Appareils
from transfert.client import appairer, envoyer
from transfert.config import charger_config
from transfert.controle import appeler
from transfert.identite import Identite


def _port():
    while True:
        with socket.socket() as s:
            s.bind(("127.0.0.1", 0))
            p = s.getsockname()[1]
        try:
            with socket.socket() as a, socket.socket() as b:
                a.bind(("0.0.0.0", p))
                b.bind(("127.0.0.1", p + 1))
            return p
        except OSError:
            pass


def _fichier(dossier: Path, mo: int) -> Path:
    chemin = dossier / f"bench_{mo}Mo.bin"
    if not chemin.exists() or chemin.stat().st_size != mo * 1024 * 1024:
        with open(chemin, "wb") as f:
            for _ in range(mo):
                f.write(os.urandom(1024 * 1024))
    return chemin


def _afficher(resultats, iperf):
    print(f"\n{'bloc':>6} {'parallèle':>9} {'démarrage':>10} {'débit':>12} {'η':>6}")
    for (bloc, par), b in resultats:
        eta = f"{b.debit_mbits / iperf:.2f}" if iperf else "-"
        print(f"{bloc // 1024 // 1024:>4}Mo {par:>9} {b.latence_demarrage * 1000:>8.0f}ms "
              f"{b.debit_mbits:>8.0f}Mbit/s {eta:>6}")


async def _local(mo: int, essais):
    from transfert.serveur import Service
    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        services = []
        for nom in ("BENCH-A", "BENCH-B"):
            cfg = charger_config(tmp / nom)
            cfg.nom, cfg.port, cfg.reception, cfg.decouverte, cfg.ouvrir_liens = \
                nom, _port(), str(tmp / f"{nom}-recus"), False, False
            cfg.mdns = False
            services.append(Service(cfg, verbeux=False))
        A, B = services
        await A.demarrer()
        await B.demarrer()
        try:
            await asyncio.to_thread(appeler, B.cfg, "POST", "/controle/appairage/ouvrir")

            async def ok(code, *_):
                e = await asyncio.to_thread(appeler, B.cfg, "GET", "/controle/appairage")
                await asyncio.to_thread(appeler, B.cfg, "POST", "/controle/appairage/decision", {"accepter": True})
                return e["code"] == code

            await appairer(A.cfg, A.identite, A.appareils, "127.0.0.1", B.cfg.port, ok)
            src = _fichier(tmp, mo)
            resultats = []
            for bloc, par in essais:
                A.cfg.paralleles = par
                for f in B.cfg.dossier_reception.glob("bench_*"):
                    f.unlink()
                (b,) = await envoyer(A.cfg, A.identite, A.appareils, "BENCH-B", [src], taille_bloc=bloc)
                resultats.append(((bloc, par), b))
            return resultats
        finally:
            await A.arreter()
            await B.arreter()


async def _distant(cible: str, mo: int, essais):
    cfg = charger_config()
    ident, appareils = Identite(cfg.donnees, cfg.nom), Appareils(cfg.donnees)
    dossier = Path(tempfile.gettempdir()) / "transfert-bench"
    dossier.mkdir(exist_ok=True)
    src = _fichier(dossier, mo)
    resultats = []
    for bloc, par in essais:
        cfg.paralleles = par
        (b,) = await envoyer(cfg, ident, appareils, cible, [src], taille_bloc=bloc)
        resultats.append(((bloc, par), b))
        print(f"  bloc {bloc >> 20} Mo, {par} en parallèle : {b.debit_mbits:.0f} Mbit/s")
    print("\nPense à supprimer les fichiers bench_*.bin reçus sur l'autre PC.")
    return resultats


def main():
    p = argparse.ArgumentParser(description="Mesure de vitesse de Transfert")
    g = p.add_mutually_exclusive_group(required=True)
    g.add_argument("--local", action="store_true")
    g.add_argument("--cible")
    p.add_argument("--mo", type=int, default=200, help="taille du fichier de test en Mo")
    p.add_argument("--iperf", type=float, help="débit TCP brut iperf3 sur le même lien, en Mbit/s")
    a = p.parse_args()
    essais = [(1 << 20, 1), (4 << 20, 1), (4 << 20, 3), (8 << 20, 3)]
    if sys.platform == "win32":
        asyncio.set_event_loop_policy(asyncio.WindowsProactorEventLoopPolicy())
    res = asyncio.run(_local(a.mo, essais) if a.local else _distant(a.cible, a.mo, essais))
    _afficher(res, a.iperf)


if __name__ == "__main__":
    main()
