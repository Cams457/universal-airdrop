# Raccourci iPhone « Envoyer au PC »

Objectif : dans n'importe quelle app (Photos, Fichiers, Safari…), **Partager → Envoyer au PC**, et c'est
envoyé, sans rien saisir.

Sans Raccourci, l'iPhone peut déjà envoyer : sur le PC, **Afficher le QR code**, puis scanne-le.
Le Raccourci ajoute simplement le confort du bouton Partager.

Une limite d'iOS : la rangée d'icônes du haut de la feuille de partage (AirDrop, contacts, apps) est
réservée aux applications installées. Un Raccourci apparaît dans la liste juste en dessous ; on peut
l'épingler en **Favoris** pour qu'il soit toujours en tête (étape 5).

## Prérequis : ajouter l'iPhone (une fois par PC)

Dans l'application sur le PC : **Ajouter un iPhone**, puis suis les trois étapes :
1. **Installer le certificat du PC.** Scanne le QR code, installe le profil, puis active la confiance
   totale dans Réglages > Général > Informations > Réglages des certificats.
   Vérifie que l'empreinte SHA-256 affichée par l'iPhone est identique à celle du PC : c'est ce qui
   garantit que personne n'a glissé son propre certificat.
2. **Récupérer le jeton.** Scanne le second QR code ; la page montre le **jeton** et l'**adresse** du PC
   (par exemple `https://pc-de-camil.local:47800`). Copie-les : la page ne s'affiche qu'une fois.
3. **Le Raccourci** : si quelqu'un l'a déjà partagé (lien iCloud), installe-le et colle le jeton et
   l'adresse quand il les demande. Sinon, construis-le une fois avec le guide ci-dessous.

## Construire le Raccourci (10 minutes)

Ouvre l'app **Raccourcis** > **+**.

### 1. Nom et icône
Touche le nom en haut > **Renommer** > `Envoyer au PC`. Choisis une icône (un ordinateur).

### 2. Accepter ce qu'on partage
Touche **(i)** ou le menu ⌄ près du nom > **Détails** :
- active **Afficher dans la feuille de partage** ;
- dans **Types acceptés**, garde : *Images, Médias, Fichiers, URL*.

Dans « Si aucune entrée », choisis **Arrêter et répondre**.

### 3. Les actions, dans l'ordre

| # | Action à ajouter (barre de recherche) | Réglage |
|---|---|---|
| 1 | **Texte** | colle ton jeton ; touche la variable produite > Renommer > `Jeton` |
| 2 | **Texte** | colle l'adresse du PC, par exemple `https://pc-de-camil.local:47800` ; renomme la variable `Adresse` |
| 3 | **Répéter avec chaque élément** | dans : *Entrée du raccourci* |
| 4 | ↳ **Obtenir le type** | de : *Élément répété* |
| 5 | ↳ **Si** | *Type* **est** `URL` |
| 6 | ↳↳ **Obtenir le contenu de l'URL** | URL : `Adresse` + `/api/rapide/lien` · réglages A |
| 7 | ↳ **Sinon** | |
| 8 | ↳↳ **Obtenir le contenu de l'URL** | URL : `Adresse` + `/api/rapide/fichier` · réglages B |
| 9 | ↳ **Fin si** | |
| 10 | **Fin de la répétition** | |
| 11 | **Afficher une notification** | `Envoyé au PC` |

**Réglages A (lien)**, dans « Afficher plus » de l'action :
- Méthode : **POST**
- En-têtes : `Authorization` = `Bearer ` suivi de la variable `Jeton` (avec un espace après Bearer)
- Corps de la requête : **JSON**, avec un champ texte de clé `url` et de valeur *Élément répété*

**Réglages B (fichier)** :
- Méthode : **POST**
- En-têtes : `Authorization` = `Bearer ` + `Jeton`
- Corps de la requête : **Formulaire**, avec un champ de type **Fichier**, de clé `fichier` et de valeur *Élément répété*

### 4. Premier essai
Photos > une photo > **Partager** > **Envoyer au PC**. iOS demande l'autorisation de contacter le PC :
**Toujours autoriser**. La photo arrive dans *Téléchargements\Universal Airdrop*.

### 5. Le mettre en tête de la feuille de partage
Feuille de partage > tout en bas **Modifier les actions…** > touche **+** devant *Envoyer au PC*.

## Partager le Raccourci à d'autres (pour que personne n'ait à le reconstruire)

1. Dans Raccourcis, appui long sur *Envoyer au PC* > **Détails** > **Configurer** > **Questions d'importation**.
   Ajoute une question pour l'action 1 (« Colle ton jeton ») et une pour l'action 2 (« Colle l'adresse de ton PC »).
2. Remplace ton jeton et ton adresse par du texte vide : chacun mettra les siens à l'installation.
3. Appui long > **Partager** > **Copier le lien iCloud**.
4. Colle ce lien dans l'application sur le PC (**Ce PC** > **Réglages** > *Lien iCloud du Raccourci*).
   L'assistant **Ajouter un iPhone** l'affichera alors en QR code : n'importe qui l'installe en 30 secondes.

## Deuxième PC
Duplique le raccourci (appui long > Dupliquer), renomme-le (par exemple « Envoyer au PC fixe ») et mets le
jeton et l'adresse donnés par **Ajouter un iPhone** sur ce deuxième PC.

## En partage de connexion
- **PC connecté au partage de l'iPhone** : rien à changer, l'adresse `….local` désigne le PC sur ce réseau aussi.
- **iPhone connecté au point d'accès du PC** : rien à changer non plus.
- Si l'adresse `….local` ne répond pas sur un réseau, utilise l'une des *adresses de secours* données avec le jeton.

## Dépannage

| Symptôme | Cause probable | Correction |
|---|---|---|
| « Connexion non sécurisée » | certificat non installé ou confiance non activée | refaire l'étape 1 de « Ajouter un iPhone » |
| Erreur 401 | jeton mal copié ou appareil retiré | refaire « Ajouter un iPhone » et recoller le jeton |
| Erreur 429 | trop d'essais avec un mauvais jeton | attendre 10 minutes, corriger le jeton |
| Serveur introuvable | application arrêtée, ou réseau différent | lancer l'application sur le PC ; vérifier le Wi-Fi |
| Grosse vidéo qui échoue | limite de mémoire des Raccourcis | utiliser **Afficher le QR code** sur le PC |
