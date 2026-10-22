"""Intégration Windows (V6) : entrée « Envoyer vers » du clic droit et lancement au démarrage.

Aucune modification du registre : uniquement des fichiers .cmd déposés dans les dossiers
shell:sendto et shell:startup de l'utilisateur. Désinstaller = supprimer ces deux fichiers.
"""

import os
import sys
from pathlib import Path

NOM_ENVOYER_VERS = "Transfert vers un appareil.cmd"
NOM_DEMARRAGE = "Transfert (demon).cmd"


def _appdata() -> Path:
    return Path(os.environ.get("APPDATA", Path.home() / "AppData" / "Roaming"))


def dossier_envoyer_vers() -> Path:
    return _appdata() / "Microsoft" / "Windows" / "SendTo"


def dossier_demarrage() -> Path:
    return _appdata() / "Microsoft" / "Windows" / "Start Menu" / "Programs" / "Startup"


def _python(fenetre: bool) -> str:
    exe = Path(sys.executable)
    if not fenetre and exe.name.lower() == "python.exe" and (exe.parent / "pythonw.exe").exists():
        exe = exe.parent / "pythonw.exe"      # démon sans fenêtre de console
    return str(exe)


def contenu_envoyer_vers(racine_projet: Path) -> str:
    return (f'@echo off\r\ncd /d "{racine_projet}"\r\n'
            f'"{_python(True)}" -m transfert envoyer-choisir %*\r\n'
            'if errorlevel 1 pause\r\n')


def contenu_demarrage(racine_projet: Path) -> str:
    return (f'@echo off\r\ncd /d "{racine_projet}"\r\n'
            f'start "" "{_python(False)}" -m transfert demon\r\n')


def installer(racine_projet: Path, demarrage: bool) -> list[Path]:
    crees = []
    cible = dossier_envoyer_vers() / NOM_ENVOYER_VERS
    cible.parent.mkdir(parents=True, exist_ok=True)
    cible.write_text(contenu_envoyer_vers(racine_projet), encoding="mbcs" if sys.platform == "win32" else "utf-8")
    crees.append(cible)
    if demarrage:
        cible = dossier_demarrage() / NOM_DEMARRAGE
        cible.parent.mkdir(parents=True, exist_ok=True)
        cible.write_text(contenu_demarrage(racine_projet), encoding="mbcs" if sys.platform == "win32" else "utf-8")
        crees.append(cible)
    return crees


def desinstaller() -> list[Path]:
    retires = []
    for p in (dossier_envoyer_vers() / NOM_ENVOYER_VERS, dossier_demarrage() / NOM_DEMARRAGE):
        if p.exists():
            p.unlink()
            retires.append(p)
    return retires


def choisir_appareil(noms: list[str], fichiers: list[str]) -> str | None:
    """Petite fenêtre de choix de l'appareil destinataire (tkinter, fourni avec Python)."""
    import tkinter as tk
    choix = {"nom": None}
    racine = tk.Tk()
    racine.title("Transfert")
    racine.attributes("-topmost", True)
    tk.Label(racine, text=f"Envoyer {len(fichiers)} fichier(s) vers :", padx=16, pady=10).pack()
    liste = tk.Listbox(racine, height=min(8, max(3, len(noms))), width=36, activestyle="dotbox")
    for n in noms:
        liste.insert("end", n)
    liste.pack(padx=16)
    if noms:
        liste.selection_set(0)
        liste.focus_set()

    def valider(_=None):
        sel = liste.curselection()
        if sel:
            choix["nom"] = noms[sel[0]]
        racine.destroy()

    liste.bind("<Double-Button-1>", valider)
    racine.bind("<Return>", valider)
    racine.bind("<Escape>", lambda _: racine.destroy())
    tk.Button(racine, text="Envoyer", command=valider, width=12).pack(pady=12)
    racine.mainloop()
    return choix["nom"]
