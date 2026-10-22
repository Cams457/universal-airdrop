// Guide de construction des Raccourcis iPhone, partagé par l'interface du PC et l'app iPhone.
// Données + petits outils d'affichage (texte avec `code`, **gras**, *italique*, sans HTML injecté).
(function () {
  "use strict";
  function el(tag, attrs) {
    var e = document.createElement(tag);
    if (attrs) for (var k in attrs) {
      if (k === "texte") e.textContent = attrs[k];
      else if (k === "classe") e.className = attrs[k];
      else e.setAttribute(k, attrs[k]);
    }
    for (var i = 2; i < arguments.length; i++) if (arguments[i]) e.appendChild(arguments[i]);
    return e;
  }
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
  function encadre(lignes) { var d = el("div", { classe: "encadre" }); lignes.forEach(function (t) { d.appendChild(el("p", null, riche(t))); }); return d; }
  function tableActions(lignes) {
    var t = el("table", { classe: "actions-raccourci" }, el("tr", null, el("th", { texte: "#" }), el("th", { texte: "Action à ajouter" }), el("th", { texte: "Réglage" })));
    lignes.forEach(function (l, i) {
      t.appendChild(el("tr", null, el("td", { texte: String(i + 1) }),
        el("td", { classe: l[2] ? "retrait" : "" }, riche((l[2] ? "↳ ".repeat(l[2]) : "") + "**" + l[0] + "**")),
        el("td", null, riche(l[1]))));
    });
    return t;
  }

  var REGLAGES = [
    "**Réglages G** (touche « Afficher plus » sur l'action) : méthode **GET** ; en-tête `Authorization` = `Bearer ` (avec l'espace) suivi de la variable `Jeton`.",
    "**Réglages L** (lien ou texte) : méthode **POST** ; en-têtes `Authorization` = `Bearer ` + `Jeton` et `X-Destinations` = `Destinations` ; corps **JSON** avec un champ Texte de clé `texte` et de valeur *Élément répété*.",
    "**Réglages F** (fichier) : méthode **POST** ; mêmes en-têtes ; corps **Formulaire** avec un champ **Fichier** de clé `fichier` et de valeur *Élément répété*."
  ];
  var ENVOYER_DEBUT = "App **Raccourcis** › **+** › renomme-le `Envoyer`. Touche **(i)** : active **Afficher dans la feuille de partage** ; types acceptés : *Images, Médias, Fichiers, URL, Texte* ; si aucune entrée : **Arrêter et répondre**.";
  var ENVOYER = [
    ["Texte", "colle l'**adresse du PC**"],
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
    ["Afficher une notification", "`Envoyé ✓`"]];
  var RECEVOIR_DEBUT = "**+** › renomme-le `Recevoir` (pas besoin de la feuille de partage). Les actions 1 à 4 sont les mêmes que pour Envoyer.";
  var RECEVOIR = [
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
    ["Afficher une notification", "*Valeur du dictionnaire*"]];
  var AUTOMATIQUE = [
    "**En arrivant à la maison** : Raccourcis › **Automatisation** › **+** › **Wi-Fi** › ton réseau › **Se connecte** › **Exécuter immédiatement** (désactive *Me notifier*) › **Suivant** › *Recevoir*.",
    "**D'un double tapotement au dos** : Réglages › Accessibilité › Toucher › **Toucher le dos** › Toucher deux fois › *Recevoir*.",
    "**Depuis l'écran d'accueil** : appui long › **+** › Raccourcis › *Recevoir*.",
    "iPhone 15 Pro et plus récents : le **bouton Action** peut lancer *Recevoir*."];

  window.UAGuide = {
    el: el, riche: riche, pas: pas, encadre: encadre, tableActions: tableActions,
    envoyer: function () { return [encadre([ENVOYER_DEBUT]), tableActions(ENVOYER), encadre(REGLAGES)]; },
    recevoir: function () { return [encadre([RECEVOIR_DEBUT]), tableActions(RECEVOIR)]; },
    automatique: function () { return [pas(AUTOMATIQUE)]; }
  };
})();
