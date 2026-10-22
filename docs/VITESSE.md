# Vitesse : ce qui limite, et ce que donne chaque réseau

## 1. Le logiciel n'est pas la limite

Mesures avec `UniversalAirdrop bench --local` :
- deux processus séparés sur le même PC, comme deux PC ;
- machine de test à 2 cœurs, partagés entre l'émetteur et le récepteur.

| Cas | Débit |
|---|---|
| Gros fichier, blocs de 4 Mio, 4 en vol | 1 900 à 3 000 Mbit/s |
| 40 photos de 3 Mo | 1 300 à 2 000 Mbit/s |
| Latence de démarrage | 13 à 95 ms |

Comparaison avec un premier prototype en Python, à scénario identique (1 Gio, blocs de 4 Mio, 4 en vol) :

| Version | Débit | Processeur de l'émetteur | Processeur du récepteur |
|---|---|---|---|
| Python | 2 535 Mbit/s | 2,41 s/Gio | 3,22 s/Gio |
| Go | 3 026 Mbit/s | 2,12 s/Gio | 2,40 s/Gio |

Le gain reste modeste, car dans les deux versions le travail lourd est fait par du code natif. Le
SHA-256 pèse à lui seul environ 0,8 s/Gio de chaque côté, même avec l'accélération matérielle.

## 2. C'est la radio qui fixe le plafond

Pour N octets envoyés à un débit utile D, la durée vaut :

```latex
t = \frac{8N}{D}
```

Débits utiles estimés, à vérifier chez toi avec iperf3 :

| Réseau | Sauts radio | Débit utile estimé | Vidéo de 500 Mo |
|---|---|---|---|
| PC en Ethernet, téléphone en Wi-Fi 5 vers la box | 1 | 200 à 400 Mbit/s | 10 à 20 s |
| Deux appareils en Wi-Fi 5 via la box | 2 | 100 à 250 Mbit/s | 16 à 40 s |
| Partage de connexion de l'iPhone en 5 GHz | 1 | 100 à 300 Mbit/s | 13 à 40 s |
| Partage de connexion en 2,4 GHz (« Maximiser la compatibilité ») | 1 | 20 à 60 Mbit/s | 1 à 3 min |
| Point d'accès mobile de Windows | 1 | selon la carte Wi-Fi du PC | — |
| Bluetooth (PAN du partage de l'iPhone) | 1 | 1 à 2 Mbit/s | 35 à 70 min |
| Tailscale à la maison (chemin direct sur le Wi-Fi) | 1 à 2 | proche du réseau local, un peu moins (chiffrement WireGuard) | 20 à 60 s |
| Tailscale à distance (4G/5G ↔ box) | Internet | 10 à 50 Mbit/s, limité par l'envoi montant le plus faible | 1,5 à 7 min |

Pourquoi deux sauts radio divisent environ le débit par deux : sur un même canal, la box et les deux
appareils se partagent le temps d'antenne. Chaque trame passe une fois de l'émetteur à la box, puis une
fois de la box au récepteur : elle occupe donc l'air deux fois.

En partage de connexion, le téléphone **est** le point d'accès. Le trajet téléphone ↔ PC ne fait plus
qu'un saut, mais la puce Wi-Fi d'un téléphone en point d'accès est souvent plus modeste que celle d'une box.

## 3. Ce qui aide vraiment, par ordre d'efficacité

1. **Le 5 GHz.** En partage de connexion iPhone, désactive « Maximiser la compatibilité ». Sur la box,
   connecte-toi au réseau 5 GHz.
2. **La proximité.** Le débit dépend fortement du rapport signal sur bruit.
3. **L'Ethernet** pour le PC qui reçoit le plus : un saut radio en moins.
4. Le **Bluetooth** seulement pour les liens, les PDF et les photos.

L'application choisit d'elle-même le chemin le plus rapide quand un PC est joignable par plusieurs
réseaux à la fois : Ethernet, puis Wi-Fi, puis Bluetooth.

## 4. Mesurer chez toi

```bat
UniversalAirdrop.exe bench --cible "PC fixe" --iperf 250
```

`--iperf` est le débit TCP brut mesuré par iperf3 sur le même lien. L'outil affiche alors
η = débit utile / débit iperf3, dont la cible est η ≥ 0,8.
