# universal-airdrop

Un « AirDrop » maison entre PC Windows et iPhone : PDF, photos, vidéos et liens, sur le réseau local,
**chiffré, authentifié et rapide**. Il fonctionne via la box, en **partage de connexion** Wi-Fi, ou en
**Bluetooth**, sans Internet.

| Lien | Débit utile estimé | Usage |
|---|---|---|
| Wi-Fi (box ou partage de connexion) | 150 à 400 Mbit/s | tout, vidéos comprises |
| Bluetooth (partage de connexion de l'iPhone, PAN) | 1 à 2 Mbit/s | liens, PDF, photos |

Le Bluetooth passe par le partage de connexion de l'iPhone, car iOS n'offre pas de transfert de
fichiers Bluetooth vers un PC. Le protocole reste le même : HTTPS sur IP.

Sur l'iPhone : **Partager → PC de Camil**, et c'est envoyé.

| Sens | Comment |
|---|---|
| iPhone → PC | Raccourci iOS « PC de Camil » dans la feuille de partage ([guide](docs/RACCOURCI_IOS.md)) |
| PC → iPhone | `python -m transfert partager fichier.pdf` → QR code à scanner avec l'appareil photo |
| PC → PC | `python -m transfert envoyer "PC fixe" video.mp4`, ou clic droit → Envoyer vers |
| Liens | Safari → Partager → PC de Camil ; ou `python -m transfert lien "PC fixe" https://…` |

## Installation (invite cmd, sur chaque PC)

```bat
git clone https://github.com/Cams457/universal-airdrop
cd universal-airdrop
python -m pip install -r requirements.txt
python verif_env.py
python -m transfert nommer "PC de Camil"
python -m transfert pare-feu
```

`pare-feu` affiche les deux règles à ajouter. Relis-les, puis exécute `pare_feu.cmd` dans une invite cmd
**ouverte en administrateur**.

Ensuite, laisse tourner le démon :

```bat
python -m transfert demon
```

## Premières étapes

**Ajouter l'iPhone** (dans une seconde fenêtre cmd, démon lancé) :

```bat
python -m transfert iphone-ajouter "iPhone de Camil"
```

Puis suis [docs/RACCOURCI_IOS.md](docs/RACCOURCI_IOS.md).

**Appairer deux PC** :
- sur le PC B : `python -m transfert appairage-ouvrir` ;
- sur le PC A : `python -m transfert appairer <IP de B>`.

Les deux écrans affichent un code à 6 chiffres. On valide seulement s'ils sont identiques.

**Clic droit « Envoyer vers »** et lancement au démarrage :

```bat
python -m transfert installer-windows --demarrage
```

## Commandes

| Commande | Rôle |
|---|---|
| `demon` | serveur HTTPS (à laisser tourner) |
| `nommer "PC de Camil"` | nom affiché, et nom réseau `pc-de-camil.local` |
| `adresses` | IP du PC par réseau : box, partage iPhone, point d'accès mobile |
| `empreinte` | empreinte de la CA du PC (à comparer sur l'iPhone) |
| `appairage-ouvrir` / `appairer HOTE` | appairage PC ↔ PC par code |
| `envoyer CIBLE FICHIERS…` | envoi PC → PC par blocs, avec reprise |
| `lien CIBLE URL` | ouvre un lien sur l'autre PC |
| `partager CHEMIN\|URL…` | PC → iPhone par QR code (10 min, usage unique) |
| `iphone-ajouter NOM` / `revoquer NOM` | gestion des appareils |
| `appareils` | liste des appareils de confiance |
| `pare-feu` | commandes netsh à relire |
| `installer-windows` / `desinstaller-windows` | clic droit « Envoyer vers » et démarrage automatique |

## Vitesse

Le logiciel n'est plus la limite. Avec `python bench.py --local`, sur une machine de test à
2 cœurs, en deux processus séparés :

| Cas | Débit |
|---|---|
| 40 photos de 3 Mo | environ 2 000 Mbit/s |
| Gros fichier | 2 300 à 2 900 Mbit/s |
| Démarrage | moins de 70 ms |

C'est la radio qui fixe le plafond. Le Wi-Fi 6 en 2×2 et 80 MHz donne environ 1 200 Mbit/s de débit
physique, soit 600 à 800 Mbit/s utiles en un seul saut. Le détail, la dérivation et le matériel
nécessaire sont dans [docs/VITESSE.md](docs/VITESSE.md).

Sur le vrai réseau, mesure d'abord le débit brut avec iperf3, puis :

```bat
python bench.py --cible "PC fixe" --iperf <débit iperf3 en Mbit/s>
```

La cible est η = débit utile / débit iperf3 ≥ 0,8.

## Sécurité

Voir [docs/SECURITE.md](docs/SECURITE.md) :
- modèle de menace ;
- protocole d'appairage avec engagement ;
- limites connues.

## Structure

```
transfert/
  serveur.py     démon HTTPS : transferts, API iPhone, partages, appairage, contrôle local
  client.py      appairage (côté initiateur) et envoi par blocs parallèles
  blocs.py       découpage, SHA-256 par bloc, reprise après coupure
  identite.py    CA locale + certificat serveur (réémis quand l'IP change)
  securite.py    noms sûrs, IP autorisées, URL, jetons, engagement, code de vérification
  appareils.py   registre des appareils de confiance (rechargé à chaud)
  mdns.py        annonce Bonjour « pc-de-camil.local »
  decouverte.py  découverte des PC par multicast, sur chaque interface
  partage.py     liens de partage à usage unique (QR code)
  profil_ios.py  profil .mobileconfig pour l'iPhone
  windows.py     « Envoyer vers » et démarrage automatique
v0/              V0 : envoi TCP minimal en clair (étape d'apprentissage)
tests/           pytest : 39 tests, dont deux démons réels en HTTPS
```

Tests : `python -m pip install pytest` puis `python -m pytest`.

## Avancement (feuille de route)

| Phase | État |
|---|---|
| Phase 0 — Préparation | `verif_env.py` prêt ; à lancer sur chaque PC |
| V0 — PC → PC en TCP | fait et testé |
| V1 — iPhone → PC (Raccourci) | API faite et testée ; Raccourci à construire, test vidéo > 500 Mo à faire |
| V2 — Blocs, reprise, vitesse | fait et testé ; mesures réelles à faire |
| V3 — PC → iPhone (QR code) | fait et testé ; scan réel à faire |
| V4 — Compatibilité LocalSend | non commencé : dépend de la décision de la V1 |
| V5 — Sécurité | fait et testé dès la V1 (HTTPS, épinglage, jetons, code avec engagement) |
| V6 — Intégration Windows | « Envoyer vers » et démarrage faits ; icône de notification à faire |
| Partage de connexion | fait (multi-interfaces, certificat réémis à chaud, Bonjour) ; à tester sur le terrain |
| Bluetooth | fait (via le PAN du partage de l'iPhone, interface reconnue, avertissement de débit) ; à tester |
