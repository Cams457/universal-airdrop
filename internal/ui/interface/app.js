// Interface locale de Universal Airdrop. Aucune dépendance ; tout texte venant des données est inséré
// avec textContent (jamais innerHTML), pour qu'un nom de fichier piégé ne puisse pas injecter de code.
(function () {
  "use strict";

  // ---------- secret local (transmis dans le fragment #k=…, jamais envoyé au serveur) ----------
  var m = location.hash.match(/k=([A-Za-z0-9_-]+)/);
  if (m) { sessionStorage.setItem("ua-k", m[1]); history.replaceState(null, "", location.pathname + location.search); }
  var SECRET = sessionStorage.getItem("ua-k");
  var GUIDE = "https://github.com/Cams457/universal-airdrop/blob/main/docs/RACCOURCI_IOS.md";

  function $(id) { return document.getElementById(id); }

  function el(tag, attrs) {
    var e = document.createElement(tag);
    if (attrs) for (var k in attrs) {
      if (k === "texte") e.textContent = attrs[k];
      else if (k === "classe") e.className = attrs[k];
      else if (k.slice(0, 2) === "on") e.addEventListener(k.slice(2), attrs[k]);
      else e.setAttribute(k, attrs[k]);
    }
    for (var i = 2; i < arguments.length; i++) {
      var c = arguments[i];
      if (c == null) continue;
      e.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
    }
    return e;
  }

  function vider(e) { while (e.firstChild) e.removeChild(e.firstChild); return e; }

  function api(chemin, corps, methode) {
    var o = { method: methode || (corps === undefined ? "GET" : "POST"), headers: { "X-Controle": SECRET } };
    if (corps !== undefined && !(corps instanceof FormData)) { o.headers["Content-Type"] = "application/json"; o.body = JSON.stringify(corps); }
    else if (corps instanceof FormData) o.body = corps;
    return fetch(chemin, o).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (!r.ok) throw new Error(j.erreur || ("Erreur " + r.status));
        return j;
      });
    });
  }

  function taille(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1).replace(".", ",") + " Go";
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(".", ",") + " Mo";
    return Math.max(1, Math.round(n / 1e3)) + " Ko";
  }

  function heure(iso) { var d = new Date(iso); return d.toLocaleTimeString("fr-FR", { hour: "2-digit", minute: "2-digit" }); }

  // ---------- fenêtre modale ----------
  var fenetre = $("fenetre");
  var surFermeture = null;
  fenetre.addEventListener("close", function () { if (surFermeture) { var f = surFermeture; surFermeture = null; f(); } });
  function ouvrir(contenu, fermeture) {
    var c = vider($("contenu-fenetre"));
    c.appendChild(contenu);
    surFermeture = fermeture || null;
    if (!fenetre.open) fenetre.showModal();
  }
  function fermer() { if (fenetre.open) fenetre.close(); }

  function blocQR(liens, legende) {
    var b = el("div");
    if (!liens || !liens.length) { b.appendChild(el("p", { classe: "erreur", texte: "Ce PC n'est connecté à aucun réseau." })); return b; }
    b.appendChild(el("img", { classe: "qr", src: liens[0].qr, alt: "QR code" }));
    b.appendChild(el("p", { classe: "url", texte: liens[0].url }));
    if (legende) b.appendChild(el("p", { classe: "doux", texte: legende }));
    if (liens.length > 1) {
      var d = el("details", null, el("summary", { texte: "Le téléphone est sur un autre réseau ?" }));
      liens.slice(1).forEach(function (l) { d.appendChild(el("p", { classe: "url", texte: l.url + "  (" + l.reseau + ")" })); });
      b.appendChild(d);
    }
    return b;
  }

  // ---------- état ----------
  var etat = null, premierRendu = true;

  function rafraichir() {
    return api("/controle/etat").then(function (e) {
      etat = e;
      rendre();
      if (premierRendu) { premierRendu = false; apresPremierRendu(); }
    }).catch(function () {
      var s = $("statut"); s.className = "statut"; s.textContent = "Application arrêtée — relance Universal Airdrop";
    });
  }

  function pcs() { return (etat.appareils || []).filter(function (a) { return a.type === "pc"; }); }
  function enLigne(a) { return !!(etat.vus && etat.vus[a.empreinte]); }

  function rendre() {
    var s = $("statut");
    s.className = "statut ok";
    var res = etat.adresses && etat.adresses.length ? etat.adresses[0].reseau : "aucun réseau";
    s.textContent = etat.nom + " · " + res;

    // appareils
    var l = vider($("appareils"));
    if (!etat.appareils || !etat.appareils.length) l.appendChild(el("p", { classe: "doux", texte: "Aucun appareil pour l'instant. Ajoute ton iPhone ou un autre PC." }));
    (etat.appareils || []).forEach(function (a) {
      var etiquette = a.type === "pc" ? (enLigne(a) ? "PC · en ligne" : "PC · hors ligne") : "téléphone";
      l.appendChild(el("div", { classe: "ligne" },
        el("span", { classe: "nom" }, el("span", { classe: "point" + (a.type === "pc" && enLigne(a) ? " en-ligne" : "") }), a.nom),
        el("span", { classe: "rangee" }, el("span", { classe: "doux", texte: etiquette }),
          el("button", { classe: "lien-discret danger", texte: "Retirer", onclick: function () { retirer(a.nom); } }))));
    });

    // événements
    var ev = (etat.evenements || []).filter(function (e) { return e.type === "recu" || e.type === "lien" || e.type === "envoye" || e.type === "erreur"; }).reverse();
    var le = vider($("evenements"));
    if (!ev.length) le.appendChild(el("p", { classe: "doux", texte: "Rien pour l'instant." }));
    ev.slice(0, 12).forEach(function (e) {
      var ligne = el("div", { classe: "ligne" + (e.chemin ? " cliquable" : "") },
        el("span", { classe: "nom" + (e.type === "erreur" ? " erreur" : ""), texte: e.texte }), el("span", { classe: "doux", texte: heure(e.heure) }));
      if (e.chemin) ligne.addEventListener("click", function () { api("/controle/ouvrir", { chemin: e.chemin }); });
      le.appendChild(ligne);
    });

    // envois en cours
    var lv = vider($("envois"));
    (etat.envois || []).sort(function (a, b) { return a.debut < b.debut ? 1 : -1; }).slice(0, 4).forEach(function (e) {
      var pct = e.total ? Math.round(100 * e.fait / e.total) : 100;
      var t = e.erreur ? "Échec : " + e.erreur : e.fini ? "Envoyé ✓ · " + Math.round(e.mbits) + " Mbit/s" : pct + " % · " + taille(e.fait) + " sur " + taille(e.total);
      var barre = el("div", { classe: "barre" }, el("div"));
      barre.firstChild.style.width = (e.erreur ? 100 : pct) + "%";
      lv.appendChild(el("div", { classe: "envoi" },
        el("div", { classe: "ligne" }, el("span", { classe: "nom", texte: "Vers « " + e.cible + " » · " + e.fichiers + " fichier(s)" })),
        barre, el("p", { classe: e.erreur ? "erreur" : e.fini ? "ok" : "doux", texte: t })));
    });

    // infos
    var inf = vider($("infos"));
    inf.appendChild(el("p", null, el("strong", { texte: etat.nom }), etat.nom_mdns ? el("span", { classe: "doux", texte: "  ·  " + etat.nom_mdns }) : null));
    (etat.adresses || []).forEach(function (a) { inf.appendChild(el("p", { classe: "doux", texte: a.ip + " — " + a.reseau })); });
    inf.appendChild(el("p", { classe: "doux", texte: "Réception : " + etat.reception }));
    var sy = etat.systeme || {};
    document.querySelectorAll(".windows").forEach(function (n) { n.hidden = !sy.pris_en_charge; });
    $("etat-pare-feu").textContent = sy.pare_feu ? "Pare-feu : autorisé ✓" : "Pare-feu : pas encore autorisé";
    $("etat-pare-feu").className = sy.pare_feu ? "ok" : "erreur";
    $("pare-feu").hidden = !!sy.pare_feu;
    $("demarrage").checked = !!sy.demarrage;
    $("envoyer-vers").checked = !!sy.envoyer_vers;
    $("ouvrir-liens").checked = !!etat.ouvrir_liens;
  }

  function apresPremierRendu() {
    $("nom").value = etat.nom;
    $("lien-raccourci").value = etat.lien_raccourci || "";
    var params = new URLSearchParams(location.search);
    var sel = params.get("sel");
    if (params.get("action") === "recevoir") { history.replaceState(null, "", location.pathname); $("recevoir-telephone").click(); }
    else if (sel) {
      api("/controle/selection/" + sel).then(function (r) { montrerSelection(sel, r.fichiers); })
        .catch(function (e) { alert(e.message); });
    } else if (!etat.configure) assistant();
  }

  // ---------- envoyer ----------
  var depot = $("depot"), choix = $("choix");
  ["dragenter", "dragover"].forEach(function (t) { depot.addEventListener(t, function (e) { e.preventDefault(); depot.classList.add("survol"); }); });
  ["dragleave", "drop"].forEach(function (t) { depot.addEventListener(t, function () { depot.classList.remove("survol"); }); });
  depot.addEventListener("drop", function (e) { e.preventDefault(); if (e.dataTransfer.files.length) televerser(e.dataTransfer.files); });
  choix.addEventListener("change", function () { if (choix.files.length) televerser(choix.files); choix.value = ""; });

  function televerser(fichiers) {
    var form = new FormData(), noms = [], total = 0;
    for (var i = 0; i < fichiers.length; i++) { form.append("fichier", fichiers[i], fichiers[i].name); noms.push({ nom: fichiers[i].name, taille: taille(fichiers[i].size) }); total += fichiers[i].size; }
    var liste = vider($("liste-selection"));
    liste.appendChild(el("p", { classe: "doux", texte: "Préparation de " + fichiers.length + " fichier(s), " + taille(total) + "…" }));
    $("selection").hidden = false; vider($("cibles"));
    api("/controle/televerser", form).then(function (r) { montrerSelection(r.selection, noms); })
      .catch(function (e) { liste.appendChild(el("p", { classe: "erreur", texte: e.message })); });
  }

  function montrerSelection(id, fichiers) {
    $("selection").hidden = false;
    var liste = vider($("liste-selection"));
    fichiers.forEach(function (f) { liste.appendChild(el("div", { classe: "ligne" }, el("span", { classe: "nom", texte: f.nom }), el("span", { classe: "doux", texte: f.taille }))); });
    var c = vider($("cibles"));
    pcs().forEach(function (a) {
      c.appendChild(el("button", { classe: "cible", onclick: function () { envoyerVers(id, a.nom); } },
        el("strong", null, el("span", { classe: "point" + (enLigne(a) ? " en-ligne" : "") }), a.nom),
        el("span", { texte: enLigne(a) ? "PC · en ligne" : "PC · hors ligne ?" })));
    });
    c.appendChild(el("button", { classe: "cible", onclick: function () { partagerVersTelephone(id); } },
      el("strong", { texte: "📱 Un téléphone" }), el("span", { texte: "par QR code, iPhone ou Android" })));
  }

  function finSelection() { $("selection").hidden = true; history.replaceState(null, "", location.pathname); }
  $("annuler-selection").addEventListener("click", finSelection);

  function envoyerVers(id, nom) {
    api("/controle/envoyer", { cible: nom, selection: id }).then(function () { finSelection(); rafraichir(); })
      .catch(function (e) { alert(e.message); });
  }

  function partagerVersTelephone(id) {
    api("/controle/partager", { selection: id }).then(function (r) {
      finSelection();
      ouvrir(el("div", null, el("h2", { texte: "Scanne avec l'appareil photo du téléphone" }),
        blocQR(r.liens, "Lien valable 10 minutes, utilisable une seule fois.")));
    }).catch(function (e) { alert(e.message); });
  }

  // ---------- recevoir depuis un téléphone ----------
  $("recevoir-telephone").addEventListener("click", function () {
    api("/controle/telephone/envoi", {}).then(function (r) {
      ouvrir(el("div", null, el("h2", { texte: "Scanne avec l'appareil photo du téléphone" }),
        blocQR(r.liens, "Puis choisis tes photos ou fichiers et touche « Envoyer ». Lien valable 15 minutes."),
        el("p", { classe: "alerte", texte: "Le téléphone affiche un avertissement de sécurité ? C'est normal la première fois : le PC utilise son propre certificat. Sur iPhone, ajoute l'iPhone (bouton « Ajouter un iPhone ») pour ne plus le voir. Sur Android : « Paramètres avancés », puis « Continuer »." })));
    }).catch(function (e) { alert(e.message); });
  });

  // ---------- appareils ----------
  function retirer(nom) {
    if (!confirm("Retirer « " + nom + " » des appareils de confiance ? Il ne pourra plus envoyer vers ce PC.")) return;
    api("/controle/revoquer", { nom: nom }).then(rafraichir).catch(function (e) { alert(e.message); });
  }

  $("ajouter-iphone").addEventListener("click", function () { assistantIphone(); });

  function etapes(n, total) {
    var e = el("div", { classe: "etapes" });
    for (var i = 0; i < total; i++) e.appendChild(el("span", { classe: i < n ? "fait" : "" }));
    return e;
  }

  function assistantIphone(suite) {
    var nom = el("input", { type: "text", value: "iPhone", maxlength: "40" });
    ouvrir(el("div", null, etapes(1, 4), el("h2", { texte: "Ajouter un iPhone" }),
      el("p", { texte: "Trois étapes, environ 5 minutes, une seule fois. Garde l'iPhone sous la main." }),
      el("label", { classe: "champ" }, "Nom de l'iPhone", nom),
      el("div", { classe: "boutons" }, el("button", { classe: "bouton", texte: "Commencer", onclick: function () {
        api("/controle/telephone/ajouter", { nom: nom.value }).then(function (r) { etapeProfil(r, suite); }).catch(function (e) { alert(e.message); });
      } }))), suite);
  }

  function etapeProfil(r, suite) {
    ouvrir(el("div", null, etapes(2, 4), el("h2", { texte: "1. Installer le certificat du PC" }),
      blocQR(r.profil),
      el("ol", { classe: "guide" },
        el("li", { texte: "Scanne avec l'appareil photo de l'iPhone et ouvre le lien dans Safari. Si Safari parle de connexion non privée : « Afficher les détails », puis « visiter ce site web »." }),
        el("li", { texte: "La page qui s'ouvre te guide : télécharger le profil, l'installer, puis l'activer dans Réglages > Général > Informations > Réglages des certificats." })),
      el("p", { classe: "doux", texte: "Vérifie que l'empreinte SHA-256 affichée par l'iPhone est exactement :" }),
      el("p", { classe: "empreinte", texte: r.empreinte }),
      el("div", { classe: "boutons" }, el("button", { classe: "bouton", texte: "C'est fait", onclick: function () { etapeJeton(r, suite); } }))), suite);
  }

  function etapeJeton(r, suite) {
    ouvrir(el("div", null, etapes(3, 4), el("h2", { texte: "2. Récupérer le jeton d'accès" }),
      blocQR(r.jeton, "Cette page ne s'ouvre qu'une fois : copie le jeton et l'adresse (appui long sur le texte)."),
      el("div", { classe: "boutons" }, el("button", { classe: "bouton", texte: "Copié", onclick: function () { etapeRaccourci(r, suite); } }))), suite);
  }

  function etapeRaccourci(r, suite) {
    var c = el("div", null, etapes(4, 4), el("h2", { texte: "3. Le Raccourci « Envoyer au PC »" }));
    if (r.lien_raccourci) {
      c.appendChild(blocQR([{ url: r.lien_raccourci, qr: r.raccourci_qr }]));
      c.appendChild(el("p", { texte: "Ouvre ce lien sur l'iPhone, touche « Ajouter le raccourci », puis colle le jeton et l'adresse quand il les demande." }));
    } else {
      c.appendChild(el("p", { texte: "Construis le Raccourci une fois en suivant le guide (10 minutes). Ensuite, dans n'importe quelle app : Partager → « Envoyer au PC »." }));
      c.appendChild(el("p", null, el("a", { href: GUIDE, target: "_blank", rel: "noopener", texte: "Ouvrir le guide du Raccourci" })));
    }
    c.appendChild(el("p", { classe: "doux", texte: "Adresse à utiliser : " + r.adresse }));
    c.appendChild(el("p", { classe: "alerte", texte: "Sans Raccourci, l'iPhone peut quand même envoyer : bouton « Afficher le QR code » de la carte « Recevoir depuis un téléphone »." }));
    c.appendChild(el("div", { classe: "boutons" }, el("button", { classe: "bouton", texte: "Terminer", onclick: function () { fermer(); rafraichir(); } })));
    ouvrir(c, suite);
  }

  // ---------- appairer un autre PC ----------
  $("appairer-pc").addEventListener("click", function () {
    var vus = [], connus = {};
    pcs().forEach(function (a) { connus[a.empreinte] = true; });
    for (var fp in (etat.vus || {})) if (!connus[fp]) vus.push(etat.vus[fp]);
    var hote = el("input", { type: "text", placeholder: "Adresse IP de l'autre PC, ex. 192.168.1.20" });
    var c = el("div", null, el("h2", { texte: "Ajouter un PC" }),
      el("p", { texte: "Lance Universal Airdrop sur l'autre PC. Sur l'un des deux, choisis « Attendre », sur l'autre « Se connecter »." }),
      el("div", { classe: "boutons gauche" },
        el("button", { classe: "bouton secondaire", texte: "Attendre une demande ici", onclick: attendre })),
      el("h3", { texte: "Se connecter à un PC" }));
    if (vus.length) c.appendChild(el("p", { classe: "doux", texte: "PC détectés sur le réseau : " + vus.join(", ") + " (saisis son adresse ci-dessous, affichée dans sa carte « Ce PC »)." }));
    c.appendChild(el("div", { classe: "rangee" }, hote, el("button", { classe: "bouton", texte: "Se connecter", onclick: function () { connecter(hote.value); } })));
    ouvrir(c);
  });

  function attendre() {
    api("/controle/appairage/ouvrir", {}).then(function () {
      var zone = el("div", null, el("h2", { texte: "En attente de l'autre PC…" }),
        el("p", { texte: "Sur l'autre PC : « Ajouter un PC » > « Se connecter », avec une de ces adresses :" }));
      (etat.adresses || []).forEach(function (a) { zone.appendChild(el("p", { classe: "url", texte: a.ip + "  (" + a.reseau + ")" })); });
      var fin = false;
      ouvrir(zone, function () { fin = true; });
      (function sonder() {
        if (fin) return;
        api("/controle/appairage").then(function (e) {
          if (e.etat === "a_confirmer") return comparer(e.code, e.nom, function (ok) { return api("/controle/appairage/decision", { accepter: ok }); }, function () { fin = true; });
          if (e.etat === "expire" || e.etat === "ferme") { ouvrir(el("p", { texte: "Temps écoulé sans demande. Recommence." })); return; }
          setTimeout(sonder, 700);
        });
      })();
    }).catch(function (e) { alert(e.message); });
  }

  function connecter(hote) {
    if (!hote.trim()) return;
    api("/controle/appairer", { hote: hote.trim() }).then(function (r) {
      var fin = false;
      ouvrir(el("p", { texte: "Connexion à " + hote + "…" }), function () { fin = true; });
      (function sonder() {
        if (fin) return;
        api("/controle/appairer/" + r.id).then(function (e) {
          if (e.etat === "a_confirmer" && !e._vu) {
            e._vu = true;
            return comparer(e.code, e.nom, function (ok) { return api("/controle/appairer/" + r.id + "/decision", { accepter: ok }); }, function () {
              (function attendreFin() {
                api("/controle/appairer/" + r.id).then(function (f) {
                  if (f.etat === "termine") { ouvrir(el("div", null, el("h2", { classe: "ok", texte: "Appairé avec « " + f.nom + " » ✓" }))); rafraichir(); }
                  else if (f.etat === "erreur") ouvrir(el("p", { classe: "erreur", texte: f.erreur }));
                  else setTimeout(attendreFin, 700);
                });
              })();
            });
          }
          if (e.etat === "erreur") { ouvrir(el("p", { classe: "erreur", texte: e.erreur })); return; }
          if (e.etat === "termine") { ouvrir(el("h2", { classe: "ok", texte: "Appairé avec « " + e.nom + " » ✓" })); rafraichir(); return; }
          setTimeout(sonder, 700);
        });
      })();
    }).catch(function (e) { alert(e.message); });
  }

  function comparer(code, nom, decider, apres) {
    ouvrir(el("div", null, el("h2", { texte: "Compare les codes" }),
      el("p", { texte: "« " + nom + " » demande à s'appairer. Le même code doit s'afficher sur les deux écrans :" }),
      el("p", { classe: "code", texte: code.slice(0, 3) + " " + code.slice(3) }),
      el("p", { classe: "doux", texte: "Codes différents ? Refuse : quelqu'un essaie peut-être de s'interposer." }),
      el("div", { classe: "boutons" },
        el("button", { classe: "bouton secondaire", texte: "Différent", onclick: function () { decider(false).then(function () { fermer(); }); } }),
        el("button", { classe: "bouton", texte: "Identique", onclick: function () {
          decider(true).then(function () { ouvrir(el("p", { texte: "Finalisation…" })); if (apres) apres(); setTimeout(rafraichir, 800); });
        } }))));
  }

  // ---------- réglages ----------
  $("ouvrir-dossier").addEventListener("click", function () { api("/controle/ouvrir", {}); });
  $("enregistrer-nom").addEventListener("click", function () {
    api("/controle/reglages", { nom: $("nom").value }).then(function (r) {
      alert("Nom enregistré : les autres appareils le voient déjà.");
      rafraichir();
    }).catch(function (e) { alert(e.message); });
  });
  $("enregistrer-raccourci").addEventListener("click", function () {
    api("/controle/reglages", { lien_raccourci: $("lien-raccourci").value.trim() }).then(function () { alert("Lien enregistré."); }).catch(function (e) { alert(e.message); });
  });
  $("ouvrir-liens").addEventListener("change", function () { api("/controle/reglages", { ouvrir_liens: this.checked }).then(rafraichir); });
  $("demarrage").addEventListener("change", function () { api("/controle/systeme", { demarrage: this.checked }).then(rafraichir).catch(function (e) { alert(e.message); }); });
  $("envoyer-vers").addEventListener("change", function () { api("/controle/systeme", { envoyer_vers: this.checked }).then(rafraichir).catch(function (e) { alert(e.message); }); });
  $("pare-feu").addEventListener("click", pareFeu);
  function pareFeu() {
    return api("/controle/systeme", { pare_feu: true }).then(function () { setTimeout(rafraichir, 3000); }).catch(function (e) { alert(e.message); });
  }
  $("quitter").addEventListener("click", function () {
    if (confirm("Quitter Universal Airdrop ? Ce PC ne recevra plus rien jusqu'au prochain lancement.")) api("/controle/quitter", {}).then(function () { document.body.textContent = "Universal Airdrop est arrêté. Tu peux fermer cet onglet."; });
  });

  // ---------- assistant de premier lancement ----------
  function assistant() {
    var nom = el("input", { type: "text", value: etat.nom, maxlength: "60" });
    ouvrir(el("div", null, etapes(1, 3), el("h2", { texte: "Bienvenue dans Universal Airdrop" }),
      el("p", { texte: "Envoie photos, vidéos, PDF et liens entre tes PC et ton téléphone, sans câble ni Internet : même Wi-Fi, partage de connexion ou Bluetooth." }),
      el("label", { classe: "champ" }, "Comment s'appelle ce PC ? (visible sur tes autres appareils)", nom),
      el("div", { classe: "boutons" }, el("button", { classe: "bouton", texte: "Continuer", onclick: function () {
        api("/controle/reglages", { nom: nom.value }).then(function () { return rafraichir(); }).then(etapeSysteme).catch(function (e) { alert(e.message); });
      } }))), terminerAssistant);
  }

  function etapeSysteme() {
    var sy = etat.systeme || {};
    if (!sy.pris_en_charge) return etapeTelephone();
    var dem = el("input", { type: "checkbox", checked: "" }), ev = el("input", { type: "checkbox", checked: "" });
    ouvrir(el("div", null, etapes(2, 3), el("h2", { texte: "Autoriser la réception" }),
      el("p", { texte: "Windows doit autoriser les autres appareils de ton réseau à envoyer vers ce PC. Clique ci-dessous, puis « Oui » dans la fenêtre de Windows." }),
      el("p", { classe: "doux", texte: "Seuls les appareils de ton réseau local sont autorisés, jamais Internet." }),
      el("div", { classe: "boutons gauche" }, el("button", { classe: "bouton", texte: sy.pare_feu ? "Déjà autorisé ✓" : "Autoriser", onclick: pareFeu })),
      el("label", { classe: "interrupteur" }, dem, "Lancer au démarrage de Windows"),
      el("label", { classe: "interrupteur" }, ev, "Ajouter « Universal Airdrop » au clic droit > Envoyer vers"),
      el("div", { classe: "boutons" }, el("button", { classe: "bouton", texte: "Continuer", onclick: function () {
        Promise.all([api("/controle/systeme", { demarrage: dem.checked }), api("/controle/systeme", { envoyer_vers: ev.checked })])
          .catch(function () {}).then(etapeTelephone);
      } }))), terminerAssistant);
  }

  function etapeTelephone() {
    ouvrir(el("div", null, etapes(3, 3), el("h2", { texte: "Et ton téléphone ?" }),
      el("p", { texte: "Avec un iPhone, ajoute-le pour envoyer depuis le bouton Partager. Avec un Android, rien à installer : utilise « Recevoir depuis un téléphone »." }),
      el("div", { classe: "boutons" },
        el("button", { classe: "bouton secondaire", texte: "Plus tard", onclick: fermer }),
        el("button", { classe: "bouton", texte: "Ajouter un iPhone", onclick: function () { terminerAssistant(); assistantIphone(); } }))), terminerAssistant);
  }

  function terminerAssistant() { api("/controle/reglages", { configure: true }).then(rafraichir); }

  // ---------- démarrage ----------
  if (!SECRET) { $("sans-secret").hidden = false; $("statut").textContent = ""; return; }
  $("app").hidden = false;
  rafraichir();
  setInterval(function () { if (!document.hidden) rafraichir(); }, 1500);
})();
