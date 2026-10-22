"""Profil de configuration iOS (.mobileconfig) contenant la CA du PC.

Une fois le profil installé ET la confiance totale activée (Réglages > Général > Informations >
Réglages des certificats), Safari et les Raccourcis acceptent le HTTPS du PC.
Il faut vérifier l'empreinte SHA-256 affichée par iOS contre celle de `transfert empreinte` :
c'est ce qui garantit que personne n'a substitué sa propre CA pendant la livraison du profil.
"""

import plistlib
import uuid


def generer(der_ca: bytes, nom_pc: str, empreinte: str) -> bytes:
    uuid_cert = str(uuid.uuid5(uuid.NAMESPACE_URL, f"transfert-ca-{empreinte}")).upper()
    uuid_profil = str(uuid.uuid5(uuid.NAMESPACE_URL, f"transfert-profil-{empreinte}")).upper()
    profil = {
        "PayloadType": "Configuration",
        "PayloadVersion": 1,
        "PayloadIdentifier": f"local.transfert.{empreinte[:16]}",
        "PayloadUUID": uuid_profil,
        "PayloadDisplayName": f"Transfert - {nom_pc}",
        "PayloadDescription": "Autorité de certification locale de ce PC, pour le transfert de fichiers en HTTPS.",
        "PayloadContent": [{
            "PayloadType": "com.apple.security.root",
            "PayloadVersion": 1,
            "PayloadIdentifier": f"local.transfert.{empreinte[:16]}.ca",
            "PayloadUUID": uuid_cert,
            "PayloadDisplayName": f"Transfert CA - {nom_pc}",
            "PayloadContent": der_ca,
        }],
    }
    return plistlib.dumps(profil, fmt=plistlib.FMT_XML)
