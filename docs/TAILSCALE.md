# Hors de chez toi : Tailscale

Sans rien de plus, Universal Airdrop fonctionne quand tes appareils sont sur le **même réseau** (box,
partage de connexion, point d'accès du PC). Pour qu'il marche **partout** (iPhone en 4G, PC au bureau,
autre Wi-Fi), on utilise [Tailscale](https://tailscale.com).

## Ce que c'est, et pourquoi lui

Tailscale crée un **réseau privé** entre tes appareils, chiffré par WireGuard. Chaque appareil y reçoit une
adresse fixe (dans `100.64.0.0/10`) et un nom du type `pc-de-camil.tail1234.ts.net`.

- **Gratuit** pour un usage personnel ; aucun serveur à louer, rien à régler sur ta box.
- **Ton PC n'est pas exposé à Internet** : seuls tes appareils connectés à ton compte Tailscale le voient.
- **Chiffré de bout en bout** entre tes appareils (et Universal Airdrop ajoute son propre TLS par-dessus).
- **À la maison**, Tailscale passe directement par ton Wi-Fi : la vitesse reste celle du réseau local.
  **Ailleurs**, il relie les appareils en direct quand c'est possible, sinon par un relais chiffré ; la
  vitesse dépend alors des deux connexions Internet (souvent 10 à 50 Mbit/s en 4G/5G).

Les serveurs de coordination de Tailscale voient quels appareils se connectent, jamais le contenu échangé.

## Tutoriel (10 minutes)

### 1. Le compte
Crée un compte sur [tailscale.com](https://tailscale.com) avec Google, Microsoft ou Apple. Utilise **le même
compte** sur tous tes appareils.

### 2. Chaque PC
1. Télécharge **Tailscale pour Windows** depuis [tailscale.com/download](https://tailscale.com/download),
   installe-le et connecte-toi. Son icône apparaît près de l'horloge.
2. Dans Universal Airdrop : bouton **?** › *Installer sur ce PC* › **Autoriser** (une fois) : la règle du
   pare-feu s'ouvre alors aussi aux adresses Tailscale.
3. Après une minute, la carte **Ce PC** affiche **À distance (Tailscale) : https://….ts.net:47800**.
   Le certificat du PC couvre automatiquement ce nom.

### 3. L'iPhone
1. Installe l'app **Tailscale** depuis l'App Store (gratuite) et connecte-toi avec le même compte.
2. Autorise l'**ajout de la configuration VPN**. Laisse Tailscale **activé** : il consomme très peu.
3. Ouvre l'app **Airdrop** › **Hors de chez toi**. Elle indique que le PC est prêt.
4. Touche **Ouvrir l'app par Tailscale**, puis, dans Safari : **Partager** › **Sur l'écran d'accueil** ›
   **Ajouter**. Ouvre la nouvelle icône et **Relie cet iPhone** (code à comparer sur le PC).
   L'ancienne icône peut être supprimée.

Pourquoi une nouvelle icône ? Une app web appartient à l'adresse qui l'a ouverte. L'adresse en `.local`
n'existe que sur ton réseau local ; l'adresse en `.ts.net` fonctionne partout, y compris à la maison.

### 4. Les Raccourcis (si tu les utilises)
Dans *Envoyer* et *Recevoir*, remplace l'adresse en `.local` par l'adresse en `.ts.net` (l'app Airdrop
l'affiche dans **Raccourcis**, avec un bouton Copier).

### 5. Entre PC
Rien à faire : chaque PC retient les adresses (dont Tailscale) des PC appairés et les essaie quand le réseau
local ne répond pas. Les essais partent presque en même temps : le PC le plus proche répond le premier.

## Limites

- Les **QR codes** sont faits pour être scannés sur place : à distance, utilise l'app ou les Raccourcis.
- Le PC destinataire doit être **allumé**, avec Universal Airdrop et Tailscale lancés.
- Sur un réseau qui bloque les VPN (certaines entreprises ou écoles), Tailscale peut ne pas se connecter.

## Dépannage

| Symptôme | Correction |
|---|---|
| L'app dit « Tailscale n'est pas encore actif sur le PC » | Vérifie l'icône Tailscale du PC (connecté ?), attends une minute, rouvre la section |
| « PC injoignable » en 4G | Tailscale est-il activé sur l'iPhone ? L'app a-t-elle été ouverte par l'adresse `.ts.net` ? |
| Connexion refusée seulement à distance | Sur le PC : **?** › Installer sur ce PC › **Autoriser** (règle de pare-feu à mettre à jour) |
| « Connexion non sécurisée » sur l'adresse `.ts.net` | Attends une minute (le PC renouvelle son certificat avec ce nom), puis recharge |
