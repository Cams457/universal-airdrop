# Raccourcis iPhone « Envoyer » et « Recevoir » (v2)

Deux Raccourcis, construits une seule fois, et plus aucun QR code ensuite :

| Raccourci | Ce qu'il fait |
|---|---|
| **Envoyer** | Dans n'importe quelle app : **Partager → Envoyer**, tu coches un ou plusieurs appareils (PC, autre iPhone, iPad), c'est parti. Un lien ou un texte arrive **dans le presse-papiers** de l'autre appareil. |
| **Recevoir** | Récupère ce que les PC t'ont envoyé : photos et vidéos dans **Photos**, autres fichiers dans **Fichiers**, liens et textes **copiés** (colle-les où tu veux). Il peut se lancer **tout seul** en arrivant sur ton Wi-Fi. |

**Pourquoi « Recevoir » et pas une réception instantanée ?** Sans application de l'App Store, iOS ne
laisse rien tourner en permanence sur l'iPhone : il ne peut pas « écouter » le PC. Le PC garde donc ce
qui t'est destiné dans une **boîte d'envoi** (7 jours), et « Recevoir » vient la vider. Une vraie
application demanderait un Mac et un compte développeur Apple (99 €/an).

Une limite d'iOS : la rangée d'icônes du haut de la feuille de partage (AirDrop, contacts) est réservée
aux applications. Un Raccourci apparaît dans la liste juste en dessous ; on peut le mettre en
**Favoris** pour qu'il soit en tête (étape 5).

## Prérequis : relier l'iPhone à un PC (une fois, sans QR code)

1. Sur le PC : **Ajouter un iPhone**. L'application affiche une adresse, par exemple
   `https://pc-de-camil.local:47800/iphone`.
2. Sur l'iPhone, ouvre **Safari** et tape cette adresse **avec `https://` au début** (sans lui, le PC
   redirige tout seul depuis la version 2.0.1), ou scanne le QR code proposé : c'est la même chose.
   Si Safari parle de connexion non privée : **Afficher les détails** > **visiter ce site web**.
3. La page guide pas à pas :
   1. **Télécharger le profil**, puis **Autoriser** ;
   2. Réglages > **Profil téléchargé** > **Installer** (sinon : Général > VPN et gestion de l'appareil) ;
   3. Réglages > Général > Informations > **Réglages des certificats** > active *Universal Airdrop CA* ;
      vérifie que l'empreinte SHA-256 est la même que sur le PC ;
   4. **Relier cet iPhone** : donne-lui un nom (par exemple `iPhone de Camil`). Le PC ouvre une fenêtre
      avec un code à 6 chiffres : s'il est identique à celui de l'iPhone, **Accepter**.
4. La page affiche alors l'**adresse du PC** et ton **jeton**. Copie-les : les deux Raccourcis les
   demandent. Le jeton ne s'affiche qu'une fois.

**Un seul PC suffit**, même si tu en as plusieurs : ce PC relaie vers les autres (et vers leurs
téléphones). Il doit simplement être allumé quand tu envoies ou reçois.

---

## Raccourci 1 : « Envoyer » (10 minutes)

App **Raccourcis** > **+**. Touche le nom en haut > **Renommer** > `Envoyer`. Icône : un avion en papier.

### Réglages du Raccourci
Touche **(i)** (ou le menu ⌄ près du nom) :
- active **Afficher dans la feuille de partage** ;
- **Types acceptés** : *Images, Médias, Fichiers, URL, Texte* ;
- « Si aucune entrée » : **Arrêter et répondre**.

### Les actions, dans l'ordre
Tape le nom de chaque action dans la barre de recherche en bas.

| # | Action | Réglage |
|---|---|---|
| 1 | **Texte** | colle l'adresse du PC, par exemple `https://pc-de-camil.local:47800` |
| 2 | **Définir la variable** | nom : `Adresse` ; valeur : *Texte* |
| 3 | **Texte** | colle ton jeton |
| 4 | **Définir la variable** | nom : `Jeton` ; valeur : *Texte* |
| 5 | **Obtenir le contenu de l'URL** | URL : `Adresse` puis `/api/rapide/destinations` · réglages **G** |
| 6 | **Obtenir la valeur du dictionnaire** | Obtenir **Valeur** pour la clé `destinations` dans *Contenu de l'URL* |
| 7 | **Choisir dans la liste** | liste : *Valeur du dictionnaire* ; invite : `Envoyer à…` ; active **Sélectionner plusieurs** |
| 8 | **Combiner le texte** | *Élément choisi*, avec le séparateur **Personnalisé** `\|` (barre verticale) |
| 9 | **Encoder l'URL** | *Texte combiné* (en anglais : *URL Encode*) |
| 10 | **Définir la variable** | nom : `Destinations` ; valeur : *Texte encodé* |
| 11 | **Répéter avec chaque élément** | dans : *Entrée du raccourci* |
| 12 | ↳ **Obtenir le type** | de : *Élément répété* |
| 13 | ↳ **Si** | *Type* **est** `URL` ; touche **Ajouter une condition** : *Type* **est** `Texte`, et choisis **N'importe laquelle** |
| 14 | ↳↳ **Obtenir le contenu de l'URL** | URL : `Adresse` puis `/api/rapide/lien` · réglages **L** |
| 15 | ↳ **Sinon** | |
| 16 | ↳↳ **Obtenir le contenu de l'URL** | URL : `Adresse` puis `/api/rapide/fichier` · réglages **F** |
| 17 | ↳ **Fin si** | |
| 18 | **Fin de la répétition** | |
| 19 | **Afficher une notification** | `Envoyé ✓` |

Pour coller une variable (`Adresse`, `Jeton`…) dans un champ : touche le champ, puis la variable dans
la barre au-dessus du clavier (ou **Sélectionner une variable**).

**Réglages G** (touche **Afficher plus** sur l'action) :
- Méthode : **GET**
- En-têtes : **Ajouter un nouvel en-tête** : clé `Authorization`, texte `Bearer ` (avec l'espace) suivi de la variable `Jeton`

**Réglages L (lien ou texte)** :
- Méthode : **POST**
- En-têtes : `Authorization` = `Bearer ` + `Jeton` ; `X-Destinations` = variable `Destinations`
- Corps de la requête : **JSON** ; ajoute un champ **Texte** de clé `texte` et de valeur *Élément répété*

**Réglages F (fichier)** :
- Méthode : **POST**
- En-têtes : `Authorization` = `Bearer ` + `Jeton` ; `X-Destinations` = variable `Destinations`
- Corps de la requête : **Formulaire** ; ajoute un champ **Fichier** de clé `fichier` et de valeur *Élément répété*

### Essai
Photos > une photo > **Partager** > **Envoyer** > coche ton PC > **OK**. La première fois, iOS demande
l'autorisation de contacter le PC : **Toujours autoriser**.

---

## Raccourci 2 : « Recevoir » (8 minutes)

**+** > renomme-le `Recevoir`. Il ne s'affiche pas dans la feuille de partage.

| # | Action | Réglage |
|---|---|---|
| 1 | **Texte** | l'adresse du PC (la même) |
| 2 | **Définir la variable** | `Adresse` |
| 3 | **Texte** | ton jeton |
| 4 | **Définir la variable** | `Jeton` |
| 5 | **Obtenir le contenu de l'URL** | URL : `Adresse` puis `/api/rapide/boite` · réglages **G** (ci-dessus) |
| 6 | **Définir la variable** | nom : `Boite` ; valeur : *Contenu de l'URL* |
| 7 | **Obtenir la valeur du dictionnaire** | **Valeur** pour `texte` dans `Boite` |
| 8 | **Si** | *Valeur du dictionnaire* **a une valeur** |
| 9 | ↳ **Copier dans le presse-papiers** | *Valeur du dictionnaire* |
| 10 | **Fin si** | |
| 11 | **Obtenir la valeur du dictionnaire** | **Valeur** pour `fichiers` dans `Boite` |
| 12 | **Répéter avec chaque élément** | dans : *Valeur du dictionnaire* |
| 13 | ↳ **Obtenir la valeur du dictionnaire** | **Valeur** pour `url` dans *Élément répété* |
| 14 | ↳ **Obtenir le contenu de l'URL** | URL : *Valeur du dictionnaire* · réglages **G** |
| 15 | ↳ **Définir la variable** | nom : `Fichier` ; valeur : *Contenu de l'URL* |
| 16 | ↳ **Obtenir la valeur du dictionnaire** | **Valeur** pour `destination` dans *Élément répété* |
| 17 | ↳ **Si** | *Valeur du dictionnaire* **est** `photos` |
| 18 | ↳↳ **Enregistrer dans l'album photo** | `Fichier` ; album : *Récents* |
| 19 | ↳ **Sinon** | |
| 20 | ↳↳ **Enregistrer le fichier** | `Fichier` ; désactive **Demander où enregistrer** ; dossier : `Universal Airdrop` |
| 21 | ↳ **Fin si** | |
| 22 | **Fin de la répétition** | |
| 23 | **Obtenir la valeur du dictionnaire** | **Valeur** pour `resume` dans `Boite` |
| 24 | **Afficher une notification** | *Valeur du dictionnaire* |

La notification dit par exemple « 2 fichier(s) et 1 lien(s) ou texte(s), copié(s) de PC de Camil », ou
« Rien à recevoir ». Chaque fichier reçu est retiré de la boîte d'envoi : il n'arrive qu'une fois.

### Le lancer sans y penser (au choix)

- **Automatiquement en arrivant à la maison** : Raccourcis > **Automatisation** > **+** > **Wi-Fi** >
  choisis ton réseau > **Se connecte** > **Exécuter immédiatement** (désactive *Me notifier*) >
  **Suivant** > *Recevoir*. Le partage de connexion n'est pas un réseau Wi-Fi pour l'iPhone : dans ce
  cas, utilise une des deux solutions ci-dessous.
- **D'un double tapotement au dos de l'iPhone** : Réglages > Accessibilité > Toucher > **Toucher le
  dos** > Toucher deux fois > *Recevoir*.
- **Depuis l'écran d'accueil** : appui long sur l'écran d'accueil > **+** > Raccourcis > *Recevoir*.
- iPhone 15 Pro et plus récents : le **bouton Action** peut lancer *Recevoir*.

---

## Partager les Raccourcis à d'autres (personne n'aura à les reconstruire)

1. Appui long sur *Envoyer* > **Détails** > **Configurer** > **Questions d'importation** : ajoute une
   question pour l'action 1 (« Adresse de ton PC ») et une pour l'action 3 (« Ton jeton »). Fais de même
   pour *Recevoir*.
2. Remplace ton adresse et ton jeton par du texte vide : chacun mettra les siens à l'installation.
3. Appui long > **Partager** > **Copier le lien iCloud**.
4. Colle le lien d'*Envoyer* dans l'application sur le PC (**Ce PC** > **Réglages** > *Lien iCloud du
   Raccourci*) : l'assistant **Ajouter un iPhone** le proposera.

## Déjà le Raccourci de la v1 (« Envoyer au PC ») ?
Il continue de marcher (sans en-tête `X-Destinations`, le PC garde les fichiers pour lui). Pour choisir
les appareils, construis *Envoyer* ci-dessus avec le même jeton et la même adresse.

## En partage de connexion
- **PC connecté au partage de l'iPhone** : rien à changer, l'adresse `….local` désigne le PC sur ce réseau aussi.
- **iPhone connecté au point d'accès du PC** : rien à changer non plus.
- Si l'adresse `….local` ne répond pas, utilise l'une des *adresses de secours* (carte **Ce PC** sur le PC).

## Dépannage

| Symptôme | Cause probable | Correction |
|---|---|---|
| « Connexion non sécurisée » | certificat non installé ou confiance non activée | refaire les étapes 1 à 3 de la page `…/iphone` |
| « Client sent an HTTP request to an HTTPS server » | adresse tapée sans `https://` (version 2.0.0) | taper `https://` au début, ou mettre à jour l'application |
| Erreur 401 | jeton mal copié ou iPhone retiré | refaire « Relier cet iPhone » et recoller le jeton |
| Erreur 429 | trop d'essais avec un mauvais jeton | attendre 10 minutes, corriger le jeton |
| « aucun destinataire connu de ce PC » | nom d'appareil changé depuis | relancer *Envoyer* : la liste est relue à chaque fois |
| Serveur introuvable | application arrêtée sur le PC, ou réseau différent | lancer l'application ; même Wi-Fi ou partage de connexion |
| Grosse vidéo qui échoue | limite de mémoire des Raccourcis (au-delà de 1 à 2 Go) | sur le PC : **Afficher le QR code** (page d'envoi dans Safari) |
| « Rien à recevoir » alors que le PC a envoyé | envoi fait vers un autre nom d'iPhone | sur le PC, vérifie le nom coché ; *Recevoir* ne lit que la boîte de ce jeton |
