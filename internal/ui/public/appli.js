// App iPhone (web app sur l'écran d'accueil) : relier l'iPhone, Recevoir, Envoyer, guide des Raccourcis.
// Le jeton est gardé dans le stockage local de l'app (propre à l'icône de l'écran d'accueil).
(function () {
  "use strict";
  function $(id) { return document.getElementById(id); }
  var G = window.UAGuide, el = G.el;
  var CLE = "ua-jeton";
  function lire(k, st) { try { return (st || localStorage).getItem(k) || ""; } catch (e) { return ""; } }
  function ecrire(k, v, st) { try { if (v) (st || localStorage).setItem(k, v); else (st || localStorage).removeItem(k); } catch (e) { /* stockage indisponible */ } }
  var jeton = lire(CLE), moi = null;
  var autonome = window.navigator.standalone === true || (window.matchMedia && matchMedia("(display-mode: standalone)").matches);
  var VUES = ["v-installer", "v-relier", "v-accueil", "v-recevoir", "v-envoyer", "v-raccourcis"];

  function vue(id) {
    VUES.forEach(function (v) { $(v).classList.toggle("cache", v !== id); });
    window.scrollTo(0, 0);
  }
  function montrer(id, oui) { $(id).classList.toggle("cache", !oui); }
  function taille(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1).replace(".", ",") + " Go";
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(".", ",") + " Mo";
    return Math.max(1, Math.round(n / 1e3)) + " Ko";
  }
  function vider(e) { while (e.firstChild) e.removeChild(e.firstChild); return e; }

  function api(chemin, opts) {
    opts = opts || {};
    opts.headers = Object.assign({ "Authorization": "Bearer " + jeton }, opts.headers || {});
    return fetch(chemin, opts).then(function (r) {
      if (r.status === 401) { ecrire(CLE, ""); jeton = ""; demarrer(); throw new Error("Cet iPhone n'est plus relié : relie-le à nouveau."); }
      if (!r.ok) return r.json().catch(function () { return {}; }).then(function (j) { throw new Error(j.erreur || "Erreur " + r.status); });
      return r;
    });
  }
  function json(r) { return r.json(); }

  // ---------- démarrage ----------
  function demarrer() {
    if (!jeton) {
      montrer("delier", false);
      $("relie-a").textContent = $("appli").getAttribute("data-pc");
      if (!autonome && !lire("ua-safari", sessionStorage)) return vue("v-installer");
      return vue("v-relier");
    }
    api("/api/rapide/moi").then(json).then(function (m) {
      moi = m;
      $("relie-a").textContent = m.nom + " · relié à " + m.pc;
      montrer("delier", true);
      vue("v-accueil");
      majRecevoir();
    }).catch(function (e) {
      if (jeton) { vue("v-accueil"); $("s-recevoir").textContent = "PC injoignable : même Wi-Fi ? App lancée sur le PC ?"; }
    });
  }
  $("continuer-safari").addEventListener("click", function () { ecrire("ua-safari", "1", sessionStorage); demarrer(); });
  $("delier").addEventListener("click", function () {
    if (!confirm("Délier cet iPhone ? Il faudra le relier à nouveau (code à comparer sur le PC).")) return;
    ecrire(CLE, ""); jeton = ""; demarrer();
  });
  document.querySelectorAll("[data-retour]").forEach(function (b) {
    b.addEventListener("click", function () { vue("v-accueil"); majRecevoir(); });
  });
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible" && jeton && !$("v-accueil").classList.contains("cache")) majRecevoir();
  });

  // ---------- relier ----------
  function erreurRelier(m) { $("r-erreur").textContent = m; montrer("r-erreur", true); montrer("r-attente", false); montrer("f-relier", true); }
  $("f-relier").addEventListener("submit", function (e) {
    e.preventDefault();
    montrer("r-erreur", false);
    fetch("/api/v1/telephone/demande", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ nom: $("nom-iphone").value.trim() || "iPhone" }) })
      .then(function (r) { return r.json().then(function (j) { if (!r.ok) throw new Error(j.erreur || "Demande refusée"); return j; }); })
      .then(function (d) {
        $("r-code").textContent = d.code.slice(0, 3) + " " + d.code.slice(3);
        montrer("f-relier", false); montrer("r-attente", true);
        (function suivre() {
          fetch("/api/v1/telephone/demande/" + encodeURIComponent(d.id)).then(function (r) {
            if (r.status === 404) throw new Error("La demande a expiré (5 minutes). Recommence.");
            return r.json();
          }).then(function (x) {
            if (x.etat === "attente") return setTimeout(suivre, 1500);
            if (x.etat === "refusee") return erreurRelier("Refusé sur le PC.");
            jeton = x.jeton; ecrire(CLE, jeton);
            montrer("r-attente", false); montrer("f-relier", true);
            demarrer();
          }).catch(function (e) { erreurRelier(e.message); });
        })();
      }).catch(function (e) { erreurRelier(e.message || "PC injoignable. Même Wi-Fi ?"); });
  });
  $("f-jeton").addEventListener("submit", function (e) {
    e.preventDefault();
    var j = $("jeton-colle").value.trim();
    if (!j) return;
    jeton = j; ecrire(CLE, j); demarrer();
  });

  // ---------- accueil ----------
  function majRecevoir() {
    api("/api/rapide/boite?garder=1").then(json).then(function (b) {
      $("s-recevoir").textContent = b.nombre ? b.resume.replace(", copié(s)", "") : "Rien de nouveau pour l'instant";
      $("badge").textContent = b.nombre;
      montrer("badge", b.nombre > 0);
    }).catch(function () { $("s-recevoir").textContent = "PC injoignable : même Wi-Fi ? App lancée sur le PC ?"; });
  }
  $("b-recevoir").addEventListener("click", ouvrirRecevoir);
  $("b-envoyer").addEventListener("click", ouvrirEnvoyer);
  $("b-raccourcis").addEventListener("click", ouvrirRaccourcis);

  // ---------- recevoir ----------
  var ICONES = {
    photos: '<rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="9" cy="10" r="2"/><path d="M21 16l-5-5-9 9"/>',
    fichiers: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
    lien: '<path d="M10 14a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 10a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/>',
    texte: '<path d="M4 6h16M4 12h16M4 18h10"/>'
  };
  function icone(n) {
    var s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    s.setAttribute("viewBox", "0 0 24 24"); s.setAttribute("aria-hidden", "true");
    s.innerHTML = ICONES[n]; // contenu constant
    return s;
  }
  function accuser(id) { return api("/api/rapide/boite/" + encodeURIComponent(id) + "/recu", { method: "POST" }).catch(function () {}); }
  var attente = []; // fichiers téléchargés, pas encore enregistrés : { e, fichier, ligne }

  function ouvrirRecevoir() {
    vue("v-recevoir");
    var l = vider($("liste-recue"));
    attente = [];
    montrer("tout-enregistrer", false);
    l.appendChild(el("p", { classe: "sous centre", texte: "Chargement…" }));
    api("/api/rapide/boite?garder=1").then(json).then(function (b) {
      vider(l);
      if (!b.elements.length) {
        l.appendChild(el("div", { classe: "vide" }, el("div", { classe: "rond doux" }, icone("photos")),
          el("h2", { texte: "Rien à recevoir" }), el("p", { texte: "Sur le PC, coche cet iPhone comme destinataire, puis reviens ici." })));
        return;
      }
      var textes = b.elements.filter(function (e) { return e.type !== "fichier"; });
      var fichiers = b.elements.filter(function (e) { return e.type === "fichier"; });
      if (textes.length) {
        var c = el("section", { classe: "carte liste" });
        textes.forEach(function (e) { c.appendChild(ligneTexte(e)); });
        l.appendChild(c);
      }
      if (fichiers.length) {
        var cf = el("section", { classe: "carte liste" });
        l.appendChild(cf);
        fichiers.forEach(function (e) { cf.appendChild(ligneFichier(e)); });
        telecharger(fichiers, 0);
      }
    }).catch(function (e) { vider(l).appendChild(el("p", { classe: "erreur", texte: e.message })); });
  }

  function ligneTexte(e) {
    var lien = e.type === "lien";
    var b = el("button", { classe: "pastille-bouton", type: "button", texte: "Copier" });
    var ligne = el("div", { classe: "fichier" }, el("span", { classe: "vignette " + (lien ? "lien" : "fichier") }, icone(lien ? "lien" : "texte")),
      el("span", { classe: "infos" }, el("span", { classe: "nom texte-recu", texte: e.texte }), el("span", { classe: "sous", texte: "de " + e.de })), b);
    b.addEventListener("click", function () {
      navigator.clipboard.writeText(e.texte).then(function () {
        b.textContent = "Copié ✓"; b.disabled = true; accuser(e.id);
        if (lien && confirm("Ouvrir le lien dans Safari ?")) window.open(e.texte, "_blank", "noopener");
      }, function () { alert("Copie impossible : appui long sur le texte, puis Copier."); });
    });
    return ligne;
  }

  function ligneFichier(e) {
    var v = el("span", { classe: "vignette " + (e.destination === "photos" ? "image" : "fichier") }, icone(e.destination === "photos" ? "photos" : "fichiers"));
    var etat = el("span", { classe: "etat-f", texte: "en attente" });
    var ligne = el("div", { classe: "fichier" }, v,
      el("span", { classe: "infos" }, el("span", { classe: "nom", texte: e.nom }), el("span", { classe: "sous", texte: taille(e.taille) + " · de " + e.de })), etat);
    ligne._v = v; ligne._etat = etat; e._ligne = ligne;
    return ligne;
  }

  // Les fichiers sont téléchargés un par un (garder=1 : rien n'est retiré avant l'enregistrement).
  function telecharger(liste, i) {
    if (i >= liste.length) return majBoutonsEnregistrer();
    var e = liste[i], ligne = e._ligne;
    ligne._etat.textContent = "téléchargement…";
    api(e.url.replace(/^https:\/\/[^/]+/, "") + "?garder=1").then(function (r) { return r.blob(); }).then(function (blob) {
      var f = new File([blob], e.nom, { type: blob.type || "application/octet-stream" });
      if (e.destination === "photos" && /^image\//.test(f.type)) {
        var img = new Image(); img.alt = "";
        img.onload = function () { vider(ligne._v).appendChild(img); };
        img.src = URL.createObjectURL(f);
      }
      var b = el("button", { classe: "pastille-bouton", type: "button", texte: "Enregistrer" });
      ligne.replaceChild(b, ligne._etat); ligne._etat = b;
      b.addEventListener("click", function () { enregistrer([{ e: e, fichier: f, ligne: ligne }]); });
      attente.push({ e: e, fichier: f, ligne: ligne });
      telecharger(liste, i + 1);
    }).catch(function (x) { ligne._etat.textContent = "échec"; ligne._etat.className = "etat-f erreur"; telecharger(liste, i + 1); });
  }

  function majBoutonsEnregistrer() {
    var reste = attente.filter(function (a) { return !a.fait; });
    var b = $("tout-enregistrer");
    montrer("tout-enregistrer", reste.length > 1);
    var photos = reste.filter(function (a) { return a.e.destination === "photos"; }).length;
    b.textContent = photos === reste.length ? "Tout enregistrer dans Photos (" + reste.length + ")" : "Tout enregistrer (" + reste.length + ")";
  }
  $("tout-enregistrer").addEventListener("click", function () {
    var reste = attente.filter(function (a) { return !a.fait; });
    var photos = reste.filter(function (a) { return a.e.destination === "photos"; });
    // la feuille de partage propose « Enregistrer N images » seulement si tout est photo ou vidéo
    enregistrer(photos.length && photos.length < reste.length ? photos : reste);
  });

  function enregistrer(lot) {
    var fichiers = lot.map(function (a) { return a.fichier; });
    if (navigator.canShare && navigator.canShare({ files: fichiers })) {
      navigator.share({ files: fichiers }).then(function () {
        lot.forEach(function (a) {
          a.fait = true; accuser(a.e.id);
          a.ligne._etat.textContent = "✓"; a.ligne._etat.disabled = true; a.ligne._etat.className = "etat-f ok";
        });
        majBoutonsEnregistrer();
      }).catch(function () { /* annulé : rien n'est retiré, on pourra réessayer */ });
    } else {
      lot.forEach(function (a) { // navigateur sans partage de fichiers : téléchargement classique
        var lienDl = el("a", { href: URL.createObjectURL(a.fichier), download: a.e.nom });
        document.body.appendChild(lienDl); lienDl.click(); lienDl.remove();
        a.fait = true; accuser(a.e.id);
      });
      majBoutonsEnregistrer();
    }
  }

  // ---------- envoyer ----------
  var choix = {};
  function ouvrirEnvoyer() {
    vue("v-envoyer");
    montrer("e-erreur", false); montrer("e-ok", false); montrer("progression", false);
    var d = vider($("destinations"));
    api("/api/rapide/destinations").then(json).then(function (r) {
      choix = {}; choix[r.ce_pc] = true;
      r.destinations.forEach(function (n) {
        var b = el("button", { classe: "puce-choix" + (choix[n] ? " choisie" : ""), type: "button", texte: n });
        b.addEventListener("click", function () { choix[n] = !choix[n]; b.classList.toggle("choisie", !!choix[n]); majGo(); });
        d.appendChild(b);
      });
      majGo();
    }).catch(function (e) { $("e-erreur").textContent = e.message; montrer("e-erreur", true); });
  }
  function dest() { return Object.keys(choix).filter(function (n) { return choix[n]; }); }
  function majGo() {
    var n = $("fichiers").files.length, t = $("texte").value.trim(), k = dest().length;
    $("b-go").disabled = !k || (!n && !t);
    $("b-go").textContent = !k ? "Choisis au moins un appareil" : (n ? "Envoyer " + (n > 1 ? n + " fichiers" : "1 fichier") : "Envoyer") + (k > 1 ? " à " + k + " appareils" : " à " + dest()[0]);
  }
  $("texte").addEventListener("input", majGo);
  $("fichiers").addEventListener("change", function () {
    var c = vider($("choisis")), f = $("fichiers").files;
    for (var i = 0; i < f.length; i++) c.appendChild(el("div", { classe: "fichier" }, el("span", { classe: "infos" }, el("span", { classe: "nom", texte: f[i].name }), el("span", { classe: "sous", texte: taille(f[i].size) }))));
    montrer("choisis", f.length > 0);
    montrer("e-ok", false);
    majGo();
  });
  $("b-go").addEventListener("click", function () {
    var d = dest(), f = $("fichiers").files, t = $("texte").value.trim();
    var entetes = { "X-Destinations": encodeURIComponent(d.join("|")) };
    $("b-go").disabled = true;
    montrer("e-erreur", false); montrer("e-ok", false);
    var suite = Promise.resolve();
    if (t) suite = suite.then(function () {
      return api("/api/rapide/lien", { method: "POST", headers: Object.assign({ "Content-Type": "application/json" }, entetes), body: JSON.stringify({ texte: t }) });
    });
    if (f.length) suite = suite.then(function () { return envoyerFichiers(f, entetes); });
    suite.then(function () {
      $("e-ok").textContent = "Envoyé à " + d.join(", ") + " ✓";
      montrer("e-ok", true); montrer("progression", false);
      $("texte").value = ""; $("fichiers").value = ""; vider($("choisis")); montrer("choisis", false); majGo();
    }).catch(function (e) {
      $("e-erreur").textContent = e.message; montrer("e-erreur", true); montrer("progression", false); majGo();
    });
  });
  function envoyerFichiers(f, entetes) {
    return new Promise(function (ok, ko) {
      var form = new FormData();
      for (var i = 0; i < f.length; i++) form.append("fichier", f[i], f[i].name);
      var x = new XMLHttpRequest(), debut = Date.now();
      montrer("progression", true);
      x.upload.onprogress = function (e) {
        if (!e.lengthComputable) return;
        var p = e.loaded / e.total, s = (Date.now() - debut) / 1000;
        $("pourcent").textContent = Math.floor(100 * p) + " %";
        $("plein").style.width = (100 * p) + "%";
        $("detail").textContent = taille(e.loaded) + " sur " + taille(e.total) + (s > 0 ? " · " + (8 * e.loaded / s / 1e6).toFixed(0) + " Mbit/s" : "");
      };
      x.onload = function () {
        if (x.status === 200) return ok();
        var m = "L'envoi a échoué";
        try { m = JSON.parse(x.responseText).erreur || m; } catch (e) { /* réponse non JSON */ }
        ko(new Error(m));
      };
      x.onerror = function () { ko(new Error("Connexion perdue : même Wi-Fi que le PC ?")); };
      x.open("POST", "/api/rapide/fichier");
      x.setRequestHeader("Authorization", "Bearer " + jeton);
      for (var k in entetes) x.setRequestHeader(k, entetes[k]);
      x.send(form);
    });
  }

  // ---------- raccourcis ----------
  function ouvrirRaccourcis() {
    vue("v-raccourcis");
    $("g-adresse").textContent = moi ? moi.adresse : location.origin;
    $("g-jeton").textContent = jeton;
    [["g-envoyer", G.envoyer()], ["g-recevoir", G.recevoir()], ["g-auto", G.automatique()]].forEach(function (x) {
      var c = vider($(x[0])); x[1].forEach(function (n) { c.appendChild(n); });
    });
  }
  document.querySelectorAll("[data-copier]").forEach(function (b) {
    b.addEventListener("click", function () {
      navigator.clipboard.writeText($(b.getAttribute("data-copier")).textContent).then(function () {
        b.textContent = "Copié ✓"; setTimeout(function () { b.textContent = "Copier"; }, 2000);
      }, function () { alert("Copie impossible : appui long sur le texte, puis Copier."); });
    });
  });

  demarrer();
})();
