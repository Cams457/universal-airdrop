// Pages de QR code (envoi, partage, guide) :
// - en mode « écran d'accueil », une icône créée depuis l'une d'elles ouvre l'app complète ;
// - la page d'envoi (/envoyer/…) scannée par un iPhone déjà relié à l'app (même adresse) ouvre
//   directement l'écran « Envoyer » de l'app. Les autres téléphones gardent la page simple.
(function () {
  "use strict";
  var autonome = window.navigator.standalone === true || (window.matchMedia && window.matchMedia("(display-mode: standalone)").matches);
  if (location.pathname.indexOf("/app") === 0) return;
  if (autonome) return location.replace("/app");
  if (location.pathname.indexOf("/envoyer/") !== 0) return;
  var jeton = "";
  try { jeton = localStorage.getItem("ua-jeton") || ""; } catch (e) { /* stockage indisponible */ }
  if (jeton) return location.replace("/app#v-envoyer");
  // Mémoire de la page effacée par iOS : le PC a peut-être gardé le lien (cookie de l'app).
  if (window.fetch) fetch("/api/rapide/reprendre", { credentials: "same-origin" }).then(function (r) {
    return r.ok ? r.json() : null;
  }).then(function (x) {
    if (!x || !x.jeton) return;
    try { localStorage.setItem("ua-jeton", x.jeton); } catch (e) { /* rien */ }
    location.replace("/app#v-envoyer");
  }, function () { /* page simple */ });
})();
