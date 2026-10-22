// Bouton « Copier » de la page du jeton (repli : sélection du texte pour un appui long).
(function () {
  "use strict";
  var b = document.getElementById("copier");
  if (!b) return;
  var cible = document.getElementById(b.getAttribute("data-cible"));
  function selectionner() {
    var r = document.createRange();
    r.selectNodeContents(cible);
    var s = window.getSelection();
    s.removeAllRanges();
    s.addRange(r);
  }
  function fait() {
    b.textContent = "Copié ✓";
    b.classList.add("ok");
    setTimeout(function () { b.textContent = "Copier"; b.classList.remove("ok"); }, 2500);
  }
  b.addEventListener("click", function () {
    var t = cible.textContent;
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(t).then(fait, function () { selectionner(); try { if (document.execCommand("copy")) fait(); } catch (e) { /* appui long */ } });
    } else {
      selectionner();
      try { if (document.execCommand("copy")) fait(); } catch (e) { /* appui long */ }
    }
  });
})();
