# Choix du langage : Go

## Le besoin
- **N'importe qui doit pouvoir l'utiliser** : un seul fichier à télécharger, rien à installer, aucune
  ligne de commande.
- Un **service réseau exposé** au réseau local, qui analyse des données venues d'autres appareils : la
  sûreté mémoire est indispensable.
- HTTPS, création et épinglage de certificats X.509, multicast, Bonjour.
- Des **transferts parallèles** : plusieurs blocs et plusieurs fichiers en vol.
- Un code qu'un étudiant peut relire et faire évoluer.

## Comparaison

| Critère | Python | **Go** | Rust | C# / .NET |
|---|---|---|---|---|
| Un seul `.exe`, sans rien installer | non (Python requis, ou PyInstaller de 20 Mo et plus) | **oui, environ 11 Mo** | oui | oui, mais 60 Mo et plus en autonome |
| Bibliothèque standard | HTTP et TLS via des paquets | **HTTPS, TLS, X.509, JSON, multicast** | presque rien en standard | oui |
| Compilation pour Windows depuis Linux | — | **une commande** | possible, plus lourd | possible |
| Concurrence | asyncio, avec le GIL | **goroutines, vrai parallélisme** | async, exigeant | async |
| Sûreté mémoire | oui | oui | oui | oui |
| Lisibilité pour un étudiant | excellente | **bonne** | difficile | moyenne |

**Ce qui a été mesuré** ([détails](VITESSE.md)), à scénario identique : Go transfère 19 % plus vite et
consomme 12 à 25 % de processeur en moins que Python. Le gain est modeste parce que le travail lourd
(TLS, SHA-256) est déjà fait par du code natif dans les deux cas. **L'argument décisif est la
distribution** : un seul `.exe` que n'importe qui lance d'un double-clic.

**Rust** consommerait encore moins de processeur, mais cet avantage est invisible sur un Wi-Fi ou un
partage de connexion à 100-400 Mbit/s, et le code serait nettement plus difficile à maintenir.

**C# / .NET** s'intégrerait parfaitement à Windows, mais l'exécutable autonome est lourd, et la
compilation comme les tests depuis Linux sont moins directs.

**C / C++** est à écarter : analyser des données reçues du réseau sans sûreté mémoire est la source
classique des failles.

## Continuité
Les routes HTTPS utilisées par le Raccourci iPhone (`/api/rapide/fichier`, `/api/rapide/lien`) et le
protocole d'appairage (engagement, code à 6 chiffres) sont les mêmes que dans la version Python. Le guide
du Raccourci reste donc valable.
