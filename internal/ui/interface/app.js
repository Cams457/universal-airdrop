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
  fenetre.addEventListener("close", function () { fenetre.classList.remove("large"); if (surFermeture) { var f = surFermeture; surFermeture = null; f(); } });
  function ouvrir(contenu, fermeture) {
    fenetre.classList.remove("large");
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
      var n = (etat.en_attente || {})[a.nom] || 0;
      var etiquette = a.type === "pc" ? (enLigne(a) ? "PC · en ligne" : "PC · hors ligne") : "téléphone" + (n ? " · " + n + " en attente" : "");
      var vider_ = n ? el("button", { classe: "lien-discret", texte: "Annuler l'envoi en attente", onclick: function () {
        if (!confirm("Annuler les " + n + " élément(s) que « " + a.nom + " » n'a pas encore récupérés ?")) return;
        api("/controle/boite/vider", { nom: a.nom }).then(rafraichir).catch(function (x) { alert(x.message); });
      } }) : null;
      l.appendChild(el("div", { classe: "ligne" },
        el("span", { classe: "nom" }, el("span", { classe: "point" + (a.type === "pc" && enLigne(a) ? " en-ligne" : "") }), a.nom),
        el("span", { classe: "rangee" }, el("span", { classe: "doux", texte: etiquette }), vider_,
          el("button", { classe: "lien-discret danger", texte: "Retirer", onclick: function () { retirer(a.nom); } }))));
    });

    demandesAcces();

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
      var t = e.annule ? "Annulé" : e.erreur ? "Échec : " + e.erreur : e.fini ? "Envoyé ✓ · " + Math.round(e.mbits) + " Mbit/s" : pct + " % · " + taille(e.fait) + " sur " + taille(e.total);
      var barre = el("div", { classe: "barre" + (e.annule ? " annulee" : "") }, el("div"));
      barre.firstChild.style.width = (e.erreur ? 100 : pct) + "%";
      var annuler = e.fini ? null : el("button", { classe: "lien-discret danger", texte: "Annuler", onclick: function () {
        this.disabled = true;
        api("/controle/envoi/" + encodeURIComponent(e.id) + "/annuler", {}).then(rafraichir).catch(function (x) { alert(x.message); });
      } });
      lv.appendChild(el("div", { classe: "envoi" },
        el("div", { classe: "ligne" }, el("span", { classe: "nom", texte: "Vers « " + e.cible + " » · " + e.fichiers + " fichier(s)" }), annuler),
        barre, el("p", { classe: e.annule ? "doux" : e.erreur ? "erreur" : e.fini ? "ok" : "doux", texte: t })));
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
    if (params.get("action") === "guide") { history.replaceState(null, "", location.pathname); ouvrirGuide(); }
    else if (params.get("action") === "recevoir") { history.replaceState(null, "", location.pathname); $("recevoir-telephone").click(); }
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

  // Panneau « Envoyer vers » : on coche un ou plusieurs appareils, puis « Envoyer ».
  // courant = { selection: id } pour des fichiers, ou { texte: "…" } pour un lien ou un texte.
  var courant = null, choisis = {};

  function montrerSelection(id, fichiers) {
    courant = { selection: id };
    var liste = vider($("liste-selection"));
    fichiers.forEach(function (f) { liste.appendChild(el("div", { classe: "ligne" }, el("span", { classe: "nom", texte: f.nom }), el("span", { classe: "doux", texte: f.taille }))); });
    montrerCibles();
  }

  function montrerTexte(texte) {
    courant = { texte: texte };
    var lien = /^https?:\/\//i.test(texte);
    var liste = vider($("liste-selection"));
    liste.appendChild(el("div", { classe: "ligne apercu-texte" },
      el("span", { classe: "nom", texte: (lien ? "🔗 " : "📝 ") + (texte.length > 160 ? texte.slice(0, 160) + "…" : texte) }),
      el("span", { classe: "doux", texte: lien ? "lien" : "texte" })));
    liste.appendChild(el("p", { classe: "doux petit", texte: "Il sera copié dans le presse-papiers de chaque appareil choisi" + (lien ? " (et ouvert, si l'appareil l'autorise)." : ".") }));
    montrerCibles();
  }

  // Toutes les destinations possibles : PC appairés, téléphones de ce PC (boîte d'envoi) et
  // téléphones des autres PC (relais).
  function destinations() {
    var l = [];
    pcs().forEach(function (a) {
      l.push({ cle: "pc:" + a.nom, nom: a.nom, sous: enLigne(a) ? "PC · en ligne" : "PC · hors ligne ?", point: enLigne(a) ? "en-ligne" : "", type: "pc" });
    });
    (etat.appareils || []).filter(function (a) { return a.type === "telephone"; }).forEach(function (a) {
      l.push({ cle: "tel:" + a.nom, nom: a.nom, sous: "📱 téléphone", point: "telephone", type: "tel" });
    });
    (etat.telephones_distants || []).forEach(function (d) {
      l.push({ cle: "dist:" + d.nom + "\u0000" + d.via, nom: d.nom, via: d.via, sous: "📱 via " + d.via, point: "telephone", type: "dist" });
    });
    return l;
  }

  function montrerCibles() {
    $("selection").hidden = false;
    var c = vider($("cibles")), tous = destinations();
    var garde = {};
    tous.forEach(function (d) { if (choisis[d.cle]) garde[d.cle] = d; });
    choisis = garde;
    if (tous.length === 1) choisis[tous[0].cle] = tous[0];
    if (!tous.length) c.appendChild(el("p", { classe: "doux", texte: "Aucun appareil pour l'instant : ajoute ton iPhone ou un autre PC (carte Appareils)." }));
    tous.forEach(function (d) {
      var b = el("button", { classe: "cible" + (choisis[d.cle] ? " choisie" : ""), type: "button", "aria-pressed": choisis[d.cle] ? "true" : "false" },
        el("span", { classe: "coche", "aria-hidden": "true" }),
        el("strong", null, el("span", { classe: "point " + d.point }), d.nom),
        el("span", { texte: d.sous }));
      b.addEventListener("click", function () {
        if (choisis[d.cle]) delete choisis[d.cle]; else choisis[d.cle] = d;
        b.classList.toggle("choisie", !!choisis[d.cle]);
        b.setAttribute("aria-pressed", choisis[d.cle] ? "true" : "false");
        majBouton();
      });
      c.appendChild(b);
    });
    majBouton();
  }

  function coches() { return Object.keys(choisis).map(function (k) { return choisis[k]; }); }

  function majBouton() {
    var l = coches(), n = l.length, b = $("envoyer-choix");
    b.disabled = n === 0;
    b.textContent = n === 0 ? "Choisis un ou plusieurs appareils" : n === 1 ? "Envoyer à " + l[0].nom : "Envoyer aux " + n + " appareils";
  }

  function corpsDestinations() {
    var cibles = [], distants = [];
    coches().forEach(function (d) { if (d.type === "dist") distants.push({ nom: d.nom, via: d.via }); else cibles.push(d.nom); });
    return { cibles: cibles, distants: distants };
  }

  $("envoyer-choix").addEventListener("click", function () {
    var b = $("envoyer-choix"), corps = corpsDestinations();
    if (!coches().length || !courant) return;
    b.disabled = true;
    if (courant.selection) corps.selection = courant.selection; else corps.texte = courant.texte;
    var req = api(courant.selection ? "/controle/envoyer" : "/controle/lien", corps);
    req.then(function (r) {
      if (courant && courant.texte) {
        var rates = (r.resultats || []).filter(function (x) { return x.erreur; });
        if (rates.length) alert("Non envoyé à : " + rates.map(function (x) { return x.cible + " (" + x.erreur + ")"; }).join(", "));
        $("texte-envoi").value = "";
      }
      finSelection(); rafraichir();
    }).catch(function (e) { b.disabled = false; alert(e.message); });
  });

  $("envoyer-telephone").addEventListener("click", function () {
    if (!courant) return;
    var corps = courant.selection ? { selection: courant.selection } : { url: courant.texte };
    if (courant.texte && !/^https?:\/\//i.test(courant.texte)) { alert("Par QR code, on ne peut partager qu'un lien http(s) ou des fichiers."); return; }
    api("/controle/partager", corps).then(function (r) {
      finSelection();
      ouvrir(el("div", null, el("h2", { texte: "Scanne avec l'appareil photo du téléphone" }),
        blocQR(r.liens, "Lien valable 10 minutes, utilisable une seule fois.")));
    }).catch(function (e) { alert(e.message); });
  });

  function finSelection() { courant = null; $("selection").hidden = true; history.replaceState(null, "", location.pathname); }
  $("annuler-selection").addEventListener("click", finSelection);

  function preparerTexte() {
    var t = $("texte-envoi").value.trim();
    if (t) montrerTexte(t);
  }
  $("bouton-texte").addEventListener("click", preparerTexte);
  $("texte-envoi").addEventListener("keydown", function (e) { if (e.key === "Enter") { e.preventDefault(); preparerTexte(); } });
  // coller un lien n'importe où dans la page (hors champ de saisie) le prépare directement
  document.addEventListener("paste", function (e) {
    var cible = e.target;
    if (cible && (cible.tagName === "INPUT" || cible.tagName === "TEXTAREA")) return;
    var t = (e.clipboardData && e.clipboardData.getData("text")) || "";
    if (t.trim()) { $("texte-envoi").value = t.trim(); preparerTexte(); }
  });

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

  // Ajouter un iPhone, sans QR code obligatoire : l'iPhone ouvre la page /iphone dans Safari
  // (adresse à taper, ou QR code pour aller plus vite), installe le certificat, puis demande l'accès ;
  // le PC affiche alors une fenêtre avec un code à comparer.
  function assistantIphone(suite) {
    api("/controle/telephone/page").then(function (r) {
      var c = el("div", null, etapes(1, 2), el("h2", { texte: "Ajouter un iPhone" }),
        el("p", { texte: "Sur l'iPhone, ouvre Safari et tape cette adresse en entier, avec https:// au début :" }),
        el("p", { classe: "adresse-grande", texte: r.liens[0].url }),
        el("p", { classe: "doux", texte: "La page guide pas à pas : installer le certificat de ce PC, puis « Relier cet iPhone ». Une fenêtre s'ouvrira ici pour accepter, avec un code à comparer." }));
      var d = el("details", null, el("summary", { texte: "Plus rapide : scanner un QR code, ou une autre adresse" }));
      d.appendChild(blocQR(r.liens, "Si l'adresse en .local ne répond pas, essaie une adresse ci-dessous (même Wi-Fi ou partage de connexion)."));
      c.appendChild(d);
      c.appendChild(el("div", { classe: "boutons" }, el("button", { classe: "bouton", texte: "Ensuite : les Raccourcis", onclick: function () { etapeRaccourcis(r, suite); } })));
      ouvrir(c, suite);
    }).catch(function (e) { alert(e.message); });
  }

  function etapeRaccourcis(r, suite) {
    var c = el("div", null, etapes(2, 2), el("h2", { texte: "Les deux Raccourcis de l'iPhone" }),
      el("ul", { classe: "guide" },
        el("li", null, el("strong", { texte: "Envoyer" }), " : dans n'importe quelle app, Partager → « Envoyer », puis coche les appareils."),
        el("li", null, el("strong", { texte: "Recevoir" }), " : récupère ce que les PC lui ont envoyé (photos dans Photos, liens copiés). Il peut se lancer tout seul en arrivant sur ton Wi-Fi.")));
    if (r.lien_raccourci) {
      c.appendChild(blocQR([{ url: r.lien_raccourci, qr: r.raccourci_qr }], "Ouvre ce lien sur l'iPhone pour installer les Raccourcis."));
    } else {
      c.appendChild(el("p", null, el("a", { href: GUIDE, target: "_blank", rel: "noopener", texte: "Ouvrir le guide des Raccourcis (15 minutes, une seule fois)" })));
    }
    c.appendChild(el("div", { classe: "boutons" }, el("button", { classe: "bouton", texte: "Terminer", onclick: function () { fermer(); rafraichir(); } })));
    ouvrir(c, suite);
  }

  // Demande d'accès d'un iPhone (page /iphone) : fenêtre avec le code à comparer.
  var demandesVues = {};
  function demandesAcces() {
    (etat.demandes || []).forEach(function (d) {
      if (demandesVues[d.id]) return;
      demandesVues[d.id] = true;
      var decider = function (ok) {
        api("/controle/telephone/demande", { id: d.id, accepter: ok }).then(function () { fermer(); rafraichir(); })
          .catch(function (e) { alert(e.message); fermer(); });
      };
      ouvrir(el("div", null, el("h2", { texte: "Ajouter « " + d.nom + " » ?" }),
        el("p", { texte: "Vérifie que l'iPhone affiche exactement ce code :" }),
        el("p", { classe: "code-grand", texte: d.code.slice(0, 3) + " " + d.code.slice(3) }),
        el("p", { classe: "doux", texte: "Demande venue de " + d.ip + ". Si le code est différent, refuse : ce n'est pas ton iPhone." }),
        el("div", { classe: "boutons" },
          el("button", { classe: "bouton secondaire", texte: "Refuser", onclick: function () { decider(false); } }),
          el("button", { classe: "bouton", texte: "Accepter", onclick: function () { decider(true); } }))));
    });
  }

  // ---------- guide (bouton « ? ») ----------
  // Texte avec `code`, **gras** et *italique*, sans HTML injecté.
  function riche(t) {
    var s = el("span"), re = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*)/g, i = 0, m;
    while ((m = re.exec(t))) {
      if (m.index > i) s.appendChild(document.createTextNode(t.slice(i, m.index)));
      var x = m[0];
      s.appendChild(x[0] === "`" ? el("code", { texte: x.slice(1, -1) })
        : x[1] === "*" ? el("strong", { texte: x.slice(2, -2) }) : el("em", { texte: x.slice(1, -1) }));
      i = m.index + x.length;
    }
    if (i < t.length) s.appendChild(document.createTextNode(t.slice(i)));
    return s;
  }
  function pas(liste) { var o = el("ol", { classe: "pas" }); liste.forEach(function (t) { o.appendChild(el("li", null, riche(t))); }); return o; }
  function tableActions(lignes) {
    var t = el("table", { classe: "actions-raccourci" }, el("tr", null, el("th", { texte: "#" }), el("th", { texte: "Action à ajouter" }), el("th", { texte: "Réglage" })));
    lignes.forEach(function (l, i) {
      var r = el("tr", null, el("td", { texte: String(i + 1) }), el("td", { classe: l[2] ? "retrait" : "" }, riche((l[2] ? "↳ ".repeat(l[2]) : "") + "**" + l[0] + "**")), el("td", null, riche(l[1])));
      t.appendChild(r);
    });
    return t;
  }
  function encadre(lignes) { var d = el("div", { classe: "encadre" }); lignes.forEach(function (t) { d.appendChild(el("p", null, riche(t))); }); return d; }

  var REGLAGES = [
    "**Réglages G** (touche « Afficher plus » sur l'action) : méthode **GET** ; en-tête `Authorization` = `Bearer ` (avec l'espace) suivi de la variable `Jeton`.",
    "**Réglages L** (lien ou texte) : méthode **POST** ; en-têtes `Authorization` = `Bearer ` + `Jeton` et `X-Destinations` = `Destinations` ; corps **JSON** avec un champ Texte de clé `texte` et de valeur *Élément répété*.",
    "**Réglages F** (fichier) : méthode **POST** ; mêmes en-têtes ; corps **Formulaire** avec un champ **Fichier** de clé `fichier` et de valeur *Élément répété*."
  ];

  function chapitres() {
    var sy = etat.systeme || {}, tels = (etat.appareils || []).filter(function (a) { return a.type === "telephone"; });
    return [
      { titre: "Installer sur ce PC", sous: "nom, pare-feu, démarrage automatique", fait: !!etat.configure && (!sy.pris_en_charge || sy.pare_feu),
        corps: function () {
          return [pas([
            "Donne un nom à ce PC : c'est ce que verront tes autres appareils (actuellement **" + etat.nom + "**).",
            "**Autoriser** la réception dans le pare-feu de Windows (fenêtre « Oui »). Seul ton réseau local est autorisé, jamais Internet." + (sy.pris_en_charge ? (sy.pare_feu ? " — déjà fait ✓" : " — **pas encore fait**") : ""),
            "Facultatif : lancement au démarrage de Windows, et « Envoyer vers » dans le clic droit sur un fichier.",
            "Sur chaque autre PC : télécharge le même `UniversalAirdrop.exe` et refais ces étapes."]),
            el("div", { classe: "boutons gauche" }, el("button", { classe: "bouton", texte: "Lancer l'assistant d'installation", onclick: assistant }))];
        } },
      { titre: "Relier ton iPhone", sous: "certificat du PC, puis code à 6 chiffres", fait: tels.length > 0,
        corps: function () {
          var adr = el("p", { classe: "adresse-grande", texte: "…" });
          api("/controle/telephone/page").then(function (r) { adr.textContent = r.liens[0].url; }).catch(function () {});
          return [el("p", { texte: "Sur l'iPhone, ouvre Safari et tape cette adresse, avec https:// au début :" }), adr,
            pas(["Si Safari parle de connexion non privée : **Afficher les détails**, puis **visiter ce site web** (normal avant l'étape 3).",
              "**Télécharger le profil**, puis **Autoriser**.",
              "**Réglages** › **Profil téléchargé** › **Installer**. Absent ? **Général** › **VPN et gestion de l'appareil**. iOS l'efface au bout de 8 minutes s'il n'est pas installé.",
              "**Réglages** › **Général** › **Informations** › **Réglages des certificats** : active *Universal Airdrop CA*. Vérifie que l'empreinte SHA-256 est celle de la carte « Ce PC ».",
              "**Relier cet iPhone** : donne-lui un nom. Une fenêtre s'ouvre ici avec un code : s'il est identique à celui de l'iPhone, **Accepter**.",
              "Copie l'**adresse du PC** et le **jeton** affichés sur l'iPhone : les Raccourcis les demandent."]),
            el("div", { classe: "boutons gauche" }, el("button", { classe: "bouton secondaire", texte: "Afficher aussi un QR code", onclick: function () { assistantIphone(); } }))];
        } },
      { titre: "Raccourci « Envoyer »", sous: "Partager → Envoyer, en choisissant les appareils", fait: false,
        corps: function () {
          return [encadre(["App **Raccourcis** › **+** › renomme-le `Envoyer`. Touche **(i)** : active **Afficher dans la feuille de partage** ; types acceptés : *Images, Médias, Fichiers, URL, Texte* ; si aucune entrée : **Arrêter et répondre**."]),
            tableActions([
              ["Texte", "colle l'**adresse du PC**, ex. `https://pc-de-camil.local:47800`"],
              ["Définir la variable", "nom `Adresse`, valeur *Texte*"],
              ["Texte", "colle ton **jeton**"],
              ["Définir la variable", "nom `Jeton`, valeur *Texte*"],
              ["Obtenir le contenu de l'URL", "`Adresse` puis `/api/rapide/destinations` · Réglages G"],
              ["Obtenir la valeur du dictionnaire", "Valeur pour la clé `destinations`"],
              ["Choisir dans la liste", "invite `Envoyer à…` ; active **Sélectionner plusieurs**"],
              ["Combiner le texte", "séparateur **Personnalisé** `|`"],
              ["Encoder l'URL", "*Texte combiné* (en anglais : URL Encode)"],
              ["Définir la variable", "nom `Destinations`"],
              ["Répéter avec chaque élément", "dans *Entrée du raccourci*"],
              ["Obtenir le type", "de *Élément répété*", 1],
              ["Si", "*Type* est `URL`, **Ajouter une condition** : *Type* est `Texte`, **N'importe laquelle**", 1],
              ["Obtenir le contenu de l'URL", "`Adresse` puis `/api/rapide/lien` · Réglages L", 2],
              ["Sinon", "", 1],
              ["Obtenir le contenu de l'URL", "`Adresse` puis `/api/rapide/fichier` · Réglages F", 2],
              ["Fin si", "", 1],
              ["Fin de la répétition", ""],
              ["Afficher une notification", "`Envoyé ✓`"]]),
            encadre(REGLAGES),
            el("p", { classe: "doux", texte: "Essai : Photos › une photo › Partager › Envoyer › coche ce PC. La première fois : « Toujours autoriser ». Pour l'avoir en tête : feuille de partage › Modifier les actions › + devant Envoyer." })];
        } },
      { titre: "Raccourci « Recevoir »", sous: "photos dans Photos, liens copiés", fait: false,
        corps: function () {
          return [encadre(["**+** › renomme-le `Recevoir` (pas besoin de la feuille de partage). Les actions 1 à 4 sont les mêmes que pour Envoyer."]),
            tableActions([
              ["Texte", "l'adresse du PC"], ["Définir la variable", "`Adresse`"], ["Texte", "ton jeton"], ["Définir la variable", "`Jeton`"],
              ["Obtenir le contenu de l'URL", "`Adresse` puis `/api/rapide/boite` · Réglages G"],
              ["Définir la variable", "nom `Boite`"],
              ["Obtenir la valeur du dictionnaire", "clé `texte` dans `Boite`"],
              ["Si", "*Valeur du dictionnaire* **a une valeur**"],
              ["Copier dans le presse-papiers", "*Valeur du dictionnaire*", 1],
              ["Fin si", ""],
              ["Obtenir la valeur du dictionnaire", "clé `fichiers` dans `Boite`"],
              ["Répéter avec chaque élément", "dans *Valeur du dictionnaire*"],
              ["Obtenir la valeur du dictionnaire", "clé `url` dans *Élément répété*", 1],
              ["Obtenir le contenu de l'URL", "*Valeur du dictionnaire* · Réglages G", 1],
              ["Définir la variable", "nom `Fichier`", 1],
              ["Obtenir la valeur du dictionnaire", "clé `destination` dans *Élément répété*", 1],
              ["Si", "*Valeur du dictionnaire* est `photos`", 1],
              ["Enregistrer dans l'album photo", "`Fichier`", 2],
              ["Sinon", "", 1],
              ["Enregistrer le fichier", "`Fichier` ; désactive **Demander où enregistrer** ; dossier `Universal Airdrop`", 2],
              ["Fin si", "", 1],
              ["Fin de la répétition", ""],
              ["Obtenir la valeur du dictionnaire", "clé `resume` dans `Boite`"],
              ["Afficher une notification", "*Valeur du dictionnaire*"]]),
            el("p", { classe: "doux", texte: "Essai : sur ce PC, glisse une photo, coche ton iPhone, Envoyer ; puis lance Recevoir sur l'iPhone." })];
        } },
      { titre: "Lancer « Recevoir » tout seul", sous: "en arrivant sur le Wi-Fi, ou d'un geste", fait: false,
        corps: function () {
          return [pas([
            "**En arrivant à la maison** : Raccourcis › **Automatisation** › **+** › **Wi-Fi** › ton réseau › **Se connecte** › **Exécuter immédiatement** (désactive *Me notifier*) › **Suivant** › *Recevoir*.",
            "**D'un double tapotement au dos** : Réglages › Accessibilité › Toucher › **Toucher le dos** › Toucher deux fois › *Recevoir*.",
            "**Depuis l'écran d'accueil** : appui long › **+** › Raccourcis › *Recevoir*.",
            "iPhone 15 Pro et plus récents : le **bouton Action** peut lancer *Recevoir*."]),
            el("p", { classe: "doux", texte: "En partage de connexion, l'iPhone n'est pas « connecté à un Wi-Fi » : utilise alors le toucher du dos ou le widget. Sans application de l'App Store, iOS ne permet pas une réception instantanée." })];
        } },
      { titre: "Ajouter un autre PC", sous: "code à 6 chiffres comparé sur les deux écrans", fait: pcs().length > 0,
        corps: function () {
          return [pas([
            "Installe l'application sur l'autre PC (chapitre 1), sur le même Wi-Fi ou le même partage de connexion.",
            "Sur l'un des deux : **Ajouter un PC** › **Attendre une demande ici**.",
            "Sur l'autre : **Ajouter un PC**, tape l'adresse affichée par le premier, **Se connecter**.",
            "Les deux écrans affichent un code à 6 chiffres : s'ils sont identiques, accepte des deux côtés.",
            "L'iPhone relié à un seul PC peut quand même envoyer aux autres : ce PC relaie."]),
            el("div", { classe: "boutons gauche" }, el("button", { classe: "bouton", texte: "Ajouter un PC", onclick: function () { fermer(); $("appairer-pc").click(); } }))];
        } },
      { titre: "Au quotidien", sous: "envoyer, recevoir, liens", fait: false,
        corps: function () {
          return [pas([
            "**Du PC** : glisse des fichiers dans la fenêtre (ou clic droit › Envoyer vers › Universal Airdrop), coche un ou plusieurs appareils, **Envoyer**.",
            "**Un lien ou un texte** : Ctrl+V dans la fenêtre, coche, Envoyer. Il arrive dans le **presse-papiers** de l'autre PC (Ctrl+V), avec une notification.",
            "**De l'iPhone** : Partager › **Envoyer** › coche les appareils.",
            "**Vers l'iPhone** : coche-le ; il le reçoit avec **Recevoir** (photos dans Photos, liens copiés).",
            "**Téléphone d'un ami ou Android** : « Afficher le QR code » pour qu'il t'envoie, « Un téléphone, par QR code » pour lui envoyer.",
            "Les fichiers reçus sont dans **Téléchargements › Universal Airdrop**."])];
        } }
    ];
  }

  function ouvrirGuide(n) {
    var ch = chapitres();
    if (n == null) {
      var l = el("ol", { classe: "sommaire" });
      ch.forEach(function (c, i) {
        l.appendChild(el("li", null, el("button", { type: "button", onclick: function () { ouvrirGuide(i); } },
          el("span", { classe: "num" + (c.fait ? " fait" : ""), texte: c.fait ? "✓" : String(i + 1) }),
          el("span", null, el("span", { classe: "titre", texte: c.titre }), el("span", { classe: "sous", texte: c.sous })))));
      });
      ouvrir(el("div", null, el("h2", { texte: "Guide d'installation" }),
        el("p", { classe: "doux", texte: "Dans l'ordre, une seule fois. Les étapes cochées sont déjà faites sur ce PC." }), l,
        el("p", { classe: "doux petit" }, "Version texte complète : ", el("a", { href: GUIDE, target: "_blank", rel: "noopener", texte: "guide des Raccourcis sur GitHub" }))));
      fenetre.classList.add("large");
      return;
    }
    var c = ch[n], div = el("div", { classe: "chapitre" },
      el("p", { classe: "fil", texte: "Étape " + (n + 1) + " sur " + ch.length }), el("h2", { texte: c.titre }));
    c.corps().forEach(function (x) { div.appendChild(x); });
    div.appendChild(el("div", { classe: "nav-guide" },
      el("button", { classe: "lien-discret", texte: "← Sommaire", onclick: function () { ouvrirGuide(); } }),
      el("span", { classe: "rangee" },
        n > 0 ? el("button", { classe: "bouton secondaire", texte: "Précédent", onclick: function () { ouvrirGuide(n - 1); } }) : null,
        n < ch.length - 1 ? el("button", { classe: "bouton", texte: "Suivant : " + ch[n + 1].titre, onclick: function () { ouvrirGuide(n + 1); } })
                          : el("button", { classe: "bouton", texte: "Terminer", onclick: fermer }))));
    ouvrir(div);
    fenetre.classList.add("large");
    fenetre.scrollTop = 0;
  }
  $("aide").addEventListener("click", function () { ouvrirGuide(); });

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
