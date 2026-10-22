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
    """A (émetteur) dans ce processus, B (récepteur) dans un AUTRE processus : chaque bout a son propre
    cœur, comme deux vrais PC. Sinon émetteur et récepteur se partageraient un seul GIL Python."""
    import subprocess
    import time
    from transfert.config import ecrire_json
    from transfert.serveur import Service
    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        port_b = _port()
        ecrire_json(tmp / "B" / "config.json", {
            "nom": "BENCH-B", "port": port_b, "reception": str(tmp / "B-recus"),
            "decouverte": False, "mdns": False, "ouvrir_liens": False})
        proc = subprocess.Popen([sys.executable, "-m", "transfert", "--donnees", str(tmp / "B"), "demon"],
                                cwd=Path(__file__).resolve().parent,
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        cfg_a = charger_config(tmp / "A")
        cfg_a.nom, cfg_a.port, cfg_a.reception, cfg_a.decouverte, cfg_a.mdns, cfg_a.ouvrir_liens = \
            "BENCH-A", _port(), str(tmp / "A-recus"), False, False, False
        A = Service(cfg_a, verbeux=False)
        cfg_b = charger_config(tmp / "B")
        try:
            for _ in range(100):
                if (tmp / "B" / "controle.json").exists():
                    break
                time.sleep(0.1)
            await A.demarrer()
            await asyncio.to_thread(appeler, cfg_b, "POST", "/controle/appairage/ouvrir")

            async def ok(code, *_):
                e = await asyncio.to_thread(appeler, cfg_b, "GET", "/controle/appairage")
                await asyncio.to_thread(appeler, cfg_b, "POST", "/controle/appairage/decision", {"accepter": True})
                return e["code"] == code

            await appairer(A.cfg, A.identite, A.appareils, "127.0.0.1", port_b, ok)
            src = _fichier(tmp, mo)
            photos = []
            for i in range(40):
                p = tmp / f"photo_{i:02d}.heic"
                p.write_bytes(os.urandom(3 * 1024 * 1024))
                photos.append(p)
            resultats = []
            for bloc, par in essais:
                A.cfg.paralleles = par
                (b,) = await envoyer(A.cfg, A.identite, A.appareils, "BENCH-B", [src], taille_bloc=bloc)
                resultats.append(((bloc, par), b))
            # 40 photos de 3 Mo : mesure le coût par fichier
            A.cfg.paralleles = essais[-1][1]
            t0 = time.perf_counter()
            bilans = await envoyer(A.cfg, A.identite, A.appareils, "BENCH-B", photos, taille_bloc=essais[-1][0])
            duree = time.perf_counter() - t0
            total = sum(b.taille for b in bilans)
            print(f"\n40 photos de 3 Mo : {total / 1e6:.0f} Mo en {duree:.2f} s, soit {8 * total / duree / 1e6:.0f} Mbit/s")
            return resultats
        finally:
            await A.arreter()
            proc.terminate()
            proc.wait(10)


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
    essais = [(1 << 20, 1), (4 << 20, 1), (4 << 20, 4), (8 << 20, 4)]
    if sys.platform == "win32":
        asyncio.set_event_loop_policy(asyncio.WindowsProactorEventLoopPolicy())
    res = asyncio.run(_local(a.mo, essais) if a.local else _distant(a.cible, a.mo, essais))
    _afficher(res, a.iperf)


if __name__ == "__main__":
    main()
