// Package ui : interface graphique locale (servie sur 127.0.0.1) et pages pour les téléphones
// (servies en HTTPS sur le réseau local). Tout est embarqué dans l'exécutable.
package ui

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed interface/* public/*
var fichiers embed.FS

const cspInterface = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
const cspPublic = "default-src 'none'; img-src 'self' data: blob:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

func avecCSP(csp string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		h.ServeHTTP(w, r)
	})
}

// Interface : l'application locale (index.html, app.js, app.css).
func Interface() http.Handler {
	sous, _ := fs.Sub(fichiers, "interface")
	return avecCSP(cspInterface, http.FileServer(http.FS(sous)))
}

// Public : feuilles de style et scripts des pages pour téléphone.
func Public() http.Handler {
	sous, _ := fs.Sub(fichiers, "public")
	return avecCSP(cspPublic, http.FileServer(http.FS(sous)))
}

var pages = template.Must(template.New("").Funcs(template.FuncMap{"genre": genre}).Parse(`
{{define "debut"}}<!doctype html><html lang="fr"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#f2f2f7" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#000000" media="(prefers-color-scheme: dark)">
<meta name="format-detection" content="telephone=no">
<title>{{.}}</title><link rel="icon" href="/statique/icone.png"><link rel="apple-touch-icon" href="/statique/icone.png">
<link rel="stylesheet" href="/statique/page.css"></head><body><main>{{end}}
{{define "fin"}}<footer class="pied"><img src="/statique/icone.png" alt="" width="16" height="16">Universal Airdrop · connexion chiffrée, sur ton réseau local</footer></main></body></html>{{end}}

{{define "entete"}}<header class="entete"><img class="appli" src="/statique/icone.png" alt="" width="64" height="64">
<p class="surtitre">{{.Surtitre}}</p><h1>{{.Titre}}</h1>
{{if .PC}}<p class="puce">{{template "i-pc"}}<span>{{.PC}}</span></p>{{end}}{{if .Sous}}<p class="sous-titre">{{.Sous}}</p>{{end}}</header>{{end}}

{{define "i-pc"}}<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/></svg>{{end}}
{{define "i-envoi"}}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15V3M7 8l5-5 5 5"/><path d="M5 13v5a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-5"/></svg>{{end}}
{{define "i-recu"}}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3v12M7 10l5 5 5-5"/><path d="M5 17v1a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-1"/></svg>{{end}}
{{define "i-image"}}<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="9" cy="10" r="2"/><path d="M21 16l-5-5-9 9"/></svg>{{end}}
{{define "i-video"}}<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="5" width="13" height="14" rx="3"/><path d="M16 10l5-3v10l-5-3"/></svg>{{end}}
{{define "i-pdf"}}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h4"/></svg>{{end}}
{{define "i-fichier"}}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/></svg>{{end}}
{{define "i-lien"}}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M10 14a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 10a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/></svg>{{end}}
{{define "i-cle"}}<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M17 6l3 3M15 8l2 2"/></svg>{{end}}
{{define "i-sablier"}}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h12M6 21h12M7 3c0 5 10 5 10 9s-10 4-10 9M17 3c0 5-10 5-10 9"/></svg>{{end}}
{{define "i-bouclier"}}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/><path d="M8.5 12l2.5 2.5 4.5-5"/></svg>{{end}}
{{define "i-ok"}}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>{{end}}
{{define "i-chevron"}}<svg class="chevron" viewBox="0 0 24 24" aria-hidden="true"><path d="M9 6l6 6-6 6"/></svg>{{end}}
{{define "icone-type"}}{{if eq . "image"}}{{template "i-image"}}{{else if eq . "video"}}{{template "i-video"}}{{else if eq . "pdf"}}{{template "i-pdf"}}{{else}}{{template "i-fichier"}}{{end}}{{end}}

{{define "message"}}{{template "debut" .Titre}}<section class="vide"><div class="rond doux">{{template "i-sablier"}}</div>
<h1>{{.Titre}}</h1><p>{{.Texte}}</p></section>{{template "fin"}}{{end}}

{{define "texte"}}{{template "debut" "Jeton d'accès"}}{{template "entete" .Entete}}
<section class="carte"><div class="carte-titre">{{template "i-cle"}}<span>À coller dans le Raccourci</span></div>
<pre class="texte" id="texte">{{.Texte}}</pre></section>
<button class="bouton principal" id="copier" type="button" data-cible="texte">Copier</button>
<p class="note">Cette page ne s'affiche qu'une fois. Si « Copier » ne marche pas : appui long sur le texte, puis « Copier ».</p>
<script src="/statique/page.js"></script>{{template "fin"}}{{end}}

{{define "partage"}}{{template "debut" "Fichiers reçus"}}{{template "entete" .Entete}}
{{if .URL}}<a class="carte ligne-lien" href="{{.URL}}" rel="noopener noreferrer"><span class="vignette lien">{{template "i-lien"}}</span>
<span class="infos"><span class="nom">Ouvrir le lien</span><span class="sous">{{.URL}}</span></span>{{template "i-chevron"}}</a>{{end}}
{{if .Fichiers}}<section class="carte liste">{{range .Fichiers}}<a class="ligne-lien" href="{{.URL}}">
<span class="vignette {{genre .Nom}}">{{template "icone-type" (genre .Nom)}}</span>
<span class="infos"><span class="nom">{{.Nom}}</span><span class="sous">{{.Taille}}</span></span>
<span class="pastille">{{template "i-recu"}}</span></a>{{end}}</section>
<p class="note">Touche un fichier, puis « Télécharger ». Il arrive dans l'app Fichiers ; pour une photo ou une vidéo, ouvre-la puis
<b>Partager → Enregistrer</b> pour l'ajouter à Photos.</p>{{end}}{{template "fin"}}{{end}}

{{define "envoi"}}{{template "debut" "Envoyer au PC"}}{{template "entete" .}}
<label class="zone" id="zone"><input type="file" id="fichiers" multiple>
<span class="rond">{{template "i-envoi"}}</span>
<span class="grand">Choisir des photos ou des fichiers</span>
<span class="sous">Photos, vidéos, PDF… plusieurs à la fois</span></label>
<section class="carte liste cache" id="liste"></section>
<section class="carte progression cache" id="progression"><div class="pourcent" id="pourcent">0 %</div>
<div class="barre"><div id="plein"></div></div><p class="sous" id="detail"></p></section>
<section class="vide cache" id="fini"><div class="rond ok">{{template "i-ok"}}</div><h2>C'est sur le PC</h2>
<p id="fini-texte"></p></section>
<p class="erreur cache" id="erreur" role="alert"></p>
<div class="bas"><button class="bouton principal" id="envoyer" type="button" disabled>Envoyer</button>
<button class="bouton secondaire cache" id="encore" type="button">Envoyer d'autres fichiers</button></div>
<script src="/statique/envoyer.js"></script>{{template "fin"}}{{end}}

{{define "iphone"}}{{template "debut" "Installer le certificat"}}{{template "entete" .Entete}}
<p class="intro">Une seule fois : ton iPhone apprend à reconnaître ce PC, pour que tes envois soient chiffrés et que personne ne puisse se faire passer pour lui.</p>
<ol class="etapes">
<li class="carte"><span class="num">1</span><div><h2>Télécharger le profil</h2>
<p>Puis touche <b>Autoriser</b> dans la fenêtre qui s'affiche.</p>
<a class="bouton principal" href="/profil.mobileconfig">Télécharger le profil</a></div></li>
<li class="carte"><span class="num">2</span><div><h2>L'installer</h2>
<p class="chemin"><span>Réglages</span> <span><i>›</i> Profil téléchargé</span> <span><i>›</i> Installer</span></p>
<p class="sous">Absent en haut de Réglages ? Va dans <b>Général → VPN et gestion de l'appareil</b>. iOS efface un profil non installé au bout de 8 minutes : retélécharge-le si besoin.</p></div></li>
<li class="carte"><span class="num">3</span><div><h2>L'activer</h2>
<p class="chemin"><span>Réglages</span> <span><i>›</i> Général</span> <span><i>›</i> Informations</span> <span><i>›</i> Réglages des certificats</span></p>
<p>Active <b>{{.NomCA}}</b>. Cette ligne n'apparaît qu'une fois le profil installé.</p></div></li>
<li class="carte"><span class="num">4</span><div><h2>Relier cet iPhone</h2>
<p>Donne-lui un nom, puis accepte sur le PC après avoir vérifié le code.</p>
<form id="relier" class="relier"><input id="nom-iphone" type="text" value="iPhone" maxlength="40" autocomplete="off" aria-label="Nom de l'iPhone">
<button class="bouton principal" type="submit">Relier cet iPhone</button></form>
<div id="attente" class="cache"><p class="sous">Sur le PC, une fenêtre s'est ouverte. Vérifie que le code est le même, puis touche <b>Accepter</b> :</p>
<p class="code" id="code"></p><p class="sous" id="attente-etat">En attente du PC…</p></div>
<div id="relie" class="cache"><p class="ok-texte">Cet iPhone est relié à {{.Entete.PC}}. Copie ces deux valeurs : les Raccourcis te les demanderont à l'installation.</p>
<div class="valeur"><span class="etiquette">Adresse du PC</span><code id="adresse"></code><button class="bouton secondaire petit" type="button" data-copier="adresse">Copier</button></div>
<div class="valeur"><span class="etiquette">Jeton (secret)</span><code id="jeton"></code><button class="bouton secondaire petit" type="button" data-copier="jeton">Copier</button></div>
<p class="sous">Ce jeton ne s'affiche qu'une fois. Garde-le seulement dans tes Raccourcis.</p></div>
<p class="erreur cache" id="relier-erreur" role="alert"></p></div></li>
</ol>
<section class="carte"><div class="carte-titre">{{template "i-bouclier"}}<span>Vérifie l'empreinte</span></div>
<p class="sous">Dans le profil installé : <b>Plus de détails</b> → le certificat → <b>SHA-256</b>. Elle doit être identique à celle-ci et à celle affichée sur le PC :</p>
<div class="empreinte">{{range .Empreinte}}<span>{{.}}</span>{{end}}</div></section>
<p class="note">Ouvre cette page dans <b>Safari</b> : les autres navigateurs ne savent pas installer de profil.</p>
<script src="/statique/iphone.js"></script>{{template "fin"}}{{end}}
`))

type FichierPartage struct{ Nom, Taille, URL string }

type entete struct{ Surtitre, Titre, PC, Sous string }

// genre classe un fichier par son extension, pour choisir l'icône.
func genre(nom string) string {
	e := strings.ToLower(path.Ext(nom))
	switch e {
	case ".jpg", ".jpeg", ".png", ".heic", ".heif", ".gif", ".webp", ".bmp", ".tif", ".tiff":
		return "image"
	case ".mov", ".mp4", ".m4v", ".avi", ".mkv", ".webm", ".3gp":
		return "video"
	case ".pdf":
		return "pdf"
	}
	return "fichier"
}

func entetesPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", cspPublic)
	w.Header().Set("X-Frame-Options", "DENY")
}

func PageMessage(w http.ResponseWriter, titre, texte string) {
	entetesPage(w)
	_ = pages.ExecuteTemplate(w, "message", map[string]string{"Titre": titre, "Texte": texte})
}

func PageTexte(w http.ResponseWriter, pc, texte string) {
	entetesPage(w)
	_ = pages.ExecuteTemplate(w, "texte", map[string]any{
		"Entete": entete{Surtitre: "Ajout de ton iPhone", Titre: "Ton jeton d'accès", PC: pc}, "Texte": texte})
}

func PagePartage(w http.ResponseWriter, pc, url string, f []FichierPartage) {
	entetesPage(w)
	titre := "Un lien pour toi"
	switch {
	case len(f) == 1:
		titre = "Un fichier pour toi"
	case len(f) > 1:
		titre = fmt.Sprintf("%d fichiers pour toi", len(f))
	}
	_ = pages.ExecuteTemplate(w, "partage", map[string]any{
		"Entete": entete{Surtitre: "Envoyé depuis", Titre: titre, PC: pc}, "URL": template.URL(url), "Fichiers": f})
}

func PageEnvoi(w http.ResponseWriter, pc string) {
	entetesPage(w)
	_ = pages.ExecuteTemplate(w, "envoi", entete{Surtitre: "Envoyer des fichiers vers", Titre: pc})
}

// PageIphone : guide d'installation du certificat du PC sur l'iPhone.
// empreinte au format « AB CD … » (32 octets), affichée par groupes de 8 octets comme dans iOS.
func PageIphone(w http.ResponseWriter, pc, empreinte string) {
	entetesPage(w)
	octets := strings.Fields(empreinte)
	var lignes []string
	for i := 0; i < len(octets); i += 8 {
		lignes = append(lignes, strings.Join(octets[i:min(i+8, len(octets))], " "))
	}
	_ = pages.ExecuteTemplate(w, "iphone", map[string]any{
		"Entete": entete{Surtitre: "Ajouter ton iPhone", Titre: "Faire confiance à ce PC", PC: pc},
		"NomCA":  "Universal Airdrop CA - " + pc, "Empreinte": lignes})
}
