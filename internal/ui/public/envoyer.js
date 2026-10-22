// Page d'envoi téléphone -> PC : choix des fichiers (avec vignettes), envoi avec progression.
(function () {
  "use strict";
  function $(id) { return document.getElementById(id); }
  var entree = $("fichiers"), zone = $("zone"), liste = $("liste"), bouton = $("envoyer"), encore = $("encore");
  var prog = $("progression"), pourcent = $("pourcent"), plein = $("plein"), detail = $("detail");
  var fini = $("fini"), finiTexte = $("fini-texte"), erreur = $("erreur"), texte = $("texte-envoi");
  var urls = [];

  function taille(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1).replace(".", ",") + " Go";
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(".", ",") + " Mo";
    return Math.max(1, Math.round(n / 1e3)) + " Ko";
  }
  function duree(s) {
    if (!isFinite(s) || s < 0) return "";
    if (s < 60) return Math.max(1, Math.round(s)) + " s";
    return Math.floor(s / 60) + " min " + Math.round(s % 60) + " s";
  }
  function genre(f) {
    var t = f.type || "", n = f.name.toLowerCase();
    if (t.indexOf("image/") === 0 || /\.(heic|heif)$/.test(n)) return "image";
    if (t.indexOf("video/") === 0) return "video";
    if (t === "application/pdf" || /\.pdf$/.test(n)) return "pdf";
    return "fichier";
  }
  var ICONES = {
    image: '<rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="9" cy="10" r="2"/><path d="M21 16l-5-5-9 9"/>',
    video: '<rect x="3" y="5" width="13" height="14" rx="3"/><path d="M16 10l5-3v10l-5-3"/>',
    pdf: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h4"/>',
    fichier: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>'
  };
  function icone(g) {
    var s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    s.setAttribute("viewBox", "0 0 24 24");
    s.setAttribute("aria-hidden", "true");
    s.innerHTML = ICONES[g]; // contenu constant, défini ci-dessus
    return s;
  }
  function el(tag, classe, texte) {
    var e = document.createElement(tag);
    if (classe) e.className = classe;
    if (texte) e.textContent = texte;
    return e;
  }
  function vider(e) { while (e.firstChild) e.removeChild(e.firstChild); }
  function liberer() { urls.forEach(function (u) { URL.revokeObjectURL(u); }); urls = []; }
  function montrer(e, oui) { e.classList.toggle("cache", !oui); }

  function afficherListe() {
    liberer();
    vider(liste);
    var f = entree.files, total = 0;
    for (var i = 0; i < f.length; i++) {
      var g = genre(f[i]);
      var ligne = el("div", "fichier");
      var v = el("span", "vignette " + g);
      if (g === "image" && f[i].size < 30e6) {
        var img = new Image(), u = URL.createObjectURL(f[i]);
        urls.push(u);
        img.alt = "";
        img.onerror = function () { var p = this.parentNode; p.removeChild(this); p.appendChild(icone("image")); };
        img.src = u;
        v.appendChild(img);
      } else {
        v.appendChild(icone(g));
      }
      var infos = el("span", "infos");
      infos.appendChild(el("span", "nom", f[i].name));
      infos.appendChild(el("span", "sous", taille(f[i].size)));
      ligne.appendChild(v);
      ligne.appendChild(infos);
      ligne.appendChild(el("span", "etat-f"));
      liste.appendChild(ligne);
      total += f[i].size;
    }
    if (f.length > 1) liste.appendChild(el("div", "resume", f.length + " fichiers · " + taille(total)));
    montrer(liste, f.length > 0);
    majBouton();
    return total;
  }

  function majBouton() {
    var n = entree.files.length, t = texte.value.trim();
    bouton.disabled = !n && !t;
    bouton.textContent = n > 1 ? "Envoyer les " + n + " fichiers" : n === 1 ? "Envoyer" : t ? "Envoyer le texte" : "Envoyer";
  }
  texte.addEventListener("input", function () { montrer(erreur, false); majBouton(); });

  // Un lien ou un texte : il arrive dans le presse-papiers du PC (Ctrl+V).
  function envoyerTexte(t) {
    return fetch(location.pathname, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ texte: t }) })
      .then(function (r) { return r.json().then(function (j) { if (!r.ok) throw new Error(j.erreur || "Envoi refusé"); return j; }); });
  }
  function finTexte(t, j) {
    montrer(zone, false); montrer(liste, false); montrer(prog, false);
    finiTexte.textContent = (j.lien ? "Lien" : "Texte") + (j.copie ? " copié dans le presse-papiers du PC : Ctrl+V pour le coller." : " reçu par le PC.");
    montrer(fini, true); montrer(bouton, false); montrer(encore, true);
    texte.value = ""; montrer(texte, false);
  }

  entree.addEventListener("change", function () {
    montrer(erreur, false);
    afficherListe();
  });

  bouton.addEventListener("click", function () {
    var f = entree.files, t = texte.value.trim();
    if (!f.length && t) {
      bouton.disabled = true;
      envoyerTexte(t).then(function (j) { finTexte(t, j); }).catch(function (e) { echec(e.message); });
      return;
    }
    if (!f.length) return;
    var form = new FormData();
    for (var i = 0; i < f.length; i++) form.append("fichier", f[i], f[i].name);
    bouton.disabled = true;
    bouton.textContent = "Envoi en cours…";
    montrer(zone, false);
    montrer(erreur, false);
    montrer(prog, true);
    pourcent.textContent = "0 %";
    plein.style.width = "0";
    detail.textContent = "Connexion au PC…";

    var xhr = new XMLHttpRequest(), debut = Date.now();
    xhr.upload.onprogress = function (e) {
      if (!e.lengthComputable) return;
      var x = e.loaded / e.total, s = (Date.now() - debut) / 1000;
      var debit = s > 0 ? 8 * e.loaded / s / 1e6 : 0;
      pourcent.textContent = Math.floor(100 * x) + " %";
      plein.style.width = (100 * x) + "%";
      var reste = debit > 0 ? (e.total - e.loaded) * 8 / (debit * 1e6) : NaN;
      detail.textContent = taille(e.loaded) + " sur " + taille(e.total) + " · " + debit.toFixed(0) + " Mbit/s" +
        (x < 1 && reste ? " · encore " + duree(reste) : "");
      if (x >= 1) detail.textContent = "Vérification sur le PC…";
    };
    xhr.onload = function () {
      if (xhr.status === 200) {
        var n = f.length, s = (Date.now() - debut) / 1000;
        var t = texte.value.trim();
        if (t) envoyerTexte(t).catch(function () { /* le texte suit ; son échec n'annule pas les fichiers */ });
        texte.value = "";
        montrer(prog, false);
        montrer(liste, false);
        finiTexte.textContent = (n > 1 ? n + " fichiers envoyés" : "« " + f[0].name + " » envoyé") + " en " + duree(s) + ".";
        montrer(fini, true);
        montrer(bouton, false);
        montrer(encore, true);
        liberer();
      } else {
        var m = "L'envoi a échoué.";
        try { m = JSON.parse(xhr.responseText).erreur || m; } catch (e) { /* réponse non JSON */ }
        echec(m);
      }
    };
    xhr.onerror = function () {
      echec("Connexion perdue. Vérifie que le téléphone est sur le même réseau que le PC, puis réessaie.");
    };
    xhr.open("POST", location.pathname);
    xhr.send(form);
  });

  function echec(m) {
    montrer(prog, false);
    montrer(zone, true);
    erreur.textContent = m;
    montrer(erreur, true);
    bouton.disabled = false;
    bouton.textContent = "Réessayer";
  }

  encore.addEventListener("click", function () {
    entree.value = "";
    vider(liste);
    montrer(liste, false);
    montrer(fini, false);
    montrer(encore, false);
    montrer(zone, true);
    montrer(texte, true);
    montrer(bouton, true);
    majBouton();
  });
})();
