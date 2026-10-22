//go:build !windows

package main

import "github.com/Cams457/universal-airdrop/internal/serveur"

func attacherConsole() {}

// icone : hors Windows, pas d'icône de notification ; on attend Ctrl+C ou « Quitter ».
func icone(*serveur.Service) { attendreArret() }
