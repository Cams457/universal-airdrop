# Universal Airdrop

Envoie **photos, vidéos, PDF et liens** entre tes PC Windows et ton téléphone (iPhone ou Android),
sans câble ni Internet : sur le même Wi-Fi, **en partage de connexion**, ou en Bluetooth.
Tout est chiffré, et seuls tes appareils peuvent t'envoyer quelque chose.

<p align="center"><img src="assets/icone.png" width="96" alt=""></p>

## Installer (2 minutes)

1. Télécharge **`UniversalAirdrop.exe`** depuis la [dernière version](https://github.com/Cams457/universal-airdrop/releases/latest).
   (PC à processeur ARM, comme certains Surface : prends `UniversalAirdrop-arm64.exe`.)
2. Double-clique dessus. Windows peut afficher *« Windows a protégé votre ordinateur »* : c'est normal pour
   un programme non signé (une signature coûte plusieurs centaines d'euros par an). Clique sur
   **Informations complémentaires**, puis **Exécuter quand même**.
3. Suis l'assistant : le nom de ton PC, puis **Autoriser** (Windows demande « Oui »).

C'est tout. L'icône <img src="assets/icone.png" width="16" alt=""> reste près de l'horloge ; un clic
dessus rouvre l'application.

## Utiliser

| Je veux… | Comment |
|---|---|
| Envoyer depuis le PC | Glisse les fichiers dans la fenêtre, puis choisis l'appareil. Ou : clic droit sur un fichier > **Envoyer vers** > **Universal Airdrop** |
| Envoyer vers un téléphone | Choisis « Un téléphone » : un QR code s'affiche, le téléphone le scanne et télécharge |
| Envoyer depuis un téléphone | Sur le PC : **Afficher le QR code**. Le téléphone le scanne, choisit ses photos, touche **Envoyer**. Marche avec tous les téléphones |
| Envoyer depuis le bouton Partager de l'iPhone | Ajoute ton iPhone une fois (**Ajouter un iPhone**, 5 minutes) : ensuite, dans n'importe quelle app, Partager → **Envoyer au PC** |
| Envoyer entre deux PC | Installe l'application sur les deux, puis **Ajouter un PC** et compare le code à 6 chiffres |

Les fichiers reçus arrivent dans **Téléchargements\Universal Airdrop** ; les liens s'ouvrent dans le navigateur.

## Sans box : partage de connexion

Aucune box ni Internet n'est nécessaire : l'application détecte toute seule le nouveau réseau, en une dizaine de secondes.

- **L'iPhone partage sa connexion** : Réglages > Partage de connexion > Autoriser d'autres utilisateurs.
  Le PC se connecte au Wi-Fi de l'iPhone. Pour plus de vitesse, désactive *Maximiser la compatibilité* (5 GHz).
- **Un Android partage sa connexion** : Paramètres > Point d'accès ; le PC s'y connecte.
- **Le PC partage sa connexion** : Paramètres Windows > Réseau et Internet > Point d'accès sans fil mobile ;
  le téléphone s'y connecte.
- **En Bluetooth** : avec l'iPhone en partage de connexion, Paramètres Windows > Bluetooth et appareils >
  (l'iPhone) > Réseau personnel (PAN) > Se connecter. C'est lent (1 à 2 Mbit/s) : idéal pour des liens,
  des PDF ou des photos, pas pour des vidéos.

Un réseau d'école ou de café bloque souvent les échanges entre appareils : passe alors par un partage de connexion.

## Sécurité, en bref

- Tous les échanges sont chiffrés (TLS). Chaque PC a son propre certificat, que tes autres appareils mémorisent.
- Un appareil inconnu ne peut rien envoyer : il faut l'ajouter, et entre deux PC, comparer un code à 6 chiffres.
- Rien n'est ouvert à Internet : seules les adresses du réseau local sont acceptées.
- Détails : [docs/SECURITE.md](docs/SECURITE.md).

## Questions fréquentes

**Le téléphone affiche un avertissement de sécurité.** C'est normal la première fois : le PC utilise son
propre certificat, qu'aucune autorité publique n'a signé. Sur iPhone, **Ajouter un iPhone** installe ce
certificat et l'avertissement disparaît. Sur Android, choisis « Paramètres avancés », puis « Continuer ».

**Est-ce que ça passe par Internet ?** Non. Les fichiers vont directement d'un appareil à l'autre.

**L'autre PC est « hors ligne ».** Vérifie qu'il est allumé, sur le même réseau, et que l'application y tourne.

**Mac ou Linux ?** L'application fonctionne aussi (sans icône près de l'horloge), à compiler depuis les
sources : voir [docs/DEVELOPPEMENT.md](docs/DEVELOPPEMENT.md).

**Désinstaller.**
1. Dans **Réglages**, décoche « Lancer au démarrage » et « Envoyer vers », puis **Quitter**.
2. Supprime `UniversalAirdrop.exe` et le dossier `%APPDATA%\UniversalAirdrop`.
3. La règle de pare-feu « Universal Airdrop » se retire dans Pare-feu Windows Defender > Paramètres avancés.

## Pour aller plus loin

| Document | Contenu |
|---|---|
| [docs/RACCOURCI_IOS.md](docs/RACCOURCI_IOS.md) | Le Raccourci iPhone « Envoyer au PC », et comment le partager à d'autres |
| [docs/VITESSE.md](docs/VITESSE.md) | Débits mesurés et ce qui limite la vitesse |
| [docs/LANGAGE.md](docs/LANGAGE.md) | Pourquoi Go |
| [docs/DEVELOPPEMENT.md](docs/DEVELOPPEMENT.md) | Compiler, tester, architecture, protocole |

La première version, écrite en Python, est conservée sur la branche
[`python-v0.1`](https://github.com/Cams457/universal-airdrop/tree/python-v0.1).
