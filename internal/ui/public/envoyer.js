// Page d'envoi téléphone -> PC : choisir des fichiers, les envoyer avec une barre de progression.
(function () {
  "use strict";
  var entree = document.getElementById("fichiers");
  var bouton = document.getElementById("envoyer");
  var etat = document.getElementById("etat");

  function taille(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1).replace(".", ",") + " Go";
    if (n >= 1e6) return (n / 1e6).toFixed(1).replace(".", ",") + " Mo";
    return Math.max(1, Math.round(n / 1e3)) + " Ko";
  }

  function vider(el) { while (el.firstChild) el.removeChild(el.firstChild); }

  function ligne(texte, droite, classe) {
    var d = document.createElement("div");
    d.className = "ligne";
    var a = document.createElement("span"); a.textContent = texte;
    var b = document.createElement("span"); b.textContent = droite || "";
    if (classe) b.className = classe;
    d.appendChild(a); d.appendChild(b);
    return d;
  }

  entree.addEventListener("change", function () {
    vider(etat);
    var total = 0;
    for (var i = 0; i < entree.files.length; i++) {
      total += entree.files[i].size;
      etat.appendChild(ligne(entree.files[i].name, taille(entree.files[i].size)));
    }
    bouton.disabled = entree.files.length === 0;
    bouton.textContent = entree.files.length > 1
      ? "Envoyer " + entree.files.length + " fichiers (" + taille(total) + ")"
      : "Envoyer";
  });

  bouton.addEventListener("click", function () {
    var form = new FormData();
    for (var i = 0; i < entree.files.length; i++) form.append("fichier", entree.files[i], entree.files[i].name);
    bouton.disabled = true;
    vider(etat);
    var barre = document.createElement("div"); barre.className = "barre";
    var plein = document.createElement("div"); barre.appendChild(plein);
    var texte = document.createElement("p");
    etat.appendChild(barre); etat.appendChild(texte);
    var xhr = new XMLHttpRequest();
    var debut = Date.now();
    xhr.upload.onprogress = function (e) {
      if (!e.lengthComputable) return;
      plein.style.width = (100 * e.loaded / e.total) + "%";
      var s = (Date.now() - debut) / 1000;
      var debit = s > 0 ? 8 * e.loaded / s / 1e6 : 0;
      texte.textContent = taille(e.loaded) + " sur " + taille(e.total) + " · " + debit.toFixed(0) + " Mbit/s";
    };
    xhr.onload = function () {
      if (xhr.status === 200) {
        plein.style.width = "100%";
        texte.className = "ok";
        texte.textContent = "C'est envoyé ✓";
        entree.value = "";
        bouton.textContent = "Envoyer";
      } else {
        var m = "Échec de l'envoi";
        try { m = JSON.parse(xhr.responseText).erreur || m; } catch (e) { /* réponse non JSON */ }
        texte.className = "erreur";
        texte.textContent = m;
        bouton.disabled = false;
      }
    };
    xhr.onerror = function () {
      texte.className = "erreur";
      texte.textContent = "Connexion perdue. Vérifie que le téléphone est sur le même réseau que le PC.";
      bouton.disabled = false;
    };
    xhr.open("POST", location.pathname);
    xhr.send(form);
  });
})();
