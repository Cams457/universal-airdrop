"""Identité cryptographique du PC : une autorité de certification (CA) locale et un certificat serveur.

Pourquoi deux certificats :
- la CA est l'identité stable du PC. Son empreinte SHA-256 est ce que les autres PC épinglent
  et ce que l'iPhone installe dans un profil ;
- le certificat serveur, signé par la CA, porte les IP actuelles du PC (champ SAN). Il peut être
  réémis quand l'IP change sans casser la confiance, puisque la CA, elle, ne change pas.

iOS exige pour un certificat serveur : un SAN, l'usage serverAuth, et une validité d'au plus 825 jours.
"""

import datetime as dt
import hashlib
import ipaddress
import socket
import ssl
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

from .reseau import ips_locales

VALIDITE_CA = dt.timedelta(days=3650)
VALIDITE_SERVEUR = dt.timedelta(days=800)
MARGE_RENOUVELLEMENT = dt.timedelta(days=30)


def empreinte_der(der: bytes) -> str:
    return hashlib.sha256(der).hexdigest()


def empreinte_pem(pem: str) -> str:
    return empreinte_der(x509.load_pem_x509_certificate(pem.encode()).public_bytes(serialization.Encoding.DER))


def empreinte_lisible(fp: str) -> str:
    """Format affiché à l'humain, comme iOS : groupes de 2 caractères hexadécimaux."""
    return " ".join(fp[i:i + 2].upper() for i in range(0, len(fp), 2))


def _ecrire(chemin: Path, contenu: bytes, secret: bool = False) -> None:
    chemin.write_bytes(contenu)
    if secret:
        try:
            chemin.chmod(0o600)
        except OSError:
            pass


def _cle_pem(cle) -> bytes:
    return cle.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8,
                             serialization.NoEncryption())


class Identite:
    def __init__(self, dossier: Path, nom: str, noms_dns: tuple[str, ...] = ()):
        self.noms_dns = tuple(noms_dns)
        self.dossier = Path(dossier) / "identite"
        self.dossier.mkdir(parents=True, exist_ok=True)
        self.nom = nom
        self.chemin_ca = self.dossier / "ca.pem"
        self.chemin_cle_ca = self.dossier / "ca_cle.pem"
        self.chemin_serveur = self.dossier / "serveur_chaine.pem"
        self.chemin_cle_serveur = self.dossier / "serveur_cle.pem"
        if not self.chemin_ca.exists():
            self._creer_ca()
        self.ca_pem = self.chemin_ca.read_text()
        self.ca = x509.load_pem_x509_certificate(self.ca_pem.encode())
        self.empreinte = empreinte_der(self.ca.public_bytes(serialization.Encoding.DER))
        self.verifier_certificat_serveur()

    # --- création -------------------------------------------------------------

    def _creer_ca(self) -> None:
        cle = ec.generate_private_key(ec.SECP256R1())
        nom = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, f"Transfert CA - {self.nom}"[:64])])
        maintenant = dt.datetime.now(dt.timezone.utc)
        cert = (x509.CertificateBuilder()
                .subject_name(nom).issuer_name(nom)
                .public_key(cle.public_key())
                .serial_number(x509.random_serial_number())
                .not_valid_before(maintenant - dt.timedelta(minutes=5))
                .not_valid_after(maintenant + VALIDITE_CA)
                .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
                .add_extension(x509.KeyUsage(digital_signature=False, content_commitment=False,
                                             key_encipherment=False, data_encipherment=False,
                                             key_agreement=False, key_cert_sign=True, crl_sign=True,
                                             encipher_only=False, decipher_only=False), critical=True)
                .add_extension(x509.SubjectKeyIdentifier.from_public_key(cle.public_key()), critical=False)
                .sign(cle, hashes.SHA256()))
        _ecrire(self.chemin_cle_ca, _cle_pem(cle), secret=True)
        _ecrire(self.chemin_ca, cert.public_bytes(serialization.Encoding.PEM))

    def _creer_serveur(self, ips: set[str]) -> None:
        cle_ca = serialization.load_pem_private_key(self.chemin_cle_ca.read_bytes(), None)
        cle = ec.generate_private_key(ec.SECP256R1())
        hote = socket.gethostname()
        dns = dict.fromkeys([hote, f"{hote}.local", "localhost", *self.noms_dns])
        noms = [x509.DNSName(d) for d in dns]
        noms += [x509.IPAddress(ipaddress.ip_address(ip)) for ip in sorted(ips)]
        maintenant = dt.datetime.now(dt.timezone.utc)
        cert = (x509.CertificateBuilder()
                .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, self.nom[:64])]))
                .issuer_name(self.ca.subject)
                .public_key(cle.public_key())
                .serial_number(x509.random_serial_number())
                .not_valid_before(maintenant - dt.timedelta(minutes=5))
                .not_valid_after(maintenant + VALIDITE_SERVEUR)
                .add_extension(x509.SubjectAlternativeName(noms), critical=False)
                .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
                .add_extension(x509.KeyUsage(digital_signature=True, content_commitment=False,
                                             key_encipherment=False, data_encipherment=False,
                                             key_agreement=False, key_cert_sign=False, crl_sign=False,
                                             encipher_only=False, decipher_only=False), critical=True)
                .add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]), critical=False)
                .add_extension(x509.AuthorityKeyIdentifier.from_issuer_public_key(cle_ca.public_key()),
                               critical=False)
                .sign(cle_ca, hashes.SHA256()))
        _ecrire(self.chemin_cle_serveur, _cle_pem(cle), secret=True)
        chaine = cert.public_bytes(serialization.Encoding.PEM) + self.ca_pem.encode()
        _ecrire(self.chemin_serveur, chaine)

    def verifier_certificat_serveur(self) -> bool:
        """Réémet le certificat serveur s'il manque, expire bientôt ou ne couvre plus les IP actuelles.

        Renvoie True si un nouveau certificat a été émis.
        """
        ips = ips_locales()
        if self.chemin_serveur.exists():
            cert = x509.load_pem_x509_certificate(self.chemin_serveur.read_bytes())
            san = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
            ips_cert = {str(ip) for ip in san.get_values_for_type(x509.IPAddress)}
            dns_cert = set(san.get_values_for_type(x509.DNSName))
            encore_valide = cert.not_valid_after_utc - dt.datetime.now(dt.timezone.utc) > MARGE_RENOUVELLEMENT
            if encore_valide and ips <= ips_cert and set(self.noms_dns) <= dns_cert:
                return False
        self._creer_serveur(ips)
        return True

    # --- contextes TLS --------------------------------------------------------

    def contexte_serveur(self) -> ssl.SSLContext:
        ctx = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH)
        ctx.minimum_version = ssl.TLSVersion.TLSv1_2
        ctx.load_cert_chain(self.chemin_serveur, self.chemin_cle_serveur)
        return ctx

    def recharger(self, ctx: ssl.SSLContext) -> None:
        """Les nouvelles connexions utiliseront le certificat réémis ; les anciennes ne sont pas coupées."""
        ctx.load_cert_chain(self.chemin_serveur, self.chemin_cle_serveur)

    def der_ca(self) -> bytes:
        return self.ca.public_bytes(serialization.Encoding.DER)


def contexte_client_epingle(ca_pem_pair: str) -> ssl.SSLContext:
    """Client qui n'accepte QUE la CA du pair : c'est l'épinglage.

    check_hostname est désactivé car on vérifie mieux qu'un nom : la chaîne doit remonter
    exactement à cette CA, et les IP du pair peuvent changer.
    """
    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH, cadata=ca_pem_pair)
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_REQUIRED
    ctx.minimum_version = ssl.TLSVersion.TLSv1_2
    return ctx


def contexte_client_premier_contact() -> ssl.SSLContext:
    """Uniquement pour le tout premier message d'appairage : chiffré mais non authentifié.

    L'authentification vient ensuite du code de vérification comparé par les humains.
    """
    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    ctx.minimum_version = ssl.TLSVersion.TLSv1_2
    return ctx
