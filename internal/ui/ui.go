// Package ui : interface graphique locale (servie sur 127.0.0.1) et pages pour les téléphones
// (servies en HTTPS sur le réseau local). Tout est embarqué dans l'exécutable.
package ui

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed interface/* public/*
var fichiers embed.FS

const cspInterface = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
const cspPublic = "default-src 'none'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

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

var pages = template.Must(template.New("").Parse(`
{{define "debut"}}<!doctype html><html lang="fr"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.}}</title>
<link rel="stylesheet" href="/statique/page.css"></head><body><main>{{end}}
{{define "fin"}}<p class="pied">Universal Airdrop</p></main></body></html>{{end}}

{{define "message"}}{{template "debut" .Titre}}<h1>{{.Titre}}</h1><p>{{.Texte}}</p>{{template "fin"}}{{end}}

{{define "texte"}}{{template "debut" "Jeton d'accès"}}<h1>Depuis {{.PC}}</h1>
<p>Appui long sur le texte, puis « Copier ». Cette page ne s'affichera qu'une fois.</p>
<pre class="texte">{{.Texte}}</pre>{{template "fin"}}{{end}}

{{define "partage"}}{{template "debut" "Fichiers partagés"}}<h1>Depuis {{.PC}}</h1>
{{if .URL}}<a class="bouton" href="{{.URL}}" rel="noopener noreferrer">Ouvrir le lien</a><p class="detail">{{.URL}}</p>{{end}}
{{range .Fichiers}}<a class="bouton" href="{{.URL}}">{{.Nom}}</a><p class="detail">{{.Taille}}</p>{{end}}
<p class="aide">Sur iPhone : touche le fichier, puis « Télécharger ». Il arrive dans l'app Fichiers ;
pour une photo ou une vidéo, ouvre-la puis « Enregistrer » pour l'ajouter à Photos.</p>{{template "fin"}}{{end}}

{{define "envoi"}}{{template "debut" "Envoyer au PC"}}<h1>Envoyer à {{.}}</h1>
<label class="zone" id="zone"><input type="file" id="fichiers" multiple>
<span class="grand">Choisir des photos ou des fichiers</span><span class="detail">Plusieurs à la fois si tu veux</span></label>
<button class="bouton principal" id="envoyer" disabled>Envoyer</button>
<div id="etat" class="etat" aria-live="polite"></div>
<script src="/statique/envoyer.js"></script>{{template "fin"}}{{end}}
`))

type FichierPartage struct{ Nom, Taille, URL string }

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
	_ = pages.ExecuteTemplate(w, "texte", map[string]string{"PC": pc, "Texte": texte})
}

func PagePartage(w http.ResponseWriter, pc, url string, f []FichierPartage) {
	entetesPage(w)
	_ = pages.ExecuteTemplate(w, "partage", map[string]any{"PC": pc, "URL": template.URL(url), "Fichiers": f})
}

func PageEnvoi(w http.ResponseWriter, pc string) {
	entetesPage(w)
	_ = pages.ExecuteTemplate(w, "envoi", pc)
}
