# Raccourci iOS « PC de Camil »

Objectif : dans n'importe quelle app (Photos, Fichiers, Safari…), **Partager → PC de Camil**, et c'est
envoyé, sans rien saisir. Le Raccourci se construit une seule fois, en 10 minutes environ.

Ce qui est possible et ce qui ne l'est pas : la rangée d'icônes tout en haut de la feuille de partage
(AirDrop, contacts, apps) est réservée aux applications installées. Un Raccourci apparaît dans la liste
d'actions juste en dessous ; on peut l'épingler en **Favoris** pour qu'il soit toujours en tête (étape 5).

## Prérequis (une fois par PC)

1. Sur le PC : `python -m transfert nommer "PC de Camil"`, puis lancer le démon : `python -m transfert demon`.
2. Dans une autre fenêtre cmd : `python -m transfert iphone-ajouter "iPhone de Camil"`.
   - Étape 1 affichée : scanne le QR code, installe le profil, puis active la confiance totale dans
     Réglages > Général > Informations > Réglages des certificats.
     **Vérifie que l'empreinte SHA-256 affichée par iOS est identique à celle du PC** : c'est ce qui
     garantit que personne n'a glissé son propre certificat.
   - Étape 2 affichée : scanne le second QR code. La page montre ton **jeton** et l'**adresse**
     (`https://pc-de-camil.local:47800`). Copie-les : la page ne s'affiche qu'une fois.

## Construction du Raccourci

Ouvre l'app **Raccourcis** > **+** (nouveau raccourci).

### 1. Nom et icône
Touche le nom en haut > **Renommer** > `PC de Camil`. Choisis une icône (un ordinateur).

### 2. Accepter ce qu'on partage
Touche **(i)** (ou le menu ⌄ près du nom) > **Détails** :
- active **Afficher dans la feuille de partage** ;
- dans **Types acceptés** (ou « Reçoit »), garde : *Images, Médias, Fichiers, URL* ; décoche le reste.

En haut du raccourci apparaît : *Reçoit Images, Médias, Fichiers, URL depuis la Feuille de partage*.
Dans « Si aucune entrée », choisis **Arrêter et répondre**.

### 3. Les actions, dans l'ordre

| # | Action à ajouter (barre de recherche) | Réglage |
|---|---|---|
| 1 | **Texte** | colle ton jeton ; touche la variable produite > Renommer > `Jeton` |
| 2 | **Texte** | `https://pc-de-camil.local:47800` ; renomme la variable `Adresse` |
| 3 | **Répéter avec chaque élément** | dans : *Entrée du raccourci* |
| 4 | ↳ **Obtenir le type** (ou « Obtenir les détails du contenu » > Type) | de : *Élément répété* |
| 5 | ↳ **Si** | *Type* **est** `URL` |
| 6 | ↳↳ **Obtenir le contenu de l'URL** | URL : `Adresse` + `/api/rapide/lien` · voir réglages A |
| 7 | ↳ **Sinon** | |
| 8 | ↳↳ **Obtenir le contenu de l'URL** | URL : `Adresse` + `/api/rapide/fichier` · voir réglages B |
| 9 | ↳ **Fin si** | |
| 10 | **Fin de la répétition** | |
| 11 | **Afficher une notification** | `Envoyé au PC de Camil` |

**Réglages A (lien)** — dans l'action « Obtenir le contenu de l'URL », touche « Afficher plus » :
- Méthode : **POST**
- En-têtes : ajoute `Authorization` = `Bearer ` suivi de la variable `Jeton` (un espace après Bearer)
- Corps de la requête : **JSON** ; ajoute un champ texte, clé `url`, valeur *Élément répété*

**Réglages B (fichier)** :
- Méthode : **POST**
- En-têtes : `Authorization` = `Bearer ` + `Jeton`
- Corps de la requête : **Formulaire** ; ajoute un champ de type **Fichier**, clé `fichier`,
  valeur *Élément répété*

### 4. Premier essai
Ouvre Photos > une photo > **Partager** > fais défiler > **PC de Camil**.
iOS demande l'autorisation de contacter `pc-de-camil.local` : **Toujours autoriser**.
La photo arrive dans `Téléchargements\Transfert` sur le PC, et le démon affiche une ligne « reçu de… ».

### 5. Le mettre en tête de la feuille de partage
Feuille de partage > tout en bas **Modifier les actions…** > touche **+** devant *PC de Camil* pour
l'ajouter aux **Favoris**. Il apparaît alors en premier dans la liste, à chaque partage.

## Deuxième PC
Duplique le raccourci (appui long > Dupliquer), renomme-le (par exemple « PC fixe de Camil »), et remplace
le jeton et l'adresse par ceux donnés par `iphone-ajouter` sur ce deuxième PC.

## En partage de connexion
- **PC connecté au partage de l'iPhone** : rien à changer, `pc-de-camil.local` se résout vers l'IP
  172.20.10.x du PC. Désactive *Maximiser la compatibilité* (Réglages > Partage de connexion) pour le 5 GHz.
- **iPhone connecté au point d'accès mobile du PC** : rien à changer non plus (192.168.137.1).

À vérifier chez toi (porte de la V1) : que le nom `.local` se résout bien dans les deux cas.

## En Bluetooth (sans Wi-Fi)
iOS ne transfère pas de fichiers en Bluetooth vers un PC, mais il **partage sa connexion** en Bluetooth.
Le PC rejoint ce réseau, et le Raccourci fonctionne tel quel.

1. Appaire une fois l'iPhone et le PC : Paramètres Windows > Bluetooth et appareils > Ajouter un appareil.
2. Sur l'iPhone : Réglages > Partage de connexion > active « Autoriser d'autres utilisateurs ».
3. Sur le PC : Paramètres > Bluetooth et appareils > (l'iPhone) > Réseau personnel (PAN) >
   **Se connecter**, en choisissant « Point d'accès ».
4. `python -m transfert adresses` doit afficher une ligne « Bluetooth ».
5. Partager → **PC de Camil**, comme d'habitude.

Débit : 1 à 2 Mbit/s environ. Une photo de 3 Mo prend une vingtaine de secondes, mais une vidéo de
500 Mo prendrait environ 45 min : garde le Bluetooth pour les liens, les PDF et les photos.

## Plan B si `pc-de-camil.local` ne répond pas
Remplace l'action 2 par :
1. **Obtenir les détails du réseau** > *Nom du réseau* (Wi-Fi) ;
2. **Si** *Nom du réseau* **est** `<nom de ta box>` → **Texte** `https://192.168.1.XX:47800` ;
   **Sinon** → **Texte** `https://172.20.10.XX:47800` ; renomme le résultat `Adresse`.

Les IP exactes sont données par `python -m transfert adresses` sur le PC, dans chaque situation.
Pour qu'elles restent stables, crée une réservation DHCP dans la box.

## Dépannage

| Symptôme | Cause probable | Correction |
|---|---|---|
| « Connexion non sécurisée » | profil non installé ou confiance non activée | refaire l'étape 1 de `iphone-ajouter` |
| Erreur 401 | jeton mal copié ou révoqué | relancer `iphone-ajouter` et recoller le jeton |
| Erreur 429 | trop d'essais avec un mauvais jeton | attendre 10 min, corriger le jeton |
| Le serveur est introuvable | démon arrêté, pare-feu, ou réseau différent | lancer le démon ; `python -m transfert pare-feu` |
| Grosse vidéo qui échoue | limite des Raccourcis (à mesurer en V1) | utiliser l'app LocalSend pour les fichiers lourds |
