# Note de sécurité

## Modèle de menace
Un attaquant actif sur le même réseau (Wi-Fi public, réseau d'école, voisin sur un partage de connexion)
peut écouter, modifier, rejouer et s'interposer (MITM). Hors périmètre : un PC ou un téléphone déjà compromis.

## Défenses

| Menace | Défense | Code |
|---|---|---|
| Écoute | TLS 1.2 minimum sur toutes les routes réseau ; aucune route en clair | `identite`, `serveur/public.go` |
| Faux PC (MITM) | CA locale par PC, épinglée par les autres PC et installée sur l'iPhone ; un autre certificat est refusé | `identite.ConfigClientEpingle` |
| Premier contact PC ↔ PC | code à 6 chiffres avec engagement, comparé par les humains | `securite`, `client.Appairer` |
| Émetteur non autorisé | jeton de 256 bits par appareil, stocké haché (SHA-256), comparé en temps constant, révocable à chaud | `appareils` |
| Devinette de jeton | 20 échecs en 10 min depuis une même IP → refus (429) | `serveur.authentifier` |
| Fichier altéré | SHA-256 par bloc et empreinte racine sur l'ensemble | `blocs` |
| Coupure de courant pendant une réception | blocs revérifiés sur disque à la reprise | `blocs.reverifier` |
| Écriture hors du dossier de réception | noms nettoyés (`..`, chemins absolus, noms réservés Windows), aucun écrasement | `securite.NomSur`, `CheminLibre` |
| Lien piégé | seuls `http` et `https` sont ouverts, sans passer par un shell | `securite.URLAutorisee`, `systeme.OuvrirURL` |
| Lien de partage deviné | jeton de 128 bits, durée limitée, usage unique | `partage` |
| Exposition à Internet | refus de toute IP source non privée ; règle de pare-feu limitée à `localsubnet` | `serveur.filtreReseau`, `systeme.AjouterPareFeu` |
| Pilotage de l'application par un site web | interface sur 127.0.0.1 seulement, secret local exigé, en-tête `Host` vérifié (contre le *DNS rebinding*), aucun en-tête CORS | `serveur.filtreControle` |
| Injection de code dans les pages | CSP stricte (`script-src 'self'`, aucun script en ligne), `textContent` uniquement, gabarits échappés | `ui` |
| Injection de commande lors de la règle de pare-feu (exécutée en administrateur) | refus de tout chemin contenant `& | ^ < > " % !` | `systeme.AjouterPareFeu` |

## Protocole d'appairage

Notations :
- fp_A, fp_B : empreintes SHA-256 des CA des deux PC ;
- r_A, r_B : aléas de 256 bits ;
- H : SHA-256, avec un préfixe de longueur sur chaque champ (sans lui, H(ab ‖ c) = H(a ‖ bc)).

1. A → B, par TLS non encore authentifié : nom, CA_A. B tire r_B, puis répond CA_B et l'engagement
   c_B = H(fp_B ‖ fp_A ‖ r_B).
2. A épingle CA_B. Puis A → B, par TLS épinglé : r_A. B révèle r_B, et A vérifie c_B.
3. Les deux écrans affichent SAS = H(fp_A ‖ fp_B ‖ r_A ‖ r_B) mod 10^6.
4. Si les humains confirment l'égalité, chaque PC remet à l'autre un jeton pour l'envoi en sens inverse.

Pourquoi l'engagement : sans lui, un attaquant qui voit les valeurs de l'un pourrait choisir les siennes
hors ligne jusqu'à faire coïncider les deux codes (10^6 essais, quelques secondes). Avec lui, chaque partie
fixe son aléa avant de connaître celui de l'autre. Un MITM ne réussit alors qu'avec une probabilité de
10^-6 par tentative, chaque tentative est visible, et une fenêtre d'appairage en admet 3 au plus.

## Téléphones
- **iPhone avec Raccourci** : jeton propre à l'appareil, HTTPS vérifié grâce au profil installé.
- **Page d'envoi par QR code** (tout téléphone) : l'URL contient un jeton de 128 bits valable 15 minutes.
  Scanner le QR code est la preuve de présence ; sans profil installé, le téléphone affiche un
  avertissement de certificat, que l'utilisateur doit accepter.

## Limites connues
- Le jeton qu'un PC présente aux autres est stocké en clair dans `%APPDATA%\UniversalAirdrop`, comme un
  mot de passe enregistré. Extension possible : chiffrement DPAPI.
- Le profil iOS est livré par HTTPS non encore approuvé : la vraie protection est la vérification
  manuelle de l'empreinte.
- Les annonces de découverte et Bonjour ne sont pas authentifiées. Elles ne servent qu'à trouver une
  adresse : falsifiées, elles provoquent au pire un échec de connexion, jamais une fuite.
- L'exécutable n'est pas signé (SmartScreen avertit au premier lancement). Les versions publiées sont
  compilées par GitHub Actions depuis les sources du dépôt.
