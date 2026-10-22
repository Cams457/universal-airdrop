# Universal Airdrop

Un « AirDrop » pour **PC Windows et iPhone** (et Android) : envoie **photos, vidéos, PDF, liens et textes**
entre tes appareils, en un geste.

- **À la maison** : par ta box, **en partage de connexion**, ou en Bluetooth, sans passer par Internet.
- **Hors de chez toi** : avec **Tailscale** (gratuit), même quand l'iPhone est en 4G.
- **Chiffré** de bout en bout, et seuls tes appareils peuvent t'envoyer quelque chose.
- Un lien ou un texte envoyé arrive **directement dans le presse-papiers** de l'autre appareil.

<p align="center"><img src="assets/icone.png" width="96" alt=""></p>

## 1. Installer sur chaque PC (2 minutes)

<p align="center">
  <a href="https://github.com/Cams457/universal-airdrop/raw/main/telecharger/UniversalAirdrop.exe"><b>⬇ Télécharger UniversalAirdrop.exe</b></a>
  (Windows 10 et 11, 12 Mo, version 1.0.9)
</p>

| Ton PC | Lien direct |
|---|---|
| La plupart des PC (Intel, AMD) | [UniversalAirdrop.exe](https://github.com/Cams457/universal-airdrop/raw/main/telecharger/UniversalAirdrop.exe) |
| PC à processeur ARM (certains Surface, Snapdragon) | [UniversalAirdrop-arm64.exe](https://github.com/Cams457/universal-airdrop/raw/main/telecharger/UniversalAirdrop-arm64.exe) |

Dans le doute : Paramètres Windows › Système › Informations système › *Type du système* (« processeur x64 » :
premier lien ; « processeur ARM » : second). Empreintes SHA-256 : [telecharger/SHA256SUMS.txt](telecharger/SHA256SUMS.txt)
(vérification facultative, dans cmd : `certutil -hashfile UniversalAirdrop.exe SHA256`).

1. Clique sur le lien : le navigateur télécharge le fichier. Edge peut afficher « n'est pas téléchargé
   couramment » : **…** › **Conserver** › **Afficher plus** › **Conserver quand même**.
2. Range-le où tu veux (par exemple dans *Documents*) et double-clique dessus. Windows peut afficher
   *« Windows a protégé votre ordinateur »* : c'est normal pour un programme non signé. Clique sur
   **Informations complémentaires**, puis **Exécuter quand même**. Rien d'autre à installer.
3. Suis l'assistant : le nom du PC, puis **Autoriser** (Windows demande « Oui » : c'est le pare-feu).

**Mettre à jour** : télécharge la nouvelle version et lance-la ; elle remplace d'elle-même celle qui tourne
(les réglages et les appareils sont gardés). Pense à lancer toujours le même fichier si *Lancer au démarrage*
est coché.

L'icône <img src="assets/icone.png" width="16" alt=""> reste près de l'horloge ; un clic dessus rouvre
l'application. Le bouton **?** en haut de la fenêtre ouvre le **guide d'installation pas à pas**.

Deux PC : sur l'un, **Ajouter un PC** › *Attendre une demande ici* ; sur l'autre, **Ajouter un PC** ›
son adresse › *Se connecter* ; les deux écrans affichent un code à 6 chiffres : s'il est identique, accepte.

## 2. Ajouter ton iPhone (5 minutes, une seule fois)

1. Sur le PC : **Ajouter un iPhone**. Une adresse s'affiche, par exemple `https://pc-de-camil.local:47800/iphone`.
2. Sur l'iPhone, ouvre **Safari** et tape cette adresse (ou scanne le QR code proposé). La page guide :
   - **Télécharger le profil** › **Autoriser**, puis Réglages › **Profil téléchargé** › **Installer** ;
   - Réglages › Général › Informations › **Réglages des certificats** › active *Universal Airdrop CA*
     (vérifie que l'empreinte est celle affichée par le PC) ;
   - **Ouvrir l'app Universal Airdrop**, puis **Partager › Sur l'écran d'accueil**.
3. Ouvre l'icône **Airdrop** de l'écran d'accueil › **Relier cet iPhone** : le PC affiche un code ; s'il est
   identique à celui de l'iPhone, **Accepter**.

L'app de l'iPhone :

| Bouton | Ce qu'il fait |
|---|---|
| **Reçus** | Tout ce que tes PC t'ont envoyé (7 jours), nouveautés en tête. **Enregistrer…** ouvre le menu d'iOS : *Photos*, ou *Enregistrer dans Fichiers* pour **choisir le dossier**. Un élément déjà enregistré reste là pour l'enregistrer ailleurs, ou le supprimer. Liens et textes : **Copier** |
| **Textes et liens** | Les textes et liens reçus, **en entier**, avec **Copier** (et **Ouvrir** pour un lien) ; ils s'affichent tout seuls si l'écran est ouvert. En haut, un champ pour **envoyer un texte** au PC, qui arrive dans son presse-papiers (l'app confirme qu'il a été copié) |
| **Envoyer** | Photos, fichiers, liens ou textes vers **un ou plusieurs appareils** (bouton *Coller* pour un lien copié) |
| **Hors de chez toi** | Tutoriel Tailscale, et passage de l'app en mode « partout » |
| **Raccourcis** | Facultatif : ajoute **Envoyer** au bouton *Partager* de toutes les apps, avec ton adresse et ton jeton prêts à copier ([guide](docs/RACCOURCI_IOS.md)) |
| **Barre du bas** | Accueil, Reçus, Textes, Envoyer, À distance : toujours à portée de pouce (et « ‹ Accueil » en haut de chaque écran) |
| **?** (en haut) | L'aide : installer, recevoir et choisir où enregistrer, envoyer, Tailscale, Raccourcis, dépannage |

## 3. Utiliser

| Je veux… | Comment |
|---|---|
| Envoyer depuis le PC | Glisse les fichiers dans la fenêtre (ou clic droit › **Envoyer vers** › Universal Airdrop), **coche un ou plusieurs appareils**, **Envoyer** |
| Envoyer un lien ou un texte depuis le PC | **Ctrl+V** dans la fenêtre, coche, **Envoyer** : il arrive dans le **presse-papiers** de l'autre PC (Ctrl+V), avec une notification |
| Recevoir sur l'iPhone | Coche l'iPhone sur le PC ; sur l'iPhone, app **Airdrop** › **Reçus** › **Enregistrer…** (Photos, ou le dossier de ton choix) |
| Envoyer depuis l'iPhone | App **Airdrop** › **Envoyer** ; ou scanne le QR code du PC (**Afficher le QR code**) : un iPhone relié arrive directement sur **Envoyer** ; ou, avec le Raccourci, **Partager › Envoyer** depuis n'importe quelle app |
| Téléphone d'un ami, Android | Rien à installer : sur le PC, **Afficher le QR code** pour qu'il t'envoie (photos, fichiers, lien ou texte) ; « **Un téléphone, par QR code** » pour lui envoyer (fichiers, lien ou texte, avec bouton *Copier*) |
| Annuler un envoi | Bouton **Annuler** sous l'envoi en cours (l'autre PC efface le fichier partiel) ; pour un iPhone, **Annuler l'envoi en attente** dans *Appareils* (seulement ce qu'il n'a pas encore récupéré : « en attente » disparaît dès que l'app l'a reçu) |

Les fichiers reçus sur un PC arrivent dans **Téléchargements\Universal Airdrop**. Un lien reçu est copié et
peut s'ouvrir tout seul dans le navigateur (réglage *Ouvrir automatiquement les liens reçus*).

## 4. Hors de chez toi : Tailscale

[Tailscale](https://tailscale.com) relie tes appareils par un réseau privé chiffré (WireGuard), où qu'ils
soient, sans rien régler sur ta box et sans exposer ton PC à Internet. Gratuit pour un usage personnel.

1. Un compte sur tailscale.com (Google, Microsoft ou Apple), **le même** pour tous tes appareils.
2. **Tailscale pour Windows** sur chaque PC ; dans Universal Airdrop : **?** › Installer sur ce PC › **Autoriser** (une fois).
3. L'app **Tailscale** sur l'iPhone (App Store, gratuite), connectée, VPN autorisé et laissé actif.
4. Dans l'app **Airdrop** de l'iPhone : **Hors de chez toi** › **Ouvrir l'app par Tailscale**, ajoute-la à
   l'écran d'accueil et relie-la : elle marche alors à la maison comme ailleurs.

Les PC entre eux n'ont rien à faire : chacun retient l'adresse Tailscale des autres. Tutoriel complet :
[docs/TAILSCALE.md](docs/TAILSCALE.md).

## Sans box : partage de connexion

L'application détecte toute seule le nouveau réseau, en une dizaine de secondes.

- **L'iPhone partage sa connexion** (Réglages › Partage de connexion) et le PC s'y connecte. Pour la vitesse,
  désactive *Maximiser la compatibilité* (5 GHz).
- **Le PC partage sa connexion** (Paramètres Windows › Réseau et Internet › Point d'accès sans fil mobile).
- **En Bluetooth** : iPhone en partage de connexion, puis Paramètres Windows › Bluetooth › (l'iPhone) ›
  Réseau personnel (PAN) › Se connecter. Lent (1 à 2 Mbit/s) : pour des liens, des PDF ou des photos.

Un réseau d'école ou de café bloque souvent les échanges entre appareils : passe par un partage de connexion
ou par Tailscale.

## Sécurité, en bref

- Tous les échanges sont chiffrés (TLS). Chaque PC a son propre certificat, que tes autres appareils mémorisent.
- Un appareil inconnu ne peut rien envoyer : on l'ajoute en comparant un code à 6 chiffres sur les deux écrans.
- Rien n'est ouvert à Internet : seules les adresses du réseau local et de Tailscale sont acceptées.
- Détails : [docs/SECURITE.md](docs/SECURITE.md).

## Questions fréquentes

**« Client sent an HTTP request to an HTTPS server » ?** L'adresse a été tapée sans `https://` : l'application
redirige désormais toute seule ; sinon, ajoute `https://` au début.

**Safari : « le serveur ne répondait plus » ?** (aussi dans l'aide **?** de l'app) L'iPhone n'arrive pas jusqu'au PC :
1. Sur le PC, un **bandeau rouge** en haut de la fenêtre ? Clique sur **Autoriser**, puis « Oui » (pare-feu).
2. Même réseau ? Sur le PC, dans cmd : `ipconfig` ; sur l'iPhone : Réglages › Wi-Fi › (i) › Adresse IP. Les
   trois premiers nombres doivent être identiques (par exemple `192.168.1.x`).
3. Wi-Fi d'école, de résidence, d'hôtel ou « invité » : ils isolent souvent les appareils entre eux. Passe
   par le **partage de connexion** de l'iPhone, ou par **Tailscale**.
4. Un antivirus avec son propre pare-feu (Norton, Avast, Kaspersky…) peut aussi bloquer : autorise
   *UniversalAirdrop.exe* dans ses réglages.

**L'app redemande de relier l'iPhone ?** Normalement une seule fois par icône. Si iOS efface la mémoire de
l'app (navigation privée, données de sites effacées), le PC lui rend son lien tout seul à l'ouverture suivante.

**Le certificat n'apparaît pas dans « Réglages des certificats » ?** Il n'y apparaît qu'après **installation**
du profil (Réglages › Profil téléchargé, ou Général › VPN et gestion de l'appareil). iOS efface un profil
non installé au bout de 8 minutes : retélécharge-le.

**L'iPhone ne reçoit rien ?** Un iPhone ne peut pas recevoir de lui-même sans application de l'App Store :
ce que tu lui envoies l'attend sur le PC (7 jours). Ouvre l'app **Airdrop** › **Reçus**.

**Le lien n'est pas dans le presse-papiers ?** Envoyé depuis l'app de l'iPhone, l'écran **Textes** affiche
« Copié dans le presse-papiers de… » ou la raison de l'échec ; sur le PC, *Reçus récemment* indique la même chose.
Pour l'iPhone, un texte reçu du PC est dans **Textes** : touche **Copier**.

**L'icône ouvre « Envoyer vers… » sans menu ni retour ?** Elle a été ajoutée depuis la page du QR code
d'envoi, pas depuis l'app. Elle ouvre maintenant l'app toute seule (à relier une fois) ; dans Safari, le bouton
**‹ Accueil de l'app** en haut de cette page y mène aussi.

**Est-ce que ça passe par Internet ?** Non à la maison. Avec Tailscale hors de chez toi, oui, mais chiffré de
bout en bout entre tes appareils.

**Mac ou Linux ?** L'application fonctionne aussi (sans icône près de l'horloge), à compiler depuis les
sources : [docs/DEVELOPPEMENT.md](docs/DEVELOPPEMENT.md).

**Désinstaller.** Dans **Réglages**, décoche « Lancer au démarrage » et « Envoyer vers », puis **Quitter** ;
supprime `UniversalAirdrop.exe` et le dossier `%APPDATA%\UniversalAirdrop`. Sur l'iPhone : supprime l'icône
et le profil (Réglages › Général › VPN et gestion de l'appareil).

## Pour aller plus loin

| Document | Contenu |
|---|---|
| [docs/TAILSCALE.md](docs/TAILSCALE.md) | Fonctionner hors de chez toi : tutoriel complet |
| [docs/RACCOURCI_IOS.md](docs/RACCOURCI_IOS.md) | Les Raccourcis « Envoyer » et « Recevoir », leur lancement automatique |
| [docs/SECURITE.md](docs/SECURITE.md) | Modèle de menace, protocole d'appairage, défenses |
| [docs/VITESSE.md](docs/VITESSE.md) | Ce qui limite le débit, réseau par réseau |
| [docs/LANGAGE.md](docs/LANGAGE.md) | Pourquoi Go |
| [docs/DEVELOPPEMENT.md](docs/DEVELOPPEMENT.md) | Compiler, tester, architecture, routes |
