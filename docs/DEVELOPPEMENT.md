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

Pousse un tag `v1.2.3` : GitHub Actions teste sous Linux et Windows, compile les deux exécutables
(amd64, arm64) et les publie dans une nouvelle version (*release*).

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
  reseau       interfaces, type de lien et débit (GetAdaptersAddresses sous Windows)
  decouverte   annonces multicast sur chaque interface
  mdns         nom Bonjour « nom-du-pc.local »
  partage      liens à usage unique (QR codes), page d'envoi pour téléphone
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
| POST | `/api/rapide/lien` | lien depuis le Raccourci ou un PC | jeton |
| GET | `/p/{jeton}` et `/p/{jeton}/{i}` | partage vers un téléphone | jeton d'URL |
| GET, POST | `/envoyer/{jeton}` | page d'envoi pour n'importe quel téléphone | jeton d'URL |

Le jeton s'envoie dans `Authorization: Bearer …` (ou `X-Jeton`).

## Ports
- TCP 47800 : service HTTPS ;
- TCP 47801 : interface locale (127.0.0.1) ;
- UDP 47802 : découverte multicast (groupe 239.255.47.80) ;
- UDP 5353 : Bonjour.

La règle de pare-feu porte sur le programme, ce qui couvre tous ces ports.
