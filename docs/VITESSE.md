# Vitesse : où est la limite, et comment la repousser

## 1. Le logiciel n'est plus la limite

Mesures avec `python bench.py --local` :
- émetteur et récepteur dans deux processus séparés, comme deux PC ;
- machine de test à 2 cœurs partagés entre les deux bouts.

| Cas | Avant optimisation | Après |
|---|---|---|
| 40 photos de 3 Mo | 589 Mbit/s | 1 956 à 2 223 Mbit/s |
| Gros fichier, 4 blocs en vol | environ 2 475 Mbit/s | 2 259 à 2 863 Mbit/s |
| Latence de démarrage | — | 13 à 70 ms |

Ce qui a changé :
1. **Plusieurs fichiers en vol** (4 à la fois). Les allers-retours `preparer` / `terminer` d'une photo
   se recouvrent avec l'envoi des autres.
2. **Appel au démon local non bloquant.** Un appel synchrone figeait la boucle d'événements.
3. **Récepteur.** Le fichier reste ouvert pendant toute la réception, au lieu d'être rouvert à chaque
   bloc. L'état de reprise est sauvegardé au plus 2 fois par seconde, au lieu de l'être à chaque bloc.
   À la reprise, chaque bloc noté est revérifié sur le disque, ce qui évite un fsync coûteux sans
   rien perdre en sûreté.
4. **Choix du chemin.** Chaque annonce porte son type de lien et son débit ; l'émetteur essaie
   d'abord le chemin le plus rapide : Ethernet, puis Wi-Fi, puis Bluetooth.

Coûts des briques sur la machine de test (1 cœur) :

| Brique | Débit |
|---|---|
| SHA-256 (accéléré par SHA-NI) | environ 10 700 Mbit/s |
| AES-128-GCM (le chiffrement de TLS 1.3) | environ 44 000 Mbit/s |
| TLS par asyncio, en boucle locale | environ 5 000 Mbit/s |

Le plafond logiciel, d'environ 2 à 3 Gbit/s, dépasse le débit utile de tout lien Wi-Fi réaliste.

## 2. La radio fixe le plafond : pourquoi le Wi-Fi 6 aide

Débit physique d'un flux spatial en Wi-Fi 6 (802.11ax), canal de 80 MHz :

```latex
D_{\text{PHY}} = \frac{N_{\text{sc}} \, b \, R}{T_{\text{sym}}}
= \frac{980 \times 10 \times \tfrac{5}{6}}{13{,}6\ \mu\text{s}} \approx 600\ \text{Mbit/s}
```

Les termes de la formule :
- N_sc = 980 sous-porteuses de données en HE80 ;
- b = 10 bits par sous-porteuse (modulation 1024-QAM, MCS 11) ;
- R = 5/6 (rendement du code correcteur) ;
- T_sym = 12,8 µs de symbole + 0,8 µs d'intervalle de garde.

Avec 2 flux (MIMO 2×2, cas des iPhone et des cartes AX200/AX210), on obtient environ 1 200 Mbit/s.
En 160 MHz, le nombre de sous-porteuses double : environ 2 400 Mbit/s.

Du débit physique au débit TCP utile, il faut compter les en-têtes, l'accès au canal, les
acquittements et les retransmissions. On garde typiquement 50 à 70 % du débit physique, et seulement
à courte distance : le MCS 11 exige un excellent rapport signal sur bruit.

| Situation (Wi-Fi 6, 2×2, 80 MHz) | Sauts radio | Débit utile estimé |
|---|---|---|
| PC en Ethernet, iPhone en Wi-Fi 6 vers la box | 1 | 600 à 800 Mbit/s |
| Les deux en Wi-Fi 6 via la box, même canal | 2 | 300 à 400 Mbit/s |
| PC en Ethernet des deux côtés (PC ↔ PC) | 0 | environ 940 Mbit/s (gigabit) |
| Wi-Fi 5 (802.11ac), 2×2, 80 MHz, via la box | 2 | 150 à 300 Mbit/s |

Pourquoi deux sauts divisent le débit par deux : sur un même canal, la box et les deux appareils se
partagent le temps d'antenne. Chaque trame passe une fois de l'émetteur à la box, puis une fois de la
box au récepteur : elle occupe donc l'air deux fois.

Ces chiffres sont des estimations ; seule une mesure avec iperf3, puis `bench.py --iperf`, les confirme.

## 3. Ce qu'il faut pour le Wi-Fi 6

- **Box** Wi-Fi 6 (ou 6E / 7), réseau 5 GHz séparé du 2,4 GHz, canal de 80 MHz au minimum
  (160 MHz si la box le permet et que l'environnement n'est pas saturé).
- **PC** : une carte Wi-Fi 6, par exemple une Intel AX200 ou AX210 en M.2, ou une clé USB Wi-Fi 6.
  `netsh wlan show interfaces` doit indiquer « 802.11ax » et un débit de réception supérieur à 1 000 Mbit/s.
- **iPhone** : iPhone 11 ou plus récent pour le Wi-Fi 6. Les modèles récents vont jusqu'au Wi-Fi 6E
  ou 7 : à vérifier dans la fiche technique de ton modèle.

## 4. Par ordre d'efficacité

1. **Brancher en Ethernet** le PC qui reçoit le plus. Un saut radio disparaît, et le débit double.
2. **Le 5 GHz, à courte distance** de la box. Le débit dépend fortement du rapport signal sur bruit.
3. **Wi-Fi 6 avec un canal de 160 MHz**, si la box le permet.
4. **Le partage de connexion** quand la box est loin. L'iPhone ↔ PC passe alors en 1 saut, mais la
   puce d'un téléphone en point d'accès est souvent plus modeste que celle d'une box.
5. Le Bluetooth **seulement pour les petits fichiers** : 1 à 2 Mbit/s.

## 5. Limites et pistes

- **Agréger plusieurs chemins à la fois**, par exemple Ethernet et Wi-Fi ensemble. C'est possible,
  car les blocs sont indépendants, mais le gain n'existe que si les deux chemins ne partagent pas le
  même goulot. Ce n'est pas implémenté.
- **La compression** est inutile pour les photos et vidéos de l'iPhone, déjà compressées en HEIC et
  HEVC. Elle n'aiderait que pour des fichiers texte volumineux.
- **Au-delà de 2,5 Gbit/s** (Ethernet 10 Gbit/s, Wi-Fi 7 multi-lien), le cœur Python deviendrait la
  limite : voir la question du langage dans le README.
