"""Ligne de commande : python -m transfert <commande>.

Commandes principales :
  demon                     lance le démon (à laisser tourner)
  nommer "PC de Camil"      nom affiché sur l'iPhone et sur les autres PC
  adresses                  IP de ce PC, réseau par réseau (box, partage iPhone, point d'accès)
  empreinte                 empreinte de la CA de ce PC (à comparer sur l'iPhone)
  appairage-ouvrir          sur le PC B : ouvre une fenêtre d'appairage de 2 minutes
  appairer HOTE[:PORT]      sur le PC A : s'appaire avec B
  appareils                 liste des appareils de confiance
  envoyer CIBLE FICHIERS... envoie vers un PC appairé
  lien CIBLE URL            ouvre un lien sur un PC appairé
  partager CHEMIN|URL ...   PC -> iPhone : affiche un QR code
  iphone-ajouter NOM        prépare un iPhone : profil, jeton, guide du Raccourci
  revoquer NOM              retire un appareil
  pare-feu                  affiche les commandes netsh à valider
  installer-windows         ajoute « Envoyer vers » au clic droit (--demarrage : démon au démarrage)
  desinstaller-windows      retire ces raccourcis
"""

import argparse
import asyncio
import os
import sys
import time
from pathlib import Path

from . import VERSION
from .appareils import Appareils
from .config import charger_config
from .controle import DemonAbsent, appeler, demon_actif
from .identite import Identite, empreinte_lisible
from .reseau import ips_classees


def _cfg_identite(args):
    cfg = charger_config(args.donnees)
    return cfg, Identite(cfg.donnees, cfg.nom, noms_dns=(f"{cfg.nom_mdns}.local",)), Appareils(cfg.donnees)


def _afficher_qr(url: str, cfg) -> None:
    try:
        import qrcode
    except ImportError:
        print(f"(qrcode non installé) Ouvre sur l'iPhone : {url}")
        return
    qr = qrcode.QRCode(border=1)
    qr.add_data(url)
    qr.make(fit=True)
    qr.print_ascii(invert=True)
    try:
        image = qr.make_image()
        chemin = Path(cfg.donnees) / "qr.png"
        image.save(chemin)
        if sys.platform == "win32":
            os.startfile(chemin)       # ouvre le QR en grand dans la visionneuse d'images
    except Exception:
        pass


def _barre(nom, progres):
    ecoule = time.perf_counter() - progres.debut
    debit = 8 * progres.fait / ecoule / 1e6 if ecoule > 0 else 0
    pct = 100 * progres.fait / progres.total if progres.total else 100
    reste = (progres.total - progres.fait) * 8 / (debit * 1e6) if debit > 0 else 0
    print(f"\r  {nom[:40]:<40} {pct:5.1f} %  {debit:6.0f} Mbit/s  reste {reste:4.0f} s", end="", flush=True)


# --- commandes ----------------------------------------------------------------

def cmd_demon(args):
    from .serveur import Service
    cfg = charger_config(args.donnees)
    service = Service(cfg)

    async def principal():
        await service.demarrer()
        print(f"\nEmpreinte de ce PC : {empreinte_lisible(service.identite.empreinte)}")
        print("Ctrl+C pour arrêter.\n")
        try:
            await asyncio.Event().wait()
        finally:
            await service.arreter()

    try:
        asyncio.run(principal())
    except KeyboardInterrupt:
        print("\nDémon arrêté.")


def cmd_nommer(args):
    cfg = charger_config(args.donnees)
    cfg.nom = args.nom.strip()[:60]
    cfg.sauver()
    print(f"Ce PC s'appelle maintenant « {cfg.nom} » ; sur le réseau : {cfg.nom_mdns}.local")
    print("Relance le démon pour que le nom soit annoncé.")


def cmd_adresses(args):
    cfg = charger_config(args.donnees)
    liste = ips_classees()
    if not liste:
        print("Aucune adresse réseau : ce PC n'est connecté à aucun réseau.")
    for ip, t in liste:
        print(f"  {ip:<16} {t}")
    print(f"\nNom Bonjour : {cfg.nom_mdns}.local (à utiliser en priorité dans le Raccourci iOS)")
    print(f"Adresse du Raccourci : https://{cfg.nom_mdns}.local:{cfg.port}")


def cmd_empreinte(args):
    _, ident, _ = _cfg_identite(args)
    print(f"Empreinte SHA-256 de la CA de ce PC :\n  {empreinte_lisible(ident.empreinte)}")


def cmd_appairage_ouvrir(args):
    cfg = charger_config(args.donnees)
    d = appeler(cfg, "POST", "/controle/appairage/ouvrir")
    print(f"Fenêtre d'appairage ouverte pour {d['duree']} s.")
    print("Sur l'autre PC, lance :  python -m transfert appairer <IP de ce PC>")
    for ip, t in ips_classees():
        print(f"   {ip}   ({t})")
    while True:
        time.sleep(0.5)
        e = appeler(cfg, "GET", "/controle/appairage")
        if e["etat"] == "a_confirmer":
            print(f"\nDemande de « {e['nom']} » depuis {e['ip']}")
            print(f"Empreinte : {e['empreinte']}")
            print(f"\n   CODE :  {e['code'][:3]} {e['code'][3:]}\n")
            rep = input("Le même code s'affiche-t-il sur l'autre PC ? (o/n) ").strip().lower()
            appeler(cfg, "POST", "/controle/appairage/decision", {"accepter": rep == "o"})
            if rep != "o":
                print("Appairage refusé.")
                return
            for _ in range(60):
                time.sleep(0.5)
                if appeler(cfg, "GET", "/controle/appairage")["etat"] == "termine":
                    print("Appairage terminé.")
                    return
            print("L'autre PC n'a pas confirmé à temps.")
            return
        if e["etat"] in ("expire", "ferme", "refuse"):
            print("Fenêtre fermée sans appairage.")
            return


def cmd_appairer(args):
    from .client import appairer
    cfg, ident, appareils = _cfg_identite(args)
    hote, _, port = args.hote.partition(":")
    port = int(port) if port else cfg.port

    def confirmer(code, nom_b, empreinte_b):
        print(f"Pair : « {nom_b} »\nEmpreinte : {empreinte_b}")
        print(f"\n   CODE :  {code[:3]} {code[3:]}\n")
        return input("Le même code s'affiche-t-il sur l'autre PC ? (o/n) ").strip().lower() == "o"

    r = asyncio.run(appairer(cfg, ident, appareils, hote, port, confirmer))
    print(f"Appairé avec « {r['nom']} ». Envoi : python -m transfert envoyer \"{r['nom']}\" fichier")


def cmd_appareils(args):
    _, _, appareils = _cfg_identite(args)
    liste = appareils.liste()
    if not liste:
        print("Aucun appareil de confiance pour l'instant.")
    for a in liste:
        print(f"  [{a['type']:<6}] {a['nom']:<24} {a['adresse']}")


def cmd_envoyer(args):
    from .client import envoyer
    cfg, ident, appareils = _cfg_identite(args)
    chemins = [Path(c) for c in args.fichiers]
    absents = [str(c) for c in chemins if not c.is_file()]
    if absents:
        sys.exit("Introuvable : " + ", ".join(absents))
    if args.paralleles:
        cfg.paralleles = args.paralleles
    bilans = asyncio.run(envoyer(cfg, ident, appareils, args.cible, chemins, rappel=_barre))
    print()
    for b in bilans:
        reprise = f", reprise : {b.blocs_envoyes}/{b.blocs_total} blocs" if b.blocs_envoyes < b.blocs_total else ""
        print(f"  {b.nom} -> {b.nom_recu} : {b.taille / 1e6:.1f} Mo en {b.duree:.1f} s "
              f"({b.debit_mbits:.0f} Mbit/s, démarrage {b.latence_demarrage * 1000:.0f} ms{reprise})")


def cmd_lien(args):
    from .client import envoyer_lien
    cfg, _, appareils = _cfg_identite(args)
    r = asyncio.run(envoyer_lien(cfg, appareils, args.cible, args.url))
    print("Lien ouvert sur l'autre PC." if r.get("ouvert") else "Lien transmis (non ouvert automatiquement).")


def cmd_partager(args):
    cfg = charger_config(args.donnees)
    chemins, url = [], None
    for x in args.elements:
        if x.startswith(("http://", "https://")):
            url = x
        else:
            chemins.append(str(Path(x).resolve()))
    d = appeler(cfg, "POST", "/controle/partage", {"chemins": chemins, "url": url})
    urls = d["urls"]
    if not urls:
        sys.exit("Aucune adresse réseau : connecte ce PC à un réseau ou au partage de l'iPhone.")
    principale = next((u for u in urls if args.ip and args.ip in u["url"]), urls[0])
    print(f"Scanne ce QR code avec l'appareil photo de l'iPhone ({principale['type']}) :\n")
    _afficher_qr(principale["url"], cfg)
    print(f"\n  {principale['url']}")
    autres = [u for u in urls if u is not principale]
    if autres:
        print("\nSi l'iPhone est sur un autre réseau :")
        for u in autres:
            print(f"  {u['url']}   ({u['type']})")
    print("\nLien valable 10 minutes, invalidé après téléchargement.")


def cmd_iphone_ajouter(args):
    from .profil_ios import generer
    cfg, ident, appareils = _cfg_identite(args)
    if not demon_actif(cfg):
        sys.exit("Lance d'abord le démon dans une autre fenêtre : python -m transfert demon")
    dossier = Path(cfg.donnees) / "ios"
    dossier.mkdir(exist_ok=True)
    profil = dossier / f"Transfert-{cfg.nom}.mobileconfig"
    profil.write_bytes(generer(ident.der_ca(), cfg.nom, ident.empreinte))
    jeton = appareils.ajouter_iphone(args.nom)

    print("=== Étape 1 : installer le profil (une seule fois par PC) ===\n")
    d = appeler(cfg, "POST", "/controle/partage", {"chemins": [str(profil)]})
    _afficher_qr(d["urls"][0]["url"], cfg)
    print(f"\n  {d['urls'][0]['url']}")
    print("\nSafari signalera une connexion non privée : c'est normal AVANT l'installation du profil.")
    print("Touche « Afficher les détails » > « visiter ce site web », puis autorise le téléchargement.")
    print("Puis : Réglages > Profil téléchargé > Installer.")
    print("Puis : Réglages > Général > Informations > Réglages des certificats > active la confiance totale.")
    print(f"\nVÉRIFIE que l'empreinte SHA-256 affichée par iOS est exactement :\n  {empreinte_lisible(ident.empreinte)}")
    input("\nAppuie sur Entrée quand le profil est installé et approuvé...")

    print("\n=== Étape 2 : récupérer le jeton du Raccourci ===\n")
    adresses = [ip for ip, _ in ips_classees()]
    texte = (f"Jeton :\n{jeton}\n\nAdresse à mettre dans le Raccourci :\n"
             f"https://{cfg.nom_mdns}.local:{cfg.port}\n\nAdresses IP de secours :\n"
             + "\n".join(f"https://{ip}:{cfg.port}" for ip in adresses))
    d = appeler(cfg, "POST", "/controle/partage", {"texte": texte})
    _afficher_qr(d["urls"][0]["url"], cfg)
    print(f"\n  {d['urls'][0]['url']}")
    print("\nCette page n'est lisible qu'UNE fois. Copie le jeton dans le Raccourci (voir docs/RACCOURCI_IOS.md).")


def cmd_revoquer(args):
    _, _, appareils = _cfg_identite(args)
    n = appareils.revoquer(args.nom)
    print(f"{n} entrée(s) retirée(s)." if n else f"Aucun appareil nommé « {args.nom} ».")


def cmd_envoyer_choisir(args):
    """Appelée par l'entrée « Envoyer vers » du clic droit."""
    from .windows import choisir_appareil
    _, _, appareils = _cfg_identite(args)
    noms = [a["nom"] for a in appareils.liste() if a["type"] == "pc"]
    if not noms:
        sys.exit("Aucun PC appairé : lance d'abord « python -m transfert appairer ».")
    nom = choisir_appareil(noms, args.fichiers)
    if not nom:
        return
    args.cible, args.paralleles = nom, None
    cmd_envoyer(args)
    time.sleep(3)


def cmd_installer_windows(args):
    from .windows import installer
    racine = Path(__file__).resolve().parent.parent
    for p in installer(racine, demarrage=args.demarrage):
        print(f"Créé : {p}")
    print("Clic droit sur un fichier > Envoyer vers > « Transfert vers un appareil ».")
    print("Pour tout retirer : python -m transfert desinstaller-windows")


def cmd_desinstaller_windows(args):
    from .windows import desinstaller
    retires = desinstaller()
    print("\n".join(f"Supprimé : {p}" for p in retires) or "Rien à supprimer.")


def cmd_pare_feu(args):
    cfg = charger_config(args.donnees)
    print("Commandes à relire, puis à lancer dans une invite cmd OUVERTE EN ADMINISTRATEUR :\n")
    print(f'netsh advfirewall firewall add rule name="Transfert" dir=in action=allow protocol=TCP '
          f'localport={cfg.port} remoteip=localsubnet profile=any')
    print('netsh advfirewall firewall add rule name="Transfert decouverte" dir=in action=allow protocol=UDP '
          'localport=47801 remoteip=localsubnet profile=any')
    print("\nPourquoi profile=any avec remoteip=localsubnet : l'interface du point d'accès mobile est souvent")
    print("classée « Public » ; on autorise donc tous les profils, mais seulement depuis les sous-réseaux")
    print("directement connectés au PC. Rien n'est ouvert à Internet.")
    print("\nPour annuler :")
    print('netsh advfirewall firewall delete rule name="Transfert"')
    print('netsh advfirewall firewall delete rule name="Transfert decouverte"')


def main(argv=None):
    p = argparse.ArgumentParser(prog="transfert", description=f"Transfert {VERSION}")
    p.add_argument("--donnees", help="dossier de données (par défaut %%APPDATA%%\\Transfert)")
    sp = p.add_subparsers(dest="commande", required=True)
    sp.add_parser("demon").set_defaults(f=cmd_demon)
    sp.add_parser("adresses").set_defaults(f=cmd_adresses)
    a = sp.add_parser("nommer")
    a.add_argument("nom")
    a.set_defaults(f=cmd_nommer)
    sp.add_parser("empreinte").set_defaults(f=cmd_empreinte)
    sp.add_parser("appairage-ouvrir").set_defaults(f=cmd_appairage_ouvrir)
    a = sp.add_parser("appairer")
    a.add_argument("hote")
    a.set_defaults(f=cmd_appairer)
    sp.add_parser("appareils").set_defaults(f=cmd_appareils)
    a = sp.add_parser("envoyer")
    a.add_argument("cible")
    a.add_argument("fichiers", nargs="+")
    a.add_argument("--paralleles", type=int)
    a.set_defaults(f=cmd_envoyer)
    a = sp.add_parser("lien")
    a.add_argument("cible")
    a.add_argument("url")
    a.set_defaults(f=cmd_lien)
    a = sp.add_parser("partager")
    a.add_argument("elements", nargs="+")
    a.add_argument("--ip", help="force l'adresse à mettre dans le QR code")
    a.set_defaults(f=cmd_partager)
    a = sp.add_parser("iphone-ajouter")
    a.add_argument("nom")
    a.set_defaults(f=cmd_iphone_ajouter)
    a = sp.add_parser("revoquer")
    a.add_argument("nom")
    a.set_defaults(f=cmd_revoquer)
    sp.add_parser("pare-feu").set_defaults(f=cmd_pare_feu)
    a = sp.add_parser("envoyer-choisir")
    a.add_argument("fichiers", nargs="+")
    a.set_defaults(f=cmd_envoyer_choisir)
    a = sp.add_parser("installer-windows")
    a.add_argument("--demarrage", action="store_true", help="lancer aussi le démon à l'ouverture de session")
    a.set_defaults(f=cmd_installer_windows)
    sp.add_parser("desinstaller-windows").set_defaults(f=cmd_desinstaller_windows)
    args = p.parse_args(argv)
    try:
        args.f(args)
    except (DemonAbsent, RuntimeError) as e:
        sys.exit(str(e))
    except Exception as e:
        from .client import ErreurTransfert
        if isinstance(e, ErreurTransfert):
            sys.exit(f"Échec : {e}")
        raise


if __name__ == "__main__":
    main()
