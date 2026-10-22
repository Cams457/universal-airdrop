# Note de sécurité

## Modèle de menace
Un attaquant actif sur le même réseau (Wi-Fi public, réseau d'école, voisin sur le partage de connexion)
peut écouter, modifier, rejouer et s'interposer (MITM). Hors périmètre : un PC ou un iPhone déjà compromis.

## Défenses

| Menace | Défense | Où |
|---|---|---|
| Écoute | TLS 1.2+ sur toutes les routes réseau, aucune route en clair | `identite.py`, `serveur.py` |
| Faux PC (MITM) | CA locale par PC, épinglée par les autres PC (`contexte_client_epingle`) et installée sur l'iPhone | `identite.py` |
| Premier contact PC ↔ PC | code à 6 chiffres avec engagement, comparé par les humains | `securite.py`, `client.py` |
| Émetteur non autorisé | jeton de 256 bits par appareil, stocké haché, révocable à chaud | `appareils.py` |
| Devinette de jeton | 20 échecs en 10 min par IP → 429 | `serveur.py` |
| Fichier altéré | SHA-256 par bloc + racine sur l'ensemble | `blocs.py` |
| Écriture hors du dossier | noms nettoyés (`..`, chemins absolus, noms réservés Windows), aucun écrasement | `securite.py` |
| Lien piégé | seuls `http`/`https` sont ouverts | `securite.py` |
| Lien de partage deviné | jeton de 128 bits, 10 min, usage unique | `partage.py` |
| Exposition à Internet | refus de toute IP source non privée ; pare-feu limité à `localsubnet` | `serveur.py`, `pare-feu` |
| Pilotage du démon à distance | API de contrôle sur 127.0.0.1 uniquement, avec secret local | `serveur.py` |
| Page de partage détournée | CSP `default-src 'none'`, `X-Frame-Options: DENY`, échappement HTML | `serveur.py` |

## Protocole d'appairage
Notations : fp_A, fp_B = empreintes SHA-256 des CA ; r_A, r_B = aléas de 256 bits ; H = SHA-256 avec
préfixe de longueur sur chaque champ (sans ce préfixe, H(ab ‖ c) = H(a ‖ bc)).

1. A → B (TLS non authentifié) : nom, CA_A. B tire r_B et répond CA_B et l'engagement c_B = H(fp_B ‖ fp_A ‖ r_B).
2. A épingle CA_B ; A → B (TLS épinglé) : r_A. B révèle r_B. A vérifie c_B.
3. Les deux écrans affichent SAS = H(fp_A ‖ fp_B ‖ r_A ‖ r_B) mod 10^6.
4. Si les humains confirment l'égalité, chaque PC remet à l'autre un jeton pour l'envoi en sens inverse.

Pourquoi l'engagement : sans lui, un attaquant qui voit les valeurs de l'un pourrait choisir les siennes
hors ligne jusqu'à faire coïncider les deux codes (10^6 essais, quelques secondes). Avec lui, chaque
partie fixe son aléa avant de connaître celui de l'autre ; un MITM réussit avec une probabilité 10^-6 par
tentative, et chaque tentative est visible (3 au plus par fenêtre d'appairage).

## Limites connues
- Le jeton sortant (celui que ce PC présente aux autres) est stocké en clair dans `%APPDATA%\Transfert`,
  comme un mot de passe enregistré. Extension possible : chiffrement DPAPI.
- La livraison du profil iOS se fait en HTTPS non encore approuvé : la vraie protection est la
  vérification manuelle de l'empreinte affichée par iOS.
- Les annonces de découverte et Bonjour ne sont pas authentifiées. Elles ne servent qu'à trouver une
  adresse ; une annonce falsifiée provoque au pire un échec de connexion, jamais une fuite, grâce à
  l'épinglage TLS.
- La V0 (`v0/`) est en clair : outil d'apprentissage, à n'utiliser que chez soi.
