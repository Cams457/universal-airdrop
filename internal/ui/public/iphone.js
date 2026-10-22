// Page /iphone, étape 4 : l'iPhone demande l'accès ; le PC affiche le même code ; une fois accepté,
// la page affiche l'adresse du PC et le jeton à coller dans les Raccourcis.
(function () {
  "use strict";
  function $(id) { return document.getElementById(id); }
  var form = $("relier");
  if (!form) return;
  function montrer(id, oui) { $(id).classList.toggle("cache", !oui); }
  function erreur(m) { $("relier-erreur").textContent = m; montrer("relier-erreur", true); montrer("attente", false); montrer("relier", true); }

  function copier(b) {
    var t = $(b.getAttribute("data-copier")).textContent;
    var fait = function () { b.textContent = "Copié ✓"; setTimeout(function () { b.textContent = "Copier"; }, 2000); };
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(t).then(fait, function () { selectionner(b); });
    } else selectionner(b);
  }
  function selectionner(b) {
    var r = document.createRange();
    r.selectNodeContents($(b.getAttribute("data-copier")));
    var s = window.getSelection(); s.removeAllRanges(); s.addRange(r);
    try { if (document.execCommand("copy")) { b.textContent = "Copié ✓"; } } catch (e) { /* appui long */ }
  }
  document.querySelectorAll("[data-copier]").forEach(function (b) { b.addEventListener("click", function () { copier(b); }); });

  function suivre(id) {
    fetch("/api/v1/telephone/demande/" + encodeURIComponent(id)).then(function (r) {
      if (r.status === 404) throw new Error("La demande a expiré (5 minutes). Recommence.");
      return r.json();
    }).then(function (d) {
      if (d.etat === "attente") { setTimeout(function () { suivre(id); }, 1500); return; }
      if (d.etat === "refusee") { erreur("Refusé sur le PC. Si c'est une erreur, recommence."); return; }
      $("adresse").textContent = d.adresse;
      $("jeton").textContent = d.jeton;
      montrer("attente", false);
      montrer("relie", true);
    }).catch(function (e) { erreur(e.message); });
  }

  form.addEventListener("submit", function (e) {
    e.preventDefault();
    montrer("relier-erreur", false);
    var nom = $("nom-iphone").value.trim() || "iPhone";
    fetch("/api/v1/telephone/demande", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ nom: nom }) })
      .then(function (r) { return r.json().then(function (j) { if (!r.ok) throw new Error(j.erreur || "Demande refusée"); return j; }); })
      .then(function (d) {
        $("code").textContent = d.code.slice(0, 3) + " " + d.code.slice(3);
        montrer("relier", false);
        montrer("attente", true);
        suivre(d.id);
      }).catch(function (e) { erreur(e.message || "PC injoignable. Même Wi-Fi ?"); });
  });
})();
