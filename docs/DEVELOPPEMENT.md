# Développement

## Compiler et tester

Prérequis : [Go](https://go.dev/dl/) 1.26 ou plus récent. Dans une invite cmd :

```bat
git clone https://github.com/Cams457/universal-airdrop
cd universal-airdrop
go test ./...
go build -trimpath -ldflags "-H windowsgui -s -w" -o UniversalAirdrop.exe ./cmd/universal-airdrop
```

`-H windowsgui` produit une application graphique, sans fenêtre noire au double-clic. Lancé depuis cmd
avec des arguments, l'exécutable réutilise la console de cmd pour afficher ses messages
(`UniversalAirdrop.exe aide`).

L'icône et la description de l'exécutable viennent des fichiers `cmd/universal-airdrop/rsrc_windows_*.syso`.
Pour les régénérer :

```bat
go install github.com/tc-hib/go-winres@latest
cd cmd\universal-airdrop
go-winres simply --arch amd64,arm64 --manifest gui --icon ..\..\assets\icone.png --product-name "Universal Airdrop"
```

Sous Linux ou macOS, `go build ./cmd/universal-airdrop` produit une version sans icône de notification.

Les tests d'intégration démarrent deux services réels en HTTPS. Ils vérifient l'appairage, les envois
dans les deux sens, la reprise, les blocs corrompus, les jetons, la limitation des échecs, la révocation
à chaud, l'API iPhone, la page d'envoi pour téléphone, les partages à usage unique et la sécurité de
l'interface locale. `go test -race ./...` les exécute sous le détecteur de concurrence.

## Publier une version

Les exécutables téléchargés depuis le README sont dans `telecharger/` (liens directs, sans page de
*release*). Pour en publier une nouvelle version, dans une invite cmd à la racine du dépôt :

```bat
set V=1.0.8
set GOOS=windows
set CGO_ENABLED=0
set GOARCH=amd64
go build -trimpath -ldflags "-H windowsgui -s -w -X main.version=%V%" -o telecharger\UniversalAirdrop.exe .\cmd\universal-airdrop
set GOARCH=arm64
go build -trimpath -ldflags "-H windowsgui -s -w -X main.version=%V%" -o telecharger\UniversalAirdrop-arm64.exe .\cmd\universal-airdrop
set GOARCH=
set GOOS=
cd telecharger
certutil -hashfile UniversalAirdrop.exe SHA256
certutil -hashfile UniversalAirdrop-arm64.exe SHA256
```

Reporte les deux empreintes dans `telecharger/SHA256SUMS.txt` et la version dans le README, puis
commite. Le numéro de version sert aussi au remplacement automatique de l'instance en cours.

En plus, un tag `v1.2.3` poussé sur GitHub déclenche GitHub Actions : tests sous Linux et Windows,
compilation des deux exécutables et publication dans une *release*.

## Architecture

```
cmd/universal-airdrop   point d'entrée : application (icône + interface) ou ligne de commande
internal/
  serveur      service : HTTPS sur le réseau local, interface de contrôle sur 127.0.0.1
  client       appairage côté initiateur, envoi par blocs parallèles, choix du chemin
  blocs        découpage, SHA-256 par bloc, reprise, revérification sur disque
  identite     CA locale + certificat serveur (réémis à chaud quand l'IP change), épinglage
  securite     noms sûrs, IP autorisées, liens, jetons, engagement, code à 6 chiffres
  appareils    registre des appareils de confiance (rechargé à chaud)
  reseau       interfaces, type de lien et débit (GetAdaptersAddresses sous Windows), Tailscale
  decouverte   annonces multicast sur chaque interface
  mdns         nom Bonjour « nom-du-pc.local »
  partage      liens à usage unique (QR codes), page d'envoi pour téléphone
  boite        boîte d'envoi de chaque téléphone (fichiers, liens, textes en attente)
  profilios    profil .mobileconfig
  systeme      pare-feu (UAC), démarrage automatique, Envoyer vers, ouverture de liens
  ui           interface locale et pages pour téléphone (HTML, CSS, JS embarqués)
```

## Routes réseau (HTTPS, adresses privées uniquement)

| Méthode | Route | Rôle | Authentification |
|---|---|---|---|
| GET | `/api/v1/info` | nom et empreinte | aucune |
| POST | `/api/v1/appairage/debut`, `/revele`, `/confirmer` | appairage PC ↔ PC | fenêtre ouverte + code |
| POST | `/api/v1/transfert/preparer` | annonce d'un fichier, blocs manquants | jeton |
| PUT | `/api/v1/transfert/{id}/bloc/{i}` | un bloc (en-tête `X-Bloc-SHA256`) | jeton |
| POST | `/api/v1/transfert/{id}/terminer` | vérification de la racine, renommage | jeton |
| POST | `/api/rapide/fichier` | fichiers depuis le Raccourci (multipart ou corps brut + `X-Nom`) | jeton |
| POST | `/api/rapide/lien` | lien ou texte depuis l'app, le Raccourci ou un PC ; réponse : `copie`, `raison` (échec de la copie), relais, attente | jeton |
| GET | `/p/{jeton}` et `/p/{jeton}/{i}` | partage vers un téléphone | jeton d'URL |
| GET, POST | `/envoyer/{jeton}` | page d'envoi pour n'importe quel téléphone | jeton d'URL |
| GET | `/api/rapide/destinations` | appareils proposés dans le Raccourci *Envoyer* | jeton |
| GET | `/api/rapide/boite` | boîte d'envoi du téléphone (textes remis, URL des fichiers) | jeton de téléphone |
| GET | `/api/rapide/boite/{id}` | un fichier en attente (retiré après lecture complète) | jeton de téléphone |
| GET | `/api/v1/telephones` | téléphones de ce PC, pour le relais | jeton de PC |
| POST, GET | `/api/v1/telephone/demande[/{id}]` | ajout d'un iPhone sans QR code | acceptation sur le PC |
| GET | `/iphone`, `/profil.mobileconfig` | guide d'installation du certificat, profil iOS | aucune (données publiques) |
| GET | `/app`, `/app.webmanifest` | app iPhone (web app de l'écran d'accueil ou Safari) ; `/app#v-envoyer` ouvre un écran précis. La page `/envoyer/{jeton}` redirige vers `/app#v-envoyer` un iPhone déjà relié à la même adresse | jeton gardé par l'app |
| GET | `/api/rapide/moi` | l'app vérifie son jeton ; adresses du PC (dont Tailscale) | jeton |
| POST | `/api/rapide/boite/{id}/recu` | l'app marque un élément enregistré ; il reste dans « Reçus » (lecture avec `garder=1`) | jeton de téléphone |
| DELETE | `/api/rapide/boite/{id}` | suppression d'un élément depuis « Reçus » | jeton de téléphone |
| GET | `/api/rapide/reprendre` | l'app a perdu son jeton : le PC le lui rend depuis le cookie `ua_app` (HttpOnly, Secure, SameSite=Strict, chemin `/api/rapide/`) | cookie |
| POST | `/api/rapide/oublier` | « Délier » : efface le cookie | aucune |

Un élément de boîte est **remis** (plus compté « en attente » sur le PC) dès qu'il est sur l'iPhone : fichier lu
en entier avec `garder=1`, ou texte présent dans une réponse de `/api/rapide/boite?garder=1`.

En-têtes : `X-Destinations: PC fixe|iPad` (choix de l'émetteur, éventuellement encodé en URL) ;
entre PC, `X-De` (appareil d'origine, affiché) et `X-Pour` (téléphones du PC receveur à qui remettre l'envoi).

Le jeton s'envoie dans `Authorization: Bearer …` (ou `X-Jeton`).

## Ports
- TCP 47800 : service HTTPS ;
- TCP 47801 : interface locale (127.0.0.1) ;
- UDP 47802 : découverte multicast (groupe 239.255.47.80) ;
- UDP 5353 : Bonjour.

La règle de pare-feu porte sur le programme, ce qui couvre tous ces ports.

## Tailscale
Les adresses `100.64.0.0/10` sont traitées comme locales ; le nom MagicDNS du PC (`tailscale status --json`)
est ajouté au certificat ; `/api/v1/info` annonce toutes les adresses du PC, que les PC appairés mémorisent
et essaient (départs décalés de 300 ms) quand la découverte locale ne voit pas le pair.
