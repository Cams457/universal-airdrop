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
  var VUES = ["v-installer", "v-relier", "v-accueil", "v-recevoir", "v-textes", "v-envoyer", "v-tailscale", "v-aide", "v-raccourcis"];
  var vuePrecedente = "v-accueil";

  // Chaque écran est une entrée d'historique : le geste « retour » de Safari ramène à l'écran précédent.
  var depuisHistorique = false;
  function vue(id) {
    var actuelle = VUES.filter(function (v) { return !$(v).classList.contains("cache"); })[0];
    if (id === "v-aide" && actuelle && actuelle !== "v-aide") vuePrecedente = actuelle;
    VUES.forEach(function (v) { $(v).classList.toggle("cache", v !== id); });
    var avecOnglets = !!jeton && id !== "v-installer" && id !== "v-relier";
    montrer("onglets", avecOnglets);
    $("appli").classList.toggle("avec-onglets", avecOnglets);
    document.querySelectorAll("[data-onglet]").forEach(function (b) { b.classList.toggle("actif", b.getAttribute("data-onglet") === id); });
    if (!depuisHistorique && actuelle !== id) {
      try { history[actuelle ? "pushState" : "replaceState"]({ vue: id }, ""); } catch (e) { /* sans historique */ }
    }
    window.scrollTo(0, 0);
  }
  var OUVRIR = {};
  window.addEventListener("popstate", function (ev) {
    var id = ev.state && ev.state.vue;
    if (!id || !OUVRIR[id] || !jeton) return;
    depuisHistorique = true;
    try { OUVRIR[id](); } finally { depuisHistorique = false; }
  });
  function montrer(id, oui) { $(id).classList.toggle("cache", !oui); }
  function taille(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1).replace(".", ",") + " Go";
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(".", ",") + " Mo";
    return Math.max(1, Math.round(n / 1e3)) + " Ko";
  }
  function vider(e) { while (e.firstChild) e.removeChild(e.firstChild); return e; }

  // Copie dans le presse-papiers de l'iPhone. API moderne d'abord ; sinon (ancien iOS, contexte
  // jugé non sûr), l'ancienne méthode : sélection d'un champ caché + execCommand("copy").
  function copierTexte(t) {
    function secours() {
      var z = el("textarea", { classe: "hors-ecran", readonly: "" });
      z.value = t; document.body.appendChild(z);
      z.focus(); z.select(); try { z.setSelectionRange(0, t.length); } catch (x) { /* rien */ }
      var ok = false; try { ok = document.execCommand("copy"); } catch (x) { ok = false; }
      z.remove(); return ok;
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(t).catch(function () { if (!secours()) throw new Error("copie refusée"); });
    }
    return secours() ? Promise.resolve() : Promise.reject(new Error("copie refusée"));
  }
  // Dernier recours : le texte est sélectionné, iOS propose alors lui-même « Copier ».
  function selectionner(n) {
    try { var r = document.createRange(); r.selectNodeContents(n); var s = window.getSelection(); s.removeAllRanges(); s.addRange(r); } catch (x) { /* rien */ }
  }

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
  var repriseEssayee = false;
  function demarrer() {
    if (!jeton && !repriseEssayee) {
      // Stockage local effacé par iOS ? Le PC a gardé une copie du lien (cookie) : on la reprend.
      repriseEssayee = true;
      return fetch("/api/rapide/reprendre", { credentials: "same-origin" }).then(function (r) {
        return r.ok ? r.json() : null;
      }).then(function (x) {
        if (x && x.jeton) { jeton = x.jeton; ecrire(CLE, jeton); }
      }, function () { /* PC injoignable : écran habituel */ }).then(demarrer);
    }
    if (!jeton) {
      montrer("delier", false);
      $("relie-a").textContent = $("appli").getAttribute("data-pc");
      if (lire("ua-nom")) $("nom-iphone").value = lire("ua-nom");
      return vue("v-relier");
    }
    api("/api/rapide/moi").then(json).then(function (m) {
      moi = m;
      $("relie-a").textContent = m.nom + " · relié à " + m.pc;
      montrer("delier", true);
      $("s-tailscale").textContent = m.via_tailscale ? "Actif ✓ : cette app marche partout"
        : m.adresse_distance ? "Prêt sur le PC : reste une étape sur l'iPhone" : "Tailscale : 4G, autre Wi-Fi…";
      montrer("astuce-install", !autonome && !lire("ua-astuce-vue"));
      // Arrivée par le QR code du PC (/app#v-envoyer…) : directement sur le bon écran.
      var cible = (location.hash || "").slice(1);
      if (cible && cible !== "v-accueil" && OUVRIR[cible]) {
        try { history.replaceState(null, "", location.pathname); } catch (x) { /* rien */ }
        OUVRIR[cible]();
      } else vue("v-accueil");
      majRecevoir();
    }).catch(function (e) {
      if (jeton) { vue("v-accueil"); $("s-recevoir").textContent = "PC injoignable : même Wi-Fi ? App lancée sur le PC ?"; }
    });
  }
  $("fermer-astuce").addEventListener("click", function () { ecrire("ua-astuce-vue", "1"); montrer("astuce-install", false); });
  $("continuer-safari").addEventListener("click", function () { ecrire("ua-safari", "1", sessionStorage); demarrer(); });
  $("delier").addEventListener("click", function () {
    if (!confirm("Délier cet iPhone ? Il faudra le relier à nouveau (code à comparer sur le PC).")) return;
    ecrire(CLE, ""); jeton = ""; repriseEssayee = true;
    fetch("/api/rapide/oublier", { method: "POST", credentials: "same-origin" }).catch(function () {}).then(demarrer);
  });
  document.querySelectorAll("[data-retour]").forEach(function (b) {
    b.addEventListener("click", function () {
      if (b.closest("#v-aide") && vuePrecedente !== "v-accueil") return vue(vuePrecedente);
      vue("v-accueil"); majRecevoir();
    });
  });
  $("b-aide").addEventListener("click", ouvrirAide);
  function accueil() { vue("v-accueil"); majRecevoir(); }
  document.querySelectorAll("[data-onglet]").forEach(function (b) {
    b.addEventListener("click", function () { OUVRIR[b.getAttribute("data-onglet")](); });
  });
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible" && jeton && !$("v-accueil").classList.contains("cache")) majRecevoir();
  });

  // ---------- relier ----------
  function erreurRelier(m) { $("r-erreur").textContent = m; montrer("r-erreur", true); montrer("r-attente", false); montrer("f-relier", true); }
  $("f-relier").addEventListener("submit", function (e) {
    e.preventDefault();
    montrer("r-erreur", false);
    ecrire("ua-nom", $("nom-iphone").value.trim());
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
      $("s-recevoir").textContent = b.nombre ? b.resume.replace(", copié(s)", "") : (b.elements.length ? "Rien de nouveau · " + b.elements.length + " dans tes reçus" : "Rien de nouveau pour l'instant");
      $("badge").textContent = b.nombre;
      montrer("badge", b.nombre > 0);
      $("badge-onglet").textContent = b.nombre;
      montrer("badge-onglet", b.nombre > 0);
      var tn = (b.elements || []).filter(function (e) { return e.type !== "fichier" && !e.enregistre; }).length;
      var tt = (b.elements || []).filter(function (e) { return e.type !== "fichier"; }).length;
      $("badge-textes").textContent = tn; montrer("badge-textes", tn > 0);
      $("badge-onglet-textes").textContent = tn; montrer("badge-onglet-textes", tn > 0);
      $("s-textes").textContent = tn ? tn + (tn > 1 ? " nouveaux à copier" : " nouveau à copier") : tt ? tt + " reçu" + (tt > 1 ? "s" : "") + " · copie, colle, renvoie" : "Reçois-les en entier, copie, colle, renvoie";
    }).catch(function () { $("s-recevoir").textContent = "PC injoignable : même Wi-Fi ? App lancée sur le PC ?"; });
  }
  $("b-recevoir").addEventListener("click", ouvrirRecevoir);
  $("b-envoyer").addEventListener("click", ouvrirEnvoyer);
  $("b-textes").addEventListener("click", ouvrirTextes);
  $("b-raccourcis").addEventListener("click", ouvrirRaccourcis);
  $("b-tailscale").addEventListener("click", ouvrirTailscale);
  $("coller").addEventListener("click", function () {
    if (!navigator.clipboard || !navigator.clipboard.readText) return alert("Appui long dans le champ, puis Coller.");
    navigator.clipboard.readText().then(function (t) { $("texte").value = t; majGo(); }, function () {});
  });

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
  ICONES.poubelle = '<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/>';
  function accuser(id) { return api("/api/rapide/boite/" + encodeURIComponent(id) + "/recu", { method: "POST" }).catch(function () {}); }
  function quand(iso) {
    var d = new Date(iso), maint = new Date(), h = d.toLocaleTimeString("fr-FR", { hour: "2-digit", minute: "2-digit" });
    var jours = Math.floor((new Date(maint.toDateString()) - new Date(d.toDateString())) / 864e5);
    return jours === 0 ? "aujourd'hui " + h : jours === 1 ? "hier " + h : d.toLocaleDateString("fr-FR", { day: "numeric", month: "short" }) + " " + h;
  }
  var prets = []; // fichiers téléchargés sur l'iPhone : { e, fichier, ligne }

  // « Reçus » : tout ce que les PC t'ont envoyé ces 7 derniers jours, nouveaux en premier.
  // Chaque élément reste là après l'enregistrement : on peut l'enregistrer ailleurs, ou le supprimer.
  function ouvrirRecevoir() {
    vue("v-recevoir");
    var l = vider($("liste-recue"));
    prets = [];
    montrer("tout-enregistrer", false);
    l.appendChild(el("p", { classe: "sous centre", texte: "Chargement…" }));
    api("/api/rapide/boite?garder=1").then(json).then(function (b) {
      vider(l);
      var els = b.elements.slice().sort(function (x, y) { return x.cree < y.cree ? 1 : -1; });
      if (!els.length) {
        l.appendChild(el("div", { classe: "vide" }, el("div", { classe: "rond doux" }, icone("photos")),
          el("h2", { texte: "Rien reçu pour l'instant" }), el("p", { texte: "Sur le PC, coche cet iPhone comme destinataire : ce qu'il t'envoie apparaît ici (7 jours)." })));
        return;
      }
      if (els.some(function (e) { return e.type === "fichier"; })) {
        l.appendChild(el("div", { classe: "encadre astuce" }, G.riche("**Enregistrer…** ouvre le menu d'iOS : *Enregistrer l'image* ou *Enregistrer la vidéo* pour **Photos**, ou *Enregistrer dans Fichiers* pour **choisir le dossier** (iPhone, iCloud Drive, clé USB…).")));
      }
      var nouveaux = els.filter(function (e) { return !e.enregistre; }), anciens = els.filter(function (e) { return e.enregistre; });
      [["Nouveaux", nouveaux], ["Déjà enregistrés", anciens]].forEach(function (g) {
        if (!g[1].length) return;
        l.appendChild(el("p", { classe: "titre-section", texte: g[0] + " (" + g[1].length + ")" }));
        var c = el("section", { classe: "carte liste" });
        g[1].forEach(function (e) { c.appendChild(e.type === "fichier" ? ligneFichier(e) : ligneTexte(e)); });
        l.appendChild(c);
      });
      telecharger(nouveaux.filter(function (e) { return e.type === "fichier"; }), 0);
    }).catch(function (e) { vider(l).appendChild(el("p", { classe: "erreur", texte: e.message })); });
  }

  function boutonSupprimer(e, ligne) {
    var b = el("button", { classe: "poubelle", type: "button", "aria-label": "Supprimer" }, icone("poubelle"));
    b.addEventListener("click", function () {
      if (!confirm("Supprimer « " + (e.nom || e.texte.slice(0, 40)) + " » de tes reçus ?")) return;
      api("/api/rapide/boite/" + encodeURIComponent(e.id), { method: "DELETE" }).then(function () {
        ligne.remove(); prets = prets.filter(function (a) { return a.e.id !== e.id; }); majBoutonsEnregistrer();
      }).catch(function (x) { alert(x.message); });
    });
    return b;
  }

  function ligneTexte(e) {
    var lien = e.type === "lien";
    var copier = el("button", { classe: "pastille-bouton", type: "button", texte: e.enregistre ? "Recopier" : "Copier" });
    var ligne = el("div", { classe: "fichier" }, el("span", { classe: "vignette " + (lien ? "lien" : "fichier") }, icone(lien ? "lien" : "texte")),
      el("span", { classe: "infos" }, el("span", { classe: "nom texte-recu", texte: e.texte }),
        el("span", { classe: "sous", texte: (e.de ? "de " + e.de + " · " : "") + quand(e.cree) })),
      el("span", { classe: "actions-ligne" }, copier, null));
    ligne.lastChild.appendChild(boutonSupprimer(e, ligne));
    copier.addEventListener("click", function () {
      copierTexte(e.texte).then(function () {
        copier.textContent = "Copié ✓"; accuser(e.id);
        setTimeout(function () { copier.textContent = "Recopier"; }, 2000);
      }, function () { ouvrirTextes(); });
    });
    ligne.querySelector(".texte-recu").addEventListener("click", ouvrirTextes); // en entier dans « Textes »
    return ligne;
  }

  function ligneFichier(e) {
    var v = el("span", { classe: "vignette " + (e.destination === "photos" ? "image" : "fichier") }, icone(e.destination === "photos" ? "photos" : "fichiers"));
    var bouton = el("button", { classe: "pastille-bouton", type: "button", texte: e.enregistre ? "Télécharger" : "…" });
    bouton.classList.add("action-fichier");
    var ligne = el("div", { classe: "fichier" }, v,
      el("span", { classe: "infos" }, el("span", { classe: "nom", texte: e.nom }),
        el("span", { classe: "sous", texte: taille(e.taille) + (e.de ? " · de " + e.de : "") + " · " + quand(e.cree) + (e.enregistre ? " · enregistré ✓" : "") })),
      el("span", { classe: "actions-ligne" }, bouton));
    ligne.lastChild.appendChild(boutonSupprimer(e, ligne));
    ligne._v = v; ligne._b = bouton; e._ligne = ligne;
    bouton.addEventListener("click", function () {
      var pret = prets.filter(function (a) { return a.e.id === e.id; })[0];
      if (pret) return enregistrer([pret]);
      if (bouton.disabled) return;
      telecharger([e], 0); // un fichier déjà enregistré se retélécharge à la demande
    });
    return ligne;
  }

  // Téléchargement un par un (garder=1 : rien n'est effacé du PC) ; ensuite, « Enregistrer… ».
  function telecharger(liste, i) {
    if (i >= liste.length) return majBoutonsEnregistrer();
    var e = liste[i], ligne = e._ligne, b = ligne._b;
    b.textContent = "Téléchargement…"; b.disabled = true;
    api(e.url.replace(/^https:\/\/[^/]+/, "") + "?garder=1").then(function (r) { return r.blob(); }).then(function (blob) {
      var f = new File([blob], e.nom, { type: blob.type || "application/octet-stream" });
      if (e.destination === "photos" && /^image\//.test(f.type)) {
        var img = new Image(); img.alt = "";
        img.onload = function () { vider(ligne._v).appendChild(img); };
        img.src = URL.createObjectURL(f);
      }
      prets.push({ e: e, fichier: f, ligne: ligne });
      b.textContent = "Enregistrer…"; b.disabled = false;
      telecharger(liste, i + 1);
    }).catch(function () { b.textContent = "Réessayer"; b.disabled = false; telecharger(liste, i + 1); });
  }

  function majBoutonsEnregistrer() {
    var nouveaux = prets.filter(function (a) { return !a.e.enregistre; });
    var b = $("tout-enregistrer");
    montrer("tout-enregistrer", nouveaux.length > 1);
    var photos = nouveaux.filter(function (a) { return a.e.destination === "photos"; }).length;
    b.textContent = photos === nouveaux.length ? "Tout enregistrer dans Photos (" + nouveaux.length + ")" : "Tout enregistrer… (" + nouveaux.length + ")";
  }
  $("tout-enregistrer").addEventListener("click", function () {
    var nouveaux = prets.filter(function (a) { return !a.e.enregistre; });
    var photos = nouveaux.filter(function (a) { return a.e.destination === "photos"; });
    // la feuille de partage propose « Enregistrer N images » seulement si tout est photo ou vidéo
    enregistrer(photos.length && photos.length < nouveaux.length ? photos : nouveaux);
  });

  // La feuille de partage d'iOS : Photos, « Enregistrer dans Fichiers » (choix du dossier), ou une app.
  function enregistrer(lot) {
    var fichiers = lot.map(function (a) { return a.fichier; });
    var fait = function () {
      lot.forEach(function (a) {
        a.e.enregistre = true; accuser(a.e.id);
        var s = a.ligne.querySelector(".infos .sous");
        if (s && s.textContent.indexOf("enregistré ✓") < 0) s.textContent += " · enregistré ✓";
      });
      majBoutonsEnregistrer();
    };
    if (navigator.canShare && navigator.canShare({ files: fichiers })) {
      navigator.share({ files: fichiers }).then(fait).catch(function () { /* annulé : rien ne change */ });
    } else {
      lot.forEach(function (a) { // navigateur sans partage de fichiers : téléchargement classique
        var lienDl = el("a", { href: URL.createObjectURL(a.fichier), download: a.e.nom });
        document.body.appendChild(lienDl); lienDl.click(); lienDl.remove();
      });
      fait();
    }
  }

  // ---------- textes et liens ----------
  // Les textes et liens reçus, en entier et sélectionnables, avec Copier (et Ouvrir pour un lien).
  // La liste se met à jour toute seule tant que l'écran est ouvert.
  var choixTexte = {}, minuterieTextes = null, signatureTextes = "";
  function ouvrirTextes() {
    vue("v-textes");
    montrer("t-etat", false);
    signatureTextes = "";
    vider($("t-liste")).appendChild(el("p", { classe: "sous centre", texte: "Chargement…" }));
    chargerDestinations($("t-destinations"), choixTexte, majTexte).catch(function () { /* affiché par la liste */ });
    chargerTextes();
    clearInterval(minuterieTextes);
    minuterieTextes = setInterval(function () {
      if ($("v-textes").classList.contains("cache")) { clearInterval(minuterieTextes); minuterieTextes = null; return; }
      if (document.visibilityState === "visible") chargerTextes();
    }, 3000);
  }
  function chargerTextes() {
    return api("/api/rapide/boite?garder=1").then(json).then(function (b) {
      var t = (b.elements || []).filter(function (e) { return e.type !== "fichier"; })
        .sort(function (x, y) { return x.cree < y.cree ? 1 : -1; });
      $("t-maj").textContent = "· à jour " + new Date().toLocaleTimeString("fr-FR", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
      var sig = t.map(function (e) { return e.id + (e.enregistre ? "+" : "-"); }).join(",");
      if (sig === signatureTextes) return; // rien de neuf : on ne touche pas à une sélection en cours
      signatureTextes = sig;
      var l = vider($("t-liste"));
      if (!t.length) {
        l.appendChild(el("div", { classe: "vide" }, el("div", { classe: "rond doux" }, icone("texte")),
          el("h2", { texte: "Aucun texte reçu" }), el("p", { texte: "Sur le PC : Ctrl+V dans la fenêtre (ou écris dans le champ texte), coche cet iPhone, Envoyer. Il s'affiche ici aussitôt." })));
        return;
      }
      t.forEach(function (e) { l.appendChild(carteTexte(e)); });
    }).catch(function (e) { vider($("t-liste")).appendChild(el("p", { classe: "erreur", texte: e.message })); });
  }
  function carteTexte(e) {
    var lien = e.type === "lien" && /^https?:\/\//i.test(e.texte);
    var corps = el("div", { classe: "texte-plein", texte: e.texte });
    var copier = el("button", { classe: "bouton principal", type: "button", texte: e.enregistre ? "Copier à nouveau" : "Copier" });
    var actions = el("div", { classe: "actions-texte" }, copier,
      lien ? el("a", { classe: "bouton secondaire", href: e.texte, target: "_blank", rel: "noopener noreferrer", texte: "Ouvrir" }) : null);
    var c = el("div", { classe: "carte texte-carte" + (e.enregistre ? "" : " nouveau") },
      el("div", { classe: "texte-entete" }, el("span", { classe: "vignette " + (lien ? "lien" : "fichier") }, icone(lien ? "lien" : "texte")),
        el("span", { classe: "sous" }, document.createTextNode((lien ? "Lien" : "Texte") + (e.de ? " · de " + e.de : "") + " · " + quand(e.cree)))),
      corps, actions);
    c.firstChild.appendChild(boutonSupprimerTexte(e, c));
    copier.addEventListener("click", function () {
      copierTexte(e.texte).then(function () {
        copier.textContent = "Copié ✓ : colle-le où tu veux";
        c.classList.remove("nouveau");
        if (!e.enregistre) { e.enregistre = true; accuser(e.id).then(majRecevoir); }
        setTimeout(function () { copier.textContent = "Copier à nouveau"; }, 2500);
      }, function () {
        selectionner(corps);
        copier.textContent = "Touche « Copier » au-dessus du texte";
      });
    });
    return c;
  }
  function boutonSupprimerTexte(e, carte) {
    var b = el("button", { classe: "poubelle", type: "button", "aria-label": "Supprimer" }, icone("poubelle"));
    b.addEventListener("click", function () {
      if (!confirm("Supprimer ce " + (e.type === "lien" ? "lien" : "texte") + " ?")) return;
      api("/api/rapide/boite/" + encodeURIComponent(e.id), { method: "DELETE" }).then(function () {
        carte.remove(); signatureTextes = ""; chargerTextes(); majRecevoir();
      }).catch(function (x) { alert(x.message); });
    });
    return b;
  }
  function majTexte() {
    var k = Object.keys(choixTexte).filter(function (n) { return choixTexte[n]; });
    $("t-envoyer").disabled = !k.length || !$("t-saisie").value.trim();
    $("t-envoyer").textContent = !k.length ? "Choisis un appareil" : "Envoyer" + (k.length > 1 ? " à " + k.length + " appareils" : " à " + k[0]);
  }
  $("t-saisie").addEventListener("input", majTexte);
  $("t-coller").addEventListener("click", function () {
    if (!navigator.clipboard || !navigator.clipboard.readText) { $("t-saisie").focus(); return alert("Appui long dans le champ, puis Coller."); }
    navigator.clipboard.readText().then(function (t) { $("t-saisie").value = t; majTexte(); }, function () { $("t-saisie").focus(); });
  });
  $("t-envoyer").addEventListener("click", function () {
    var t = $("t-saisie").value.trim(), d = Object.keys(choixTexte).filter(function (n) { return choixTexte[n]; });
    if (!t || !d.length) return;
    $("t-envoyer").disabled = true;
    $("t-etat").className = "sous"; $("t-etat").textContent = "Envoi…"; montrer("t-etat", true);
    api("/api/rapide/lien", { method: "POST", headers: { "Content-Type": "application/json", "X-Destinations": encodeURIComponent(d.join("|")) },
      body: JSON.stringify({ texte: t }) }).then(json).then(function (r) {
      var b = bilanTexte(r);
      $("t-etat").className = b.indexOf("⚠") >= 0 || b.indexOf("✗") >= 0 ? "erreur" : "ok-texte";
      $("t-etat").textContent = b;
      $("t-saisie").value = ""; majTexte();
    }).catch(function (e) {
      $("t-etat").className = "erreur"; $("t-etat").textContent = e.message; majTexte();
    });
  });

  // ---------- envoyer ----------
  // Puces des appareils destinataires (PC puis téléphones), le PC relié choisi par défaut.
  function chargerDestinations(d, choix, maj) {
    vider(d);
    return api("/api/rapide/destinations").then(json).then(function (r) {
      Object.keys(choix).forEach(function (k) { delete choix[k]; });
      choix[r.ce_pc] = true;
      var groupes = [["PC", (r.appareils || []).filter(function (a) { return a.type === "pc"; }), "💻 "],
                     ["Autres téléphones (ils le recevront dans leur app)", (r.appareils || []).filter(function (a) { return a.type !== "pc"; }), "📱 "]];
      groupes.forEach(function (g) {
        if (!g[1].length) return;
        if (g[0] !== "PC") d.appendChild(el("p", { classe: "sous-groupe", texte: g[0] }));
        g[1].forEach(function (a) {
          var n = a.nom, b = el("button", { classe: "puce-choix" + (choix[n] ? " choisie" : ""), type: "button", texte: g[2] + n });
          b.addEventListener("click", function () { choix[n] = !choix[n]; b.classList.toggle("choisie", !!choix[n]); maj(); });
          d.appendChild(b);
        });
      });
      maj();
    });
  }
  // Ce que le serveur dit d'un texte envoyé : copié ou non sur le PC, relayé, en attente.
  function bilanTexte(r) {
    var m = [];
    if (r && "copie" in r) m.push(r.copie ? "✓ Copié dans le presse-papiers de " + r.pc + " (Ctrl+V)"
      : "⚠ Reçu par " + r.pc + " mais pas copié (" + (r.raison || "raison inconnue") + ") : il est dans « Reçus récemment » du PC");
    (r && r.relais || []).forEach(function (p) {
      m.push(r.echecs && r.echecs[p] ? "✗ " + p + " : " + r.echecs[p] : "✓ Copié sur " + p);
    });
    (r && r.en_attente_pour || []).forEach(function (t) { m.push("✓ " + t + " : dans son app, rubrique Textes"); });
    return m.join("\n") || "Envoyé ✓";
  }

  var choix = {};
  function ouvrirEnvoyer() {
    vue("v-envoyer");
    montrer("e-erreur", false); montrer("e-ok", false); montrer("progression", false);
    chargerDestinations($("destinations"), choix, majGo).catch(function (e) { $("e-erreur").textContent = e.message; montrer("e-erreur", true); });
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
    var suite = Promise.resolve(), bilan = "";
    if (t) suite = suite.then(function () {
      return api("/api/rapide/lien", { method: "POST", headers: Object.assign({ "Content-Type": "application/json" }, entetes), body: JSON.stringify({ texte: t }) })
        .then(json).then(function (r) { bilan = bilanTexte(r); });
    });
    if (f.length) suite = suite.then(function () { return envoyerFichiers(f, entetes); });
    suite.then(function () {
      $("e-ok").textContent = (f.length || !bilan ? "Envoyé à " + d.join(", ") + " ✓" : "") + (bilan && f.length ? "\n" : "") + bilan;
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

  // ---------- hors de chez toi (Tailscale) ----------
  // L'app (et son jeton) appartient à l'adresse qui l'a ouverte : pour qu'elle marche partout, il
  // faut l'ouvrir par l'adresse Tailscale du PC et l'ajouter à l'écran d'accueil depuis celle-ci.
  function ouvrirTailscale() {
    vue("v-tailscale");
    var e = vider($("ts-etat")), d = moi && moi.adresse_distance;
    if (moi && moi.via_tailscale) {
      e.appendChild(el("p", { classe: "ok-fort", texte: "✓ Cette app passe par Tailscale : elle marche à la maison comme ailleurs." }));
      e.appendChild(el("p", { classe: "sous", texte: "Garde l'app Tailscale activée sur l'iPhone." }));
    } else if (d) {
      e.appendChild(el("p", { classe: "ok-fort", texte: "✓ Tailscale est actif sur " + moi.pc + "." }));
      e.appendChild(el("p", { classe: "sous", texte: "Dernière étape : ouvre l'app par l'adresse Tailscale, puis ajoute-la à l'écran d'accueil (à la place de celle-ci) et relie-la." }));
      e.appendChild(el("div", { classe: "valeur" }, el("span", { classe: "etiquette", texte: "Adresse Tailscale de l'app" }), el("code", { id: "ts-adresse", texte: d + "/app" })));
      e.appendChild(el("a", { classe: "bouton principal", href: d + "/app", texte: "Ouvrir l'app par Tailscale" }));
    } else {
      e.appendChild(el("p", { texte: "Tailscale n'est pas encore actif sur " + (moi ? moi.pc : "le PC") + "." }));
    }
    var p = vider($("ts-pas"));
    [
      "Crée un compte gratuit sur **tailscale.com** (avec Google, Microsoft ou Apple) : le **même** pour tous tes appareils.",
      "Sur **chaque PC** : installe **Tailscale pour Windows** (tailscale.com/download) et connecte-toi. Puis, dans Universal Airdrop sur le PC : bouton **?** › Installer sur ce PC › **Autoriser** (une fois, pour ouvrir le pare-feu à Tailscale).",
      "Sur cet **iPhone** : installe l'app **Tailscale** depuis l'App Store (gratuite), connecte-toi avec le même compte et **autorise la configuration VPN**. Laisse-la activée : elle consomme très peu.",
      "Attends une minute, puis reviens ici : le PC doit apparaître « actif ». Touche **Ouvrir l'app par Tailscale**.",
      "Dans Safari : **Partager** › **Sur l'écran d'accueil** › **Ajouter**. Ouvre la nouvelle icône, touche **Relier cet iPhone** et accepte sur le PC (code à comparer). Tu peux supprimer l'ancienne icône.",
      "Tes Raccourcis (facultatifs) : remplace l'adresse en `.local` par l'adresse en `.ts.net` (section Raccourcis)."
    ].forEach(function (t) { p.appendChild(el("li", null, G.riche(t))); });
  }

  // ---------- aide (bouton « ? ») ----------
  function bloc(titre, contenus, ouvert) {
    var d = el("details", { classe: "aide-bloc" }, el("summary", { texte: titre }));
    if (ouvert) d.setAttribute("open", "");
    var c = el("div", { classe: "corps" });
    contenus.forEach(function (x) { c.appendChild(typeof x === "string" ? el("p", null, G.riche(x)) : x); });
    d.appendChild(c);
    return d;
  }
  function bouton(texte, action) { var b = el("button", { classe: "bouton secondaire", type: "button", texte: texte }); b.addEventListener("click", action); return b; }
  function sous(t) { return el("h3", { texte: t }); }

  function ouvrirAide() {
    vue("v-aide");
    var c = vider($("aide-contenu")), relie = !!jeton;
    c.appendChild(bloc("Installer l'app sur l'iPhone", [G.pas([
      "Sur le PC : **Ajouter un iPhone**. Dans Safari, ouvre l'adresse affichée (ou scanne le QR code).",
      "**Télécharger le profil** › **Autoriser** ; Réglages › **Profil téléchargé** › **Installer**.",
      "Réglages › Général › Informations › **Réglages des certificats** › active *Universal Airdrop CA*.",
      "**Ouvrir l'app**, puis **Partager** › **Sur l'écran d'accueil** › **Ajouter**.",
      "Ouvre l'icône **Airdrop** › **Relier cet iPhone** › accepte sur le PC si le code est le même."])], !relie));
    c.appendChild(bloc("Recevoir et choisir où enregistrer", [G.pas([
      "Sur le PC, coche cet iPhone et **Envoyer**. Ici : **Reçus** (le badge compte les nouveautés).",
      "**Enregistrer…** ouvre le menu d'iOS : *Enregistrer l'image / la vidéo* pour **Photos**, *Enregistrer dans Fichiers* pour **choisir le dossier**, ou une autre app (Mail, WhatsApp…).",
      "**Tout enregistrer** : toutes les nouveautés d'un coup.",
      "Un élément reste dans **Reçus** 7 jours (rubrique *Déjà enregistrés*) : tu peux l'enregistrer ailleurs, ou le **supprimer** (corbeille).",
      "Un lien ou un texte : rubrique **Textes** (ci-dessous)."]),
      bouton("Ouvrir Reçus", ouvrirRecevoir)]));
    c.appendChild(bloc("Textes et liens (copier-coller)", [G.pas([
      "Sur le PC : **Ctrl+V** dans la fenêtre (ou écris dans le champ texte), coche cet iPhone, **Envoyer**.",
      "Ici : **Textes** (onglet en bas). Le texte s'affiche **en entier**, et arrive tout seul si l'écran est ouvert.",
      "**Copier**, puis colle-le où tu veux (appui long › Coller). Pour une partie seulement : appui long sur le texte, sélectionne, *Copier*.",
      "Un lien : **Ouvrir** l'ouvre dans Safari.",
      "Dans l'autre sens : écris ou **Coller** en haut de l'écran, choisis le PC, **Envoyer** : il arrive dans son **presse-papiers** (Ctrl+V). L'app te dit s'il a bien été copié."]),
      bouton("Ouvrir Textes", ouvrirTextes)]));
    c.appendChild(bloc("Envoyer", [G.pas([
      "**Envoyer** › choisis des photos ou des fichiers, et/ou colle un lien (bouton **Coller**).",
      "Coche **un ou plusieurs appareils**, puis **Envoyer**.",
      "Sur un PC, un lien ou un texte arrive **dans son presse-papiers** (Ctrl+V), avec une notification ; les fichiers dans *Téléchargements › Universal Airdrop*."]),
      bouton("Ouvrir Envoyer", ouvrirEnvoyer)]));
    c.appendChild(bloc("Hors de chez toi (Tailscale)", [
      "Pour que l'app marche en 4G ou sur un autre Wi-Fi : **Tailscale**, gratuit, sur les PC et sur l'iPhone, avec le même compte.",
      bouton("Voir le tutoriel", ouvrirTailscale)]));
    c.appendChild(bloc("Raccourcis (bouton Partager)", [
      "Facultatif : ajoute **Envoyer** au bouton *Partager* de toutes les apps (Photos, Safari…). Ton adresse et ton jeton sont prêts à copier.",
      bouton("Construire les Raccourcis", ouvrirRaccourcis)]));
    c.appendChild(bloc("Dépannage", [
      sous("« Safari n'a pas pu ouvrir la page : le serveur ne répondait plus »"),
      G.pas([
        "Sur le PC, un **bandeau rouge** en haut de Universal Airdrop ? Clique **Autoriser**, puis « Oui » (pare-feu Windows).",
        "Même réseau ? Réglages › Wi-Fi › (i) › **Adresse IP** de l'iPhone, et sur le PC `ipconfig` dans cmd : les trois premiers nombres doivent être identiques.",
        "Wi-Fi d'école, de résidence, d'hôtel ou « invité » : il isole souvent les appareils. Active le **partage de connexion** de l'iPhone et connecte le PC dessus, ou utilise **Tailscale**.",
        "Universal Airdrop est-il lancé sur le PC (icône près de l'horloge) ? Un antivirus avec pare-feu (Norton, Avast…) peut aussi bloquer."]),
      sous("« Cette connexion n'est pas privée »"),
      el("p", null, G.riche("Le certificat du PC n'est pas encore approuvé : refais l'étape **Réglages des certificats**. Il n'y apparaît qu'**après** l'installation du profil.")),
      sous("« PC injoignable » en 4G ou ailleurs"),
      el("p", null, G.riche("Normal sans Tailscale : l'app ne parle au PC que sur le même réseau. Voir **Hors de chez toi**.")),
      sous("« Cet iPhone n'est plus relié »"),
      el("p", null, G.riche("Il a été retiré sur le PC, ou l'app a été réinstallée : **Relier cet iPhone** à nouveau.")),
      sous("Rien dans Reçus"),
      el("p", null, G.riche("Sur le PC, vérifie que c'est bien **cet** iPhone (même nom) qui est coché. Ce qui est envoyé attend 7 jours."))]));
    c.appendChild(el("p", { classe: "aide-pied", texte: (moi ? moi.nom + " · relié à " + moi.pc + " · " : "") + location.host + (moi && moi.version ? " · version " + moi.version : "") }));
  }

  // ---------- raccourcis ----------
  function ouvrirRaccourcis() {
    vue("v-raccourcis");
    $("g-adresse").textContent = moi ? moi.adresse : location.origin;
    $("g-jeton").textContent = jeton;
    if (moi && moi.adresse_distance) { $("g-distance").textContent = moi.adresse_distance; montrer("g-bloc-distance", true); }
    [["g-envoyer", G.envoyer()], ["g-recevoir", G.recevoir()], ["g-auto", G.automatique()]].forEach(function (x) {
      var c = vider($(x[0])); x[1].forEach(function (n) { c.appendChild(n); });
    });
  }
  document.querySelectorAll("[data-copier]").forEach(function (b) {
    b.addEventListener("click", function () {
      var n = $(b.getAttribute("data-copier"));
      copierTexte(n.textContent).then(function () {
        b.textContent = "Copié ✓"; setTimeout(function () { b.textContent = "Copier"; }, 2000);
      }, function () { selectionner(n); alert("Le texte est sélectionné : touche « Copier » dans le menu d'iOS."); });
    });
  });

  OUVRIR = { "v-accueil": accueil, "v-recevoir": ouvrirRecevoir, "v-textes": ouvrirTextes, "v-envoyer": ouvrirEnvoyer,
    "v-tailscale": ouvrirTailscale, "v-raccourcis": ouvrirRaccourcis, "v-aide": ouvrirAide };

  demarrer();
})();
