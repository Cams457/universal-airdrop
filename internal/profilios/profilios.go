// Package profilios génère le profil de configuration iOS (.mobileconfig) contenant la CA du PC.
//
// Une fois le profil installé ET la confiance totale activée (Réglages > Général > Informations >
// Réglages des certificats), Safari et les Raccourcis acceptent le HTTPS du PC. Il faut vérifier
// l'empreinte SHA-256 affichée par iOS : c'est ce qui garantit que personne n'a substitué sa CA.
package profilios

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"html"
	"strings"
)

func uuidDe(graine string) string {
	s := sha1.Sum([]byte(graine))
	s[6] = (s[6] & 0x0f) | 0x50 // version 5
	s[8] = (s[8] & 0x3f) | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", s[0:4], s[4:6], s[6:8], s[8:10], s[10:16]))
}

// Generer renvoie le profil XML.
func Generer(derCA []byte, nomPC, empreinte string) []byte {
	e := html.EscapeString
	court := empreinte[:16]
	b64 := base64.StdEncoding.EncodeToString(derCA)
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadContent</key>
			<data>` + b64 + `</data>
			<key>PayloadDisplayName</key>
			<string>` + e("Universal Airdrop CA - "+nomPC) + `</string>
			<key>PayloadIdentifier</key>
			<string>local.universal-airdrop.` + court + `.ca</string>
			<key>PayloadType</key>
			<string>com.apple.security.root</string>
			<key>PayloadUUID</key>
			<string>` + uuidDe("ca-"+empreinte) + `</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
	</array>
	<key>PayloadDescription</key>
	<string>Autorité de certification locale de ce PC, pour envoyer des fichiers en HTTPS.</string>
	<key>PayloadDisplayName</key>
	<string>` + e("Universal Airdrop - "+nomPC) + `</string>
	<key>PayloadIdentifier</key>
	<string>local.universal-airdrop.` + court + `</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>` + uuidDe("profil-"+empreinte) + `</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`)
}
