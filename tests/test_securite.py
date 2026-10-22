"""Tests unitaires des primitives de sécurité et de l'identité."""

import datetime as dt
import plistlib
from pathlib import Path

from cryptography import x509

from transfert import identite as I
from transfert import securite as S
from transfert.profil_ios import generer


def test_nom_sur():
    assert S.nom_sur("../../windows/system32/evil.dll") == "evil.dll"
    assert S.nom_sur("C:\\Users\\x\\photo.jpg") == "photo.jpg"
    assert S.nom_sur("CON.txt") == "_CON.txt"
    assert S.nom_sur("a<b>c?.pdf") == "a_b_c_.pdf"
    assert S.nom_sur("...") == "fichier"
    assert S.nom_sur("") == "fichier"
    assert len(S.nom_sur("x" * 500 + ".mp4")) <= 200


def test_chemin_libre(tmp_path):
    (tmp_path / "a.txt").write_text("1")
    (tmp_path / "a (1).txt").write_text("2")
    assert S.chemin_libre(tmp_path, "a.txt").name == "a (2).txt"
    assert S.chemin_libre(tmp_path, "b.txt").name == "b.txt"


def test_ip_autorisee():
    for ip in ("192.168.1.10", "10.0.0.2", "172.20.10.3", "192.168.137.1", "127.0.0.1", "::1", "fe80::1"):
        assert S.ip_autorisee(ip), ip
    for ip in ("8.8.8.8", "1.1.1.1", "2001:4860::8888", "", None, "pas-une-ip"):
        assert not S.ip_autorisee(ip), ip


def test_url_autorisee():
    assert S.url_autorisee("https://exemple.fr/page?a=1")
    assert S.url_autorisee("http://192.168.1.5:8080/")
    for u in ("javascript:alert(1)", "file:///C:/Windows", "ms-settings:privacy", "https://", "",
              "https://a.fr/\x00", "data:text/html,<b>x</b>"):
        assert not S.url_autorisee(u), u


def test_jetons():
    j = S.nouveau_jeton()
    assert len(j) >= 43                   # 256 bits en base64url
    assert S.hacher_jeton(j) != j and len(S.hacher_jeton(j)) == 64


def test_engagement_et_code():
    fa, fb = "a" * 64, "b" * 64
    ra, rb, rb2 = S.nouvel_alea(), S.nouvel_alea(), S.nouvel_alea()
    c = S.engagement(fb, fa, rb)
    assert c == S.engagement(fb, fa, rb)
    assert c != S.engagement(fb, fa, rb2)                     # B ne peut pas changer r_B après coup
    code = S.code_verification(fa, fb, ra, rb)
    assert len(code) == 6 and code.isdigit()
    assert code != S.code_verification(fa, "c" * 64, ra, rb)  # une CA substituée change le code (quasi sûrement)


def test_prefixe_de_longueur():
    # sans préfixe de longueur, ("ab","c") et ("a","bc") donneraient le même hachage
    assert S._h(b"ab", b"c") != S._h(b"a", b"bc")


def test_identite_et_reemission(tmp_path, monkeypatch):
    ident = I.Identite(tmp_path, "PC-Test")
    assert len(ident.empreinte) == 64
    chaine = x509.load_pem_x509_certificates(ident.chemin_serveur.read_bytes())
    serveur, ca = chaine
    assert serveur.issuer == ca.subject
    duree = serveur.not_valid_after_utc - serveur.not_valid_before_utc
    assert duree <= dt.timedelta(days=825)                     # exigence iOS
    # nouvelle IP (partage de connexion) -> réémission, CA inchangée
    fp = ident.empreinte
    monkeypatch.setattr(I, "ips_locales", lambda: {"127.0.0.1", "172.20.10.4"})
    assert ident.verifier_certificat_serveur() is True
    san = x509.load_pem_x509_certificate(ident.chemin_serveur.read_bytes()).extensions \
        .get_extension_for_class(x509.SubjectAlternativeName).value
    assert "172.20.10.4" in {str(i) for i in san.get_values_for_type(x509.IPAddress)}
    assert I.Identite(tmp_path, "PC-Test").empreinte == fp
    assert ident.verifier_certificat_serveur() is False


def test_profil_ios(tmp_path):
    ident = I.Identite(tmp_path, "PC-Test")
    p = plistlib.loads(generer(ident.der_ca(), "PC-Test", ident.empreinte))
    assert p["PayloadType"] == "Configuration"
    assert p["PayloadContent"][0]["PayloadType"] == "com.apple.security.root"
    assert p["PayloadContent"][0]["PayloadContent"] == ident.der_ca()


def test_integration_windows(tmp_path, monkeypatch):
    from transfert import windows as W
    monkeypatch.setenv("APPDATA", str(tmp_path))
    crees = W.installer(Path("C:/Projets/universal-airdrop"), demarrage=True)
    assert len(crees) == 2 and all(p.exists() for p in crees)
    assert "envoyer-choisir %*" in crees[0].read_text()
    assert "-m transfert demon" in crees[1].read_text()
    assert len(W.desinstaller()) == 2 and not any(p.exists() for p in crees)
