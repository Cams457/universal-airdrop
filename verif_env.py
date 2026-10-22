"""Phase 0 : vérifie l'environnement d'un PC avant de lancer Transfert.

Utilisation (invite cmd) :  python verif_env.py
Ne modifie rien sur le système : il lit seulement, puis affiche un rapport à me transmettre.
"""

import importlib.util
import socket
import subprocess
import sys

PORTS = (47800, 47801)
PAQUETS = ("aiohttp", "cryptography", "qrcode")


def ligne(ok, texte):
    print(f"  [{'OK' if ok else '!!'}] {texte}")


def commande(args):
    try:
        r = subprocess.run(args, capture_output=True, text=True, timeout=15,
                           encoding="cp850" if sys.platform == "win32" else None, errors="replace")
        return r.stdout
    except (OSError, subprocess.TimeoutExpired):
        return ""


def ips():
    trouvees = set()
    try:
        trouvees.update(socket.gethostbyname_ex(socket.gethostname())[2])
    except OSError:
        pass
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("192.0.2.1", 9))
            trouvees.add(s.getsockname()[0])
    except OSError:
        pass
    return sorted(ip for ip in trouvees if not ip.startswith(("127.", "169.254.")))


def type_reseau(ip):
    if ip.startswith("172.20.10."):
        return "partage de connexion de l'iPhone"
    if ip.startswith("192.168.137."):
        return "point d'accès mobile Windows (ce PC est l'hôte)"
    return "réseau local (box ou autre)"


def port_libre(port, proto):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM if proto == "tcp" else socket.SOCK_DGRAM)
    try:
        s.bind(("0.0.0.0", port))
        return True
    except OSError:
        return False
    finally:
        s.close()


def main():
    print("=== Transfert : vérification de l'environnement ===\n")

    print("Python")
    ok = sys.version_info >= (3, 12)
    ligne(ok, f"version {sys.version.split()[0]} (3.12 ou plus requis)")

    print("\nPaquets")
    for p in PAQUETS:
        present = importlib.util.find_spec(p) is not None
        ligne(present, p if present else f"{p} manquant : python -m pip install {p}")

    print("\nAdresses IP de ce PC")
    liste = ips()
    if not liste:
        ligne(False, "aucune IP réseau trouvée : le PC est-il connecté ?")
    for ip in liste:
        ligne(True, f"{ip}  ->  {type_reseau(ip)}")

    print("\nPorts")
    ligne(port_libre(PORTS[0], "tcp"), f"TCP {PORTS[0]} libre (serveur HTTPS)")
    ligne(port_libre(PORTS[1], "udp"), f"UDP {PORTS[1]} libre (découverte)")

    if sys.platform == "win32":
        print("\nWi-Fi (netsh wlan show interfaces)")
        sortie = commande(["netsh", "wlan", "show", "interfaces"])
        cles = ("SSID", "Type de radio", "Radio type", "Bande", "Band", "Canal", "Channel",
                "Réception", "Receive rate", "Transmission", "Transmit rate")
        for l in sortie.splitlines():
            if l.strip().startswith(cles) and "BSSID" not in l:
                print("   ", l.strip())
        print("  -> Bande 5 GHz conseillée : canal supérieur à 14.")

        print("\nProfils réseau (Privé / Public)")
        sortie = commande(["powershell", "-NoProfile", "-Command",
                           "Get-NetConnectionProfile | ForEach-Object { $_.InterfaceAlias + ' : ' + $_.NetworkCategory }"])
        for l in sortie.splitlines():
            if l.strip():
                print("   ", l.strip())

        print("\nRègle de pare-feu « Transfert »")
        sortie = commande(["netsh", "advfirewall", "firewall", "show", "rule", "name=Transfert"])
        ligne("Transfert" in sortie, "présente" if "Transfert" in sortie
              else "absente : voir pare_feu.cmd (à lancer seulement après relecture)")

    print("\nCopie ce rapport et transmets-le à Claude.")


if __name__ == "__main__":
    main()
