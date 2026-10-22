"""Démon Transfert : serveur HTTPS sur le réseau local et API de contrôle sur 127.0.0.1.

Surfaces exposées au réseau (toutes en TLS, toutes refusées hors adresses privées) :
- /api/v1/info                       infos publiques (nom, empreinte de la CA)
- /api/v1/appairage/*                appairage PC <-> PC par code à 6 chiffres avec engagement
- /api/v1/transfert/*                envoi PC -> PC par blocs, avec reprise (jeton requis)
- /api/rapide/fichier, /api/rapide/lien   envoi depuis l'iPhone (Raccourci), jeton requis
- /p/<jeton>                         page de téléchargement PC -> iPhone (QR code)
L'API de contrôle n'écoute que sur 127.0.0.1 et exige un secret local : seule la ligne de
commande de l'utilisateur, sur ce PC, peut ouvrir un appairage ou créer un partage.
"""

import asyncio
import datetime as dt
import hashlib
import html
import os
import secrets
import shutil
import socket
import sys
import time
import webbrowser
from pathlib import Path
from urllib.parse import quote, unquote

from aiohttp import BodyPartReader, web

from . import VERSION, convertir
from . import securite as S
from .appareils import Appareils
from .blocs import Reception
from .config import DUREE_APPAIRAGE_S, Config, ecrire_json
from .decouverte import Decouverte
from .identite import Identite, empreinte_lisible, empreinte_pem
from .partage import Partages
from .reseau import ips_classees

TAILLE_BLOC_MIN = 64 * 1024
TAILLE_BLOC_MAX = 16 * 1024 * 1024
TAILLE_MAX = 1 << 42                 # 4 Tio
ECHECS_MAX, FENETRE_ECHECS_S = 20, 600
TENTATIVES_APPAIRAGE = 3
MORCEAU = 4 * 1024 * 1024


class _FichierPuisRappel(web.FileResponse):
    """FileResponse qui prévient une fois le fichier entièrement envoyé (sendfile quand c'est possible)."""

    def __init__(self, *args, apres=None, **kwargs):
        super().__init__(*args, **kwargs)
        self._apres = apres

    async def prepare(self, request):
        ecrivain = await super().prepare(request)
        if self._apres:
            self._apres()
        return ecrivain


def erreur(status: int, message: str) -> web.Response:
    return web.json_response({"erreur": message}, status=status)


class Appairage:
    """Fenêtre d'appairage ouverte côté récepteur (B) par l'utilisateur, pour une durée limitée."""

    def __init__(self, duree: float):
        self.expire = time.monotonic() + duree
        self.etat = "attente"          # attente -> engage -> a_confirmer -> termine | refuse | expire
        self.tentatives = 0
        self.decision = asyncio.Event()
        self.accepte = False
        self.session = None
        self.code = None
        self.pair: dict = {}

    @property
    def ouverte(self) -> bool:
        return time.monotonic() < self.expire and self.etat not in ("termine", "refuse", "expire")


class Service:
    def __init__(self, cfg: Config, verbeux: bool = True):
        self.cfg = cfg
        self.verbeux = verbeux
        self.identite = Identite(cfg.donnees, cfg.nom, noms_dns=(f"{cfg.nom_mdns}.local",))
        self.appareils = Appareils(cfg.donnees)
        self.partages = Partages()
        self.decouverte = (Decouverte(cfg.nom, self.identite.empreinte, cfg.port,
                                      self.appareils.mettre_a_jour_adresse) if cfg.decouverte else None)
        self.mdns = None
        self.receptions: dict[str, Reception] = {}
        self.appairage: Appairage | None = None
        self.echecs: dict[str, list[float]] = {}
        self.secret_controle = S.nouveau_jeton()
        self.ssl = None
        self._runners = []
        self._taches = []

    # --- utilitaires ------------------------------------------------------------

    @property
    def tmp(self) -> Path:
        """Dossier temporaire sur le même volume que la réception : le renommage final est atomique."""
        return self.cfg.dossier_reception / ".en_cours"

    def journal(self, texte: str) -> None:
        ligne = f"{dt.datetime.now().isoformat(timespec='seconds')}  {texte}"
        try:
            with open(self.cfg.donnees / "journal.log", "a", encoding="utf-8") as f:
                f.write(ligne + "\n")
        except OSError:
            pass
        if self.verbeux:
            print(ligne, flush=True)

    def _authentifier(self, request: web.Request) -> dict:
        ip = request.remote or ""
        maintenant = time.time()
        recents = [t for t in self.echecs.get(ip, []) if t > maintenant - FENETRE_ECHECS_S]
        if len(recents) >= ECHECS_MAX:
            raise web.HTTPTooManyRequests(text='{"erreur": "trop d\'échecs d\'authentification"}',
                                          content_type="application/json")
        jeton = None
        entete = request.headers.get("Authorization", "")
        if entete.startswith("Bearer "):
            jeton = entete[7:].strip()
        jeton = jeton or request.headers.get("X-Jeton", "").strip() or None
        app = self.appareils.verifier_jeton(jeton)
        if not app:
            recents.append(maintenant)
            self.echecs[ip] = recents
            raise web.HTTPUnauthorized(text='{"erreur": "jeton absent ou invalide"}',
                                       content_type="application/json")
        app["cle"] = f'{app["type"]}:{app["nom"]}:{app.get("empreinte")}'
        return app

    def _post_traiter(self, chemin: Path) -> None:
        if self.cfg.convertir_heic or self.cfg.convertir_hevc:
            asyncio.get_running_loop().run_in_executor(
                None, convertir.traiter, chemin, self.cfg.convertir_heic, self.cfg.convertir_hevc)

    # --- middlewares ----------------------------------------------------------

    @web.middleware
    async def _filtre_reseau(self, request, handler):
        if not S.ip_autorisee(request.remote):
            return erreur(403, "seules les adresses du réseau local sont acceptées")
        resp = await handler(request)
        resp.headers.setdefault("X-Content-Type-Options", "nosniff")
        resp.headers.setdefault("Referrer-Policy", "no-referrer")
        resp.headers.setdefault("Cache-Control", "no-store")
        return resp

    @web.middleware
    async def _filtre_controle(self, request, handler):
        if request.remote not in ("127.0.0.1", "::1"):
            return erreur(403, "contrôle local uniquement")
        if not S.egal_temps_constant(request.headers.get("X-Controle", ""), self.secret_controle):
            return erreur(401, "secret de contrôle invalide")
        return await handler(request)

    # --- applications ---------------------------------------------------------

    def app_publique(self) -> web.Application:
        app = web.Application(middlewares=[self._filtre_reseau], client_max_size=TAILLE_BLOC_MAX + 4096)
        app.add_routes([
            web.get("/api/v1/info", self.info),
            web.post("/api/v1/appairage/debut", self.appairage_debut),
            web.post("/api/v1/appairage/revele", self.appairage_revele),
            web.post("/api/v1/appairage/confirmer", self.appairage_confirmer),
            web.post("/api/v1/transfert/preparer", self.transfert_preparer),
            web.put(r"/api/v1/transfert/{ident:[0-9a-f]{32}}/bloc/{i:\d+}", self.transfert_bloc),
            web.post(r"/api/v1/transfert/{ident:[0-9a-f]{32}}/terminer", self.transfert_terminer),
            web.post("/api/rapide/fichier", self.rapide_fichier),
            web.post("/api/rapide/lien", self.rapide_lien),
            web.get(r"/p/{jeton:[A-Za-z0-9_-]{16,64}}", self.page_partage),
            web.get(r"/p/{jeton:[A-Za-z0-9_-]{16,64}}/{i:\d+}", self.telecharger_partage),
        ])
        return app

    def app_controle(self) -> web.Application:
        app = web.Application(middlewares=[self._filtre_controle])
        app.add_routes([
            web.get("/controle/etat", self.controle_etat),
            web.get("/controle/vus", self.controle_vus),
            web.post("/controle/appairage/ouvrir", self.controle_appairage_ouvrir),
            web.get("/controle/appairage", self.controle_appairage),
            web.post("/controle/appairage/decision", self.controle_appairage_decision),
            web.post("/controle/partage", self.controle_partage),
        ])
        return app

    # --- infos ----------------------------------------------------------------

    async def info(self, request):
        return web.json_response({"app": "transfert", "version": VERSION, "nom": self.cfg.nom,
                                  "empreinte": self.identite.empreinte})

    # --- appairage (côté B, celui qui a ouvert la fenêtre) ---------------------

    async def appairage_debut(self, request):
        a = self.appairage
        if not a or not a.ouverte or a.etat != "attente":
            return erreur(403, "aucune fenêtre d'appairage ouverte sur ce PC")
        if a.tentatives >= TENTATIVES_APPAIRAGE:
            return erreur(429, "trop de tentatives d'appairage")
        a.tentatives += 1
        try:
            c = await request.json()
            ca_pem, nom, port = str(c["ca_pem"]), str(c["nom"])[:64], int(c["port"])
            fp_a = empreinte_pem(ca_pem)
        except (ValueError, KeyError, TypeError):
            return erreur(400, "demande d'appairage invalide")
        if fp_a == self.identite.empreinte:
            return erreur(400, "un PC ne s'appaire pas avec lui-même")
        alea_b = S.nouvel_alea()
        a.session = secrets.token_urlsafe(16)
        a.pair = {"fp": fp_a, "ca_pem": ca_pem, "nom": nom, "port": port, "ip": request.remote, "alea_b": alea_b}
        a.etat = "engage"
        self.journal(f"appairage : demande de « {nom} » depuis {request.remote}")
        return web.json_response({
            "session": a.session, "nom": self.cfg.nom, "ca_pem": self.identite.ca_pem, "port": self.cfg.port,
            "engagement": S.engagement(self.identite.empreinte, fp_a, alea_b)})

    def _session(self, a: Appairage | None, c: dict, etat: str) -> bool:
        return bool(a and a.ouverte and a.etat == etat and a.session
                    and S.egal_temps_constant(str(c.get("session", "")), a.session))

    async def appairage_revele(self, request):
        a = self.appairage
        c = await request.json()
        if not self._session(a, c, "engage"):
            return erreur(403, "session d'appairage invalide")
        try:
            alea_a = bytes.fromhex(c["alea_a"])
        except (ValueError, KeyError, TypeError):
            return erreur(400, "aléa invalide")
        if len(alea_a) != 32:
            return erreur(400, "aléa invalide")
        a.code = S.code_verification(a.pair["fp"], self.identite.empreinte, alea_a, a.pair["alea_b"])
        a.etat = "a_confirmer"
        return web.json_response({"alea_b": a.pair["alea_b"].hex()})

    async def appairage_confirmer(self, request):
        a = self.appairage
        c = await request.json()
        if not self._session(a, c, "a_confirmer"):
            return erreur(403, "session d'appairage invalide")
        jeton_pour_b = str(c.get("jeton_pour_b", ""))
        if not 20 <= len(jeton_pour_b) <= 128:
            return erreur(400, "jeton invalide")
        try:
            await asyncio.wait_for(a.decision.wait(), timeout=max(0.0, a.expire - time.monotonic()))
        except asyncio.TimeoutError:
            a.etat = "expire"
            return erreur(408, "pas de décision sur ce PC dans le temps imparti")
        if not a.accepte:
            return erreur(403, "appairage refusé sur ce PC")
        jeton_pour_a = S.nouveau_jeton()
        p = a.pair
        self.appareils.ajouter_pc(p["fp"], p["nom"], p["ca_pem"], p["ip"], p["port"],
                                  jeton_sortant=jeton_pour_b, jeton_entrant=jeton_pour_a)
        a.etat = "termine"
        self.journal(f"appairage : « {p['nom']} » ajouté ({empreinte_lisible(p['fp'])[:23]}…)")
        return web.json_response({"jeton_pour_a": jeton_pour_a})

    # --- transfert PC -> PC par blocs ------------------------------------------

    async def transfert_preparer(self, request):
        app = self._authentifier(request)
        try:
            c = await request.json()
            nom = S.nom_sur(str(c["nom"]))
            taille, taille_bloc, cle = int(c["taille"]), int(c["taille_bloc"]), str(c["cle"])[:128]
        except (ValueError, KeyError, TypeError):
            return erreur(400, "demande invalide")
        if not (0 <= taille <= TAILLE_MAX and TAILLE_BLOC_MIN <= taille_bloc <= TAILLE_BLOC_MAX):
            return erreur(400, "taille ou taille de bloc hors limites")
        self.cfg.dossier_reception.mkdir(parents=True, exist_ok=True)
        if shutil.disk_usage(self.cfg.dossier_reception).free < taille:
            return erreur(507, "espace disque insuffisant")
        ident = hashlib.sha256(f"{app['cle']}|{cle}|{taille}|{taille_bloc}".encode()).hexdigest()[:32]
        rec = await asyncio.to_thread(Reception.ouvrir, self.tmp, ident, nom, taille, taille_bloc, app["cle"])
        self.receptions[ident] = rec
        return web.json_response({"id": ident, "manquants": rec.manquants()})

    def _reception(self, request, app) -> Reception:
        rec = self.receptions.get(request.match_info["ident"])
        if not rec:
            raise web.HTTPNotFound(text='{"erreur": "transfert inconnu : rappeler preparer"}',
                                   content_type="application/json")
        if rec.meta["appareil"] != app["cle"]:
            raise web.HTTPForbidden(text='{"erreur": "transfert d\'un autre appareil"}',
                                    content_type="application/json")
        return rec

    async def transfert_bloc(self, request):
        app = self._authentifier(request)
        rec = self._reception(request, app)
        donnees = await request.read()
        raison = await asyncio.to_thread(rec.ecrire_bloc, int(request.match_info["i"]), donnees,
                                         request.headers.get("X-Bloc-SHA256", ""))
        if raison:
            return erreur(422, raison)
        return web.Response(status=204)

    async def transfert_terminer(self, request):
        app = self._authentifier(request)
        rec = self._reception(request, app)
        c = await request.json()
        final, raison = await asyncio.to_thread(rec.finaliser, self.cfg.dossier_reception, str(c.get("racine", "")))
        if not final:
            return erreur(422, raison)
        del self.receptions[request.match_info["ident"]]
        self.journal(f"reçu de « {app['nom']} » : {final.name} ({final.stat().st_size / 1e6:.1f} Mo)")
        self._post_traiter(final)
        return web.json_response({"nom": final.name, "taille": final.stat().st_size})

    # --- iPhone : API rapide ---------------------------------------------------

    async def _ecrire_flux(self, source, nom: str, app: dict) -> Path:
        """Écrit un flux reçu directement sur disque, par morceaux de 4 Mio, sans tout garder en mémoire."""
        self.tmp.mkdir(parents=True, exist_ok=True)
        tmp = self.tmp / f"rapide-{secrets.token_hex(8)}.part"
        tampon = bytearray()
        f = await asyncio.to_thread(open, tmp, "wb")
        try:
            while True:
                if isinstance(source, BodyPartReader):
                    m = await source.read_chunk(1 << 20)
                else:
                    m = await source.read(1 << 20)
                if not m:
                    break
                tampon += m
                if len(tampon) >= MORCEAU:
                    await asyncio.to_thread(f.write, bytes(tampon))
                    tampon.clear()
            if tampon:
                await asyncio.to_thread(f.write, bytes(tampon))
        except BaseException:
            f.close()
            tmp.unlink(missing_ok=True)
            raise
        f.close()
        final = S.chemin_libre(self.cfg.dossier_reception, S.nom_sur(nom))
        os.replace(tmp, final)
        self.journal(f"reçu de « {app['nom']} » : {final.name} ({final.stat().st_size / 1e6:.1f} Mo)")
        self._post_traiter(final)
        return final

    async def rapide_fichier(self, request):
        app = self._authentifier(request)
        self.cfg.dossier_reception.mkdir(parents=True, exist_ok=True)
        if request.content_length and shutil.disk_usage(self.cfg.dossier_reception).free < request.content_length:
            return erreur(507, "espace disque insuffisant")
        recus = []
        if request.content_type.startswith("multipart/"):
            lecteur = await request.multipart()
            while (partie := await lecteur.next()) is not None:
                if partie.filename:
                    recus.append(await self._ecrire_flux(partie, partie.filename, app))
        else:
            nom = unquote(request.headers.get("X-Nom", "")) or f"fichier-{dt.datetime.now():%Y%m%d-%H%M%S}"
            recus.append(await self._ecrire_flux(request.content, nom, app))
        if not recus:
            return erreur(400, "aucun fichier dans la requête")
        return web.json_response({"recus": [p.name for p in recus]})

    async def rapide_lien(self, request):
        app = self._authentifier(request)
        url = ""
        if request.content_type == "application/json":
            url = str((await request.json()).get("url", ""))
        elif request.content_type in ("application/x-www-form-urlencoded", "multipart/form-data"):
            url = str((await request.post()).get("url", ""))
        else:
            url = (await request.text()).strip()
        url = url.strip()
        if not S.url_autorisee(url):
            return erreur(400, "lien refusé : seuls http et https sont acceptés")
        with open(self.cfg.donnees / "liens.txt", "a", encoding="utf-8") as f:
            f.write(f"{dt.datetime.now().isoformat(timespec='seconds')}  {app['nom']}  {url}\n")
        ouvert = False
        if self.cfg.ouvrir_liens:
            ouvert = await asyncio.to_thread(webbrowser.open, url)
        self.journal(f"lien reçu de « {app['nom']} » : {url}")
        return web.json_response({"ouvert": bool(ouvert)})

    # --- partage PC -> iPhone (QR code) ----------------------------------------

    async def page_partage(self, request):
        jeton = request.match_info["jeton"]
        p = self.partages.obtenir(jeton)
        if not p:
            return web.Response(text=PAGE.format(titre="Lien expiré", contenu="<p>Ce lien a expiré ou a déjà été utilisé.</p>"),
                                content_type="text/html", status=404, headers=ENTETES_PAGE)
        lignes = []
        if p.texte:
            lignes.append('<p>Appui long sur le texte, puis « Copier ». Cette page ne s\'affichera qu\'une fois.</p>'
                          f'<pre class="texte">{html.escape(p.texte)}</pre>')
            self.partages.consommer(jeton)
        if p.url:
            u = html.escape(p.url, quote=True)
            lignes.append(f'<a class="bouton" href="{u}" rel="noopener noreferrer">Ouvrir le lien</a><p class="url">{u}</p>')
        for i, chemin in enumerate(p.fichiers):
            taille = chemin.stat().st_size / 1e6 if chemin.exists() else 0
            lignes.append(f'<a class="bouton" href="/p/{jeton}/{i}">{html.escape(chemin.name)}</a>'
                          f'<p class="taille">{taille:.1f} Mo</p>')
        return web.Response(text=PAGE.format(titre=f"Depuis {html.escape(self.cfg.nom)}", contenu="".join(lignes)),
                            content_type="text/html", headers=ENTETES_PAGE)

    async def telecharger_partage(self, request):
        jeton, i = request.match_info["jeton"], int(request.match_info["i"])
        p = self.partages.obtenir(jeton)
        if not p or i >= len(p.fichiers) or not p.fichiers[i].is_file():
            return erreur(404, "lien expiré ou fichier introuvable")
        chemin = p.fichiers[i]
        entetes = {"Content-Disposition": f"attachment; filename*=UTF-8''{quote(chemin.name)}"}
        if chemin.suffix.lower() == ".mobileconfig":
            entetes = {"Content-Type": "application/x-apple-aspen-config"}
        def apres_envoi():
            if "Range" not in request.headers:   # téléchargement complet : on le compte
                self.partages.marquer(jeton, i)
                self.journal(f"partage : {chemin.name} téléchargé par {request.remote}")

        return _FichierPuisRappel(chemin, headers=entetes, apres=apres_envoi)

    # --- API de contrôle locale ------------------------------------------------

    async def controle_etat(self, request):
        return web.json_response({
            "nom": self.cfg.nom, "empreinte": self.identite.empreinte, "port": self.cfg.port,
            "mdns": f"{self.cfg.nom_mdns}.local" if self.mdns else None,
            "reception": str(self.cfg.dossier_reception),
            "adresses": [{"ip": ip, "type": t} for ip, t in ips_classees()],
            "appareils": self.appareils.liste()})

    async def controle_vus(self, request):
        return web.json_response(self.decouverte.actifs() if self.decouverte else {})

    async def controle_appairage_ouvrir(self, request):
        self.appairage = Appairage(DUREE_APPAIRAGE_S)
        self.journal(f"appairage : fenêtre ouverte pour {DUREE_APPAIRAGE_S} s")
        return web.json_response({"duree": DUREE_APPAIRAGE_S})

    async def controle_appairage(self, request):
        a = self.appairage
        if not a:
            return web.json_response({"etat": "ferme"})
        etat = a.etat if a.ouverte or a.etat in ("termine", "refuse") else "expire"
        rep = {"etat": etat, "reste": max(0, int(a.expire - time.monotonic()))}
        if a.pair:
            rep.update(nom=a.pair["nom"], ip=a.pair["ip"], empreinte=empreinte_lisible(a.pair["fp"]))
        if a.etat == "a_confirmer":
            rep["code"] = a.code
        return web.json_response(rep)

    async def controle_appairage_decision(self, request):
        a = self.appairage
        c = await request.json()
        if not a or a.etat != "a_confirmer":
            return erreur(409, "aucun code à confirmer")
        a.accepte = bool(c.get("accepter"))
        if not a.accepte:
            a.etat = "refuse"
            self.journal("appairage : refusé par l'utilisateur")
        a.decision.set()
        return web.json_response({"ok": True})

    async def controle_partage(self, request):
        c = await request.json()
        fichiers = [Path(x) for x in c.get("chemins", [])]
        url = c.get("url")
        texte = c.get("texte")
        if url and not S.url_autorisee(url):
            return erreur(400, "lien refusé : seuls http et https sont acceptés")
        absents = [str(f) for f in fichiers if not f.is_file()]
        if absents:
            return erreur(400, f"fichiers introuvables : {', '.join(absents)}")
        if not fichiers and not url and not texte:
            return erreur(400, "rien à partager")
        jeton = self.partages.creer(fichiers, url, texte=texte)
        urls = [{"url": f"https://{ip}:{self.cfg.port}/p/{jeton}", "type": t} for ip, t in ips_classees()]
        return web.json_response({"urls": urls})

    # --- démarrage ------------------------------------------------------------

    def _socket_ecoute(self) -> socket.socket:
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        if sys.platform == "win32":
            # SO_REUSEADDR sous Windows laisserait un autre programme voler le port : on fait l'inverse.
            s.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
        else:
            s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4 * 1024 * 1024)
        s.bind(("0.0.0.0", self.cfg.port))     # toutes les interfaces : box, partage iPhone, point d'accès
        s.listen(128)
        s.setblocking(False)
        return s

    async def _surveiller(self) -> None:
        """Toutes les 15 s : nouvelle IP (partage de connexion activé) -> nouveau certificat, rechargé à chaud."""
        while True:
            await asyncio.sleep(15)
            try:
                if await asyncio.to_thread(self.identite.verifier_certificat_serveur):
                    self.identite.recharger(self.ssl)
                    self.journal("nouvelle adresse IP : certificat serveur réémis")
                if self.mdns and await self.mdns.actualiser():
                    self.journal("nouvelle adresse IP : annonce Bonjour mise à jour")
            except Exception as e:  # la surveillance ne doit jamais arrêter le démon
                self.journal(f"surveillance : {e}")
            self.partages.nettoyer()

    async def demarrer(self) -> None:
        self.cfg.dossier_reception.mkdir(parents=True, exist_ok=True)
        self.ssl = self.identite.contexte_serveur()

        public = web.AppRunner(self.app_publique(), access_log=None)
        await public.setup()
        await web.SockSite(public, self._socket_ecoute(), ssl_context=self.ssl).start()
        controle = web.AppRunner(self.app_controle(), access_log=None)
        await controle.setup()
        await web.TCPSite(controle, "127.0.0.1", self.cfg.port_controle).start()
        self._runners = [public, controle]
        ecrire_json(self.cfg.donnees / "controle.json",
                    {"port": self.cfg.port_controle, "secret": self.secret_controle}, secret=True)

        if self.decouverte:
            try:
                await self.decouverte.demarrer()
            except OSError as e:
                self.journal(f"découverte indisponible ({e}) : saisie d'IP ou QR code en repli")
                self.decouverte = None
        if self.cfg.mdns:
            try:
                from .mdns import AnnonceMdns
                self.mdns = AnnonceMdns(self.cfg.nom, self.cfg.port, self.identite.empreinte)
                await self.mdns.demarrer()
                self.journal(f"Bonjour : ce PC répond au nom {self.mdns.hote}.local")
            except Exception as e:
                self.journal(f"Bonjour indisponible ({e}) : le Raccourci devra utiliser une IP")
                self.mdns = None
        self._taches.append(asyncio.create_task(self._surveiller()))
        self.journal(f"démon « {self.cfg.nom} » prêt, port {self.cfg.port}, réception dans {self.cfg.dossier_reception}")
        for ip, t in ips_classees():
            self.journal(f"   https://{ip}:{self.cfg.port}   ({t})")

    async def arreter(self) -> None:
        for t in self._taches:
            t.cancel()
        if self.decouverte:
            await self.decouverte.arreter()
        if self.mdns:
            await self.mdns.arreter()
        for r in self._runners:
            await r.cleanup()
        (self.cfg.donnees / "controle.json").unlink(missing_ok=True)


ENTETES_PAGE = {
    "Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'",
    "X-Frame-Options": "DENY",
}

PAGE = """<!doctype html><html lang="fr"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1"><title>{titre}</title>
<style>
body{{font-family:-apple-system,system-ui,sans-serif;max-width:32rem;margin:2rem auto;padding:0 1rem;
background:#f6f5f2;color:#1d1c1a}}
@media (prefers-color-scheme:dark){{body{{background:#1b1a18;color:#ecebe7}}.bouton{{background:#2a2926!important}}}}
h1{{font-size:1.3rem}}.bouton{{display:block;padding:1rem;margin:.75rem 0 .25rem;border-radius:12px;
background:#fff;text-decoration:none;color:inherit;font-weight:600;word-break:break-all;
box-shadow:0 1px 2px rgba(0,0,0,.08)}}.texte{{white-space:pre-wrap;word-break:break-all;padding:1rem;
border-radius:12px;background:rgba(127,127,127,.12);font-size:.95rem;-webkit-user-select:all;user-select:all}}.taille,.url{{margin:0 0 .5rem .25rem;opacity:.65;font-size:.85rem;word-break:break-all}}
</style></head><body><h1>{titre}</h1>{contenu}</body></html>"""
