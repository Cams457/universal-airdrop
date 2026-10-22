@echo off
rem Règles de pare-feu pour Transfert. A RELIRE avant de lancer, dans une invite cmd EN ADMINISTRATEUR.
rem profile=any + remoteip=localsubnet : autorise tous les profils (l'interface du point d'accès mobile
rem est souvent "Public"), mais uniquement depuis les sous-réseaux directement connectés au PC.
rem Rien n'est ouvert à Internet. Pour annuler : pare_feu.cmd supprimer

net session >nul 2>&1
if errorlevel 1 (
  echo Lance cette invite cmd en administrateur : clic droit sur "Invite de commandes" ^> Executer en tant qu'administrateur.
  exit /b 1
)

if /i "%1"=="supprimer" (
  netsh advfirewall firewall delete rule name="Transfert"
  netsh advfirewall firewall delete rule name="Transfert decouverte"
  exit /b 0
)

netsh advfirewall firewall add rule name="Transfert" dir=in action=allow protocol=TCP localport=47800 remoteip=localsubnet profile=any
netsh advfirewall firewall add rule name="Transfert decouverte" dir=in action=allow protocol=UDP localport=47801 remoteip=localsubnet profile=any
echo Regles ajoutees.
