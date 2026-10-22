"""Côté émetteur : appairage (côté initiateur A) et envoi PC -> PC par blocs parallèles.

Vitesse : plusieurs blocs sont en vol en même temps (connexions parallèles réutilisées), et
la lecture disque + le SHA-256 du bloc suivant se font dans un thread pendant que le réseau
transporte le précédent. Le premier bloc part sans attendre le hachage du fichier entier.
"""

import asyncio
import hashlib
import inspect
import time
from dataclasses import dataclass, field
from pathlib import Path

import aiohttp

from . import securite as S
from .appareils import Appareils
from .blocs import lire_et_hacher, nombre_blocs, racine
from .config import DUREE_APPAIRAGE_S, TAILLE_BLOC, Config
from .controle import DemonAbsent, appeler
from .identite import (Identite, contexte_client_epingle, contexte_client_premier_contact,
                       empreinte_lisible, empreinte_pem)

ESSAIS = 4


class ErreurTransfert(Exception):
    pass


async def _verifier(r: aiohttp.ClientResponse) -> None:
    if r.status >= 400:
        try:
            message = (await r.json()).get("erreur", r.reason)
        except (aiohttp.ContentTypeError, ValueError):
            message = r.reason
        raise ErreurTransfert(f"{r.status} : {message}")


# --- appairage ------------------------------------------------------------------

async def appairer(cfg: Config, identite: Identite, appareils: Appareils, hote: str, port: int,
                   confirmer) -> dict:
    """Appaire ce PC (A) avec le PC distant (B), qui doit avoir ouvert une fenêtre d'appairage.

    confirmer(code, nom_b, empreinte_b) -> bool affiche le code et demande à l'utilisateur
    s'il est identique à celui de l'autre écran.
    """
    base = f"https://{hote}:{port}"
    timeout = aiohttp.ClientTimeout(total=None, sock_connect=5, sock_read=DUREE_APPAIRAGE_S + 15)
    async with aiohttp.ClientSession(timeout=timeout) as s:
        # 1. Premier contact : chiffré mais pas encore authentifié. B renvoie sa CA et
        #    s'engage sur son aléa r_B AVANT de connaître r_A.
        async with s.post(f"{base}/api/v1/appairage/debut", ssl=contexte_client_premier_contact(),
                          json={"nom": cfg.nom, "ca_pem": identite.ca_pem, "port": cfg.port}) as r:
            await _verifier(r)
            d = await r.json()
        ca_pem_b = d["ca_pem"]
        fp_b = empreinte_pem(ca_pem_b)
        # 2. À partir d'ici, seul le détenteur de cette CA peut répondre : c'est elle que le code authentifie.
        ctx = contexte_client_epingle(ca_pem_b)
        alea_a = S.nouvel_alea()
        async with s.post(f"{base}/api/v1/appairage/revele", ssl=ctx,
                          json={"session": d["session"], "alea_a": alea_a.hex()}) as r:
            await _verifier(r)
            alea_b = bytes.fromhex((await r.json())["alea_b"])
        if not S.egal_temps_constant(S.engagement(fp_b, identite.empreinte, alea_b), d["engagement"]):
            raise ErreurTransfert("engagement non respecté par le pair : attaque possible, appairage annulé")
        code = S.code_verification(identite.empreinte, fp_b, alea_a, alea_b)
        ok = confirmer(code, d["nom"], empreinte_lisible(fp_b))
        if inspect.isawaitable(ok):
            ok = await ok
        if not ok:
            raise ErreurTransfert("appairage annulé : les codes ne correspondent pas")
        # 3. Confirmation : A donne à B le jeton que B utilisera pour envoyer vers A, et réciproquement.
        jeton_pour_b = S.nouveau_jeton()
        async with s.post(f"{base}/api/v1/appairage/confirmer", ssl=ctx,
                          json={"session": d["session"], "jeton_pour_b": jeton_pour_b}) as r:
            await _verifier(r)
            jeton_pour_a = (await r.json())["jeton_pour_a"]
    appareils.ajouter_pc(fp_b, d["nom"], ca_pem_b, hote, int(d["port"]),
                         jeton_sortant=jeton_pour_a, jeton_entrant=jeton_pour_b)
    return {"nom": d["nom"], "empreinte": fp_b}


# --- résolution de l'adresse d'un pair -----------------------------------------

def _candidats(cfg: Config, fp: str, info: dict) -> list[tuple[str, int]]:
    """Adresses à essayer : d'abord celle vue récemment par la découverte, puis la dernière connue."""
    candidats = []
    try:
        vu = appeler(cfg, "GET", "/controle/vus", timeout=1).get(fp)
        if vu:
            candidats.append((vu["ip"], vu["port"]))
    except (DemonAbsent, RuntimeError):
        pass
    if (info["adresse"], info["port"]) not in candidats:
        candidats.append((info["adresse"], info["port"]))
    return candidats


async def _joindre(s: aiohttp.ClientSession, candidats, ctx) -> str:
    """Première adresse qui répond avec la bonne CA ; délai court pour tenir la latence sous 2 s."""
    erreurs = []
    for ip, port in candidats:
        base = f"https://{ip}:{port}"
        try:
            async with s.get(f"{base}/api/v1/info", ssl=ctx, timeout=aiohttp.ClientTimeout(total=1.5)) as r:
                if r.status == 200:
                    return base
        except (aiohttp.ClientError, asyncio.TimeoutError) as e:
            erreurs.append(f"{ip}:{port} ({type(e).__name__})")
    raise ErreurTransfert("pair injoignable : " + ", ".join(erreurs)
                          + ". Vérifie qu'il est allumé, sur le même réseau, et que son démon tourne.")


def resoudre(appareils: Appareils, cible: str) -> tuple[str, dict]:
    r = appareils.pc(cible)
    if not r:
        raise ErreurTransfert(f"« {cible} » n'est pas un PC appairé : lance d'abord « transfert appairer »")
    return r


# --- envoi ----------------------------------------------------------------------

@dataclass
class Bilan:
    nom: str
    taille: int
    duree: float
    latence_demarrage: float
    blocs_envoyes: int
    blocs_total: int
    nom_recu: str = ""

    @property
    def debit_mbits(self) -> float:
        return 8 * self.taille / self.duree / 1e6 if self.duree > 0 else 0.0


@dataclass
class Progres:
    total: int
    fait: int = 0
    debut: float = field(default_factory=time.perf_counter)


async def envoyer_fichier(s: aiohttp.ClientSession, base: str, jeton: str, chemin: Path,
                          paralleles: int, taille_bloc: int = TAILLE_BLOC, rappel=None) -> Bilan:
    t0 = time.perf_counter()
    st = chemin.stat()
    cle = hashlib.sha256(f"{chemin.resolve()}|{st.st_size}|{st.st_mtime_ns}".encode()).hexdigest()
    auth = {"Authorization": f"Bearer {jeton}"}
    async with s.post(f"{base}/api/v1/transfert/preparer", headers=auth,
                      json={"nom": chemin.name, "taille": st.st_size, "taille_bloc": taille_bloc, "cle": cle}) as r:
        await _verifier(r)
        d = await r.json()
    ident, manquants = d["id"], set(d["manquants"])
    n = nombre_blocs(st.st_size, taille_bloc)
    hachages: list[str | None] = [None] * n
    file = asyncio.Queue()
    for i in range(n):
        file.put_nowait(i)
    progres = Progres(total=st.st_size)
    premier = [None]

    async def ouvrier():
        while True:
            try:
                i = file.get_nowait()
            except asyncio.QueueEmpty:
                return
            donnees, h = await asyncio.to_thread(lire_et_hacher, chemin, i, taille_bloc)
            hachages[i] = h
            if i in manquants:
                for essai in range(ESSAIS):
                    try:
                        async with s.put(f"{base}/api/v1/transfert/{ident}/bloc/{i}", data=donnees,
                                         headers={**auth, "X-Bloc-SHA256": h,
                                                  "Content-Type": "application/octet-stream"}) as r:
                            await _verifier(r)
                        break
                    except (aiohttp.ClientError, asyncio.TimeoutError) as e:
                        if essai == ESSAIS - 1:
                            raise ErreurTransfert(f"bloc {i} : {e} ; relance la commande pour reprendre") from e
                        await asyncio.sleep(0.5 * 2 ** essai)
                if premier[0] is None:
                    premier[0] = time.perf_counter() - t0
            progres.fait += len(donnees)
            if rappel:
                rappel(chemin.name, progres)

    await asyncio.gather(*(ouvrier() for _ in range(max(1, min(paralleles, n)))))
    async with s.post(f"{base}/api/v1/transfert/{ident}/terminer", headers=auth,
                      json={"racine": racine(hachages)}) as r:
        await _verifier(r)
        recu = await r.json()
    duree = time.perf_counter() - t0
    return Bilan(chemin.name, st.st_size, duree, premier[0] if premier[0] is not None else duree,
                 len(manquants), n, recu["nom"])


async def envoyer(cfg: Config, identite: Identite, appareils: Appareils, cible: str, chemins: list[Path],
                  rappel=None, taille_bloc: int = TAILLE_BLOC) -> list[Bilan]:
    fp, info = resoudre(appareils, cible)
    ctx = contexte_client_epingle(info["ca_pem"])
    connecteur = aiohttp.TCPConnector(ssl=ctx, limit=max(cfg.paralleles, 1) + 1)
    timeout = aiohttp.ClientTimeout(total=None, sock_connect=5, sock_read=120)
    async with aiohttp.ClientSession(connector=connecteur, timeout=timeout) as s:
        base = await _joindre(s, _candidats(cfg, fp, info), ctx)
        bilans = []
        for chemin in chemins:
            bilans.append(await envoyer_fichier(s, base, info["jeton_sortant"], Path(chemin),
                                                cfg.paralleles, taille_bloc, rappel))
    return bilans


async def envoyer_lien(cfg: Config, appareils: Appareils, cible: str, url: str) -> dict:
    if not S.url_autorisee(url):
        raise ErreurTransfert("seuls les liens http et https sont acceptés")
    fp, info = resoudre(appareils, cible)
    ctx = contexte_client_epingle(info["ca_pem"])
    async with aiohttp.ClientSession(connector=aiohttp.TCPConnector(ssl=ctx)) as s:
        base = await _joindre(s, _candidats(cfg, fp, info), ctx)
        async with s.post(f"{base}/api/rapide/lien", json={"url": url},
                          headers={"Authorization": f"Bearer {info['jeton_sortant']}"}) as r:
            await _verifier(r)
            return await r.json()
