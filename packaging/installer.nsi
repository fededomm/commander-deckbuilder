; Installer Windows per utente singolo: niente UAC, niente Program Files.
; Si costruisce da Linux con `makensis -DVERSION=x.y.z packaging/installer.nsi`.
Unicode true
!include "MUI2.nsh"

!define APP     "Commander Deckbuilder"
!define EXE     "deckbuilder.exe"
!define REGAPP  "Software\Commander Deckbuilder"
!define REGUNIN "Software\Microsoft\Windows\CurrentVersion\Uninstall\CommanderDeckbuilder"
!ifndef VERSION
  !define VERSION "0.0.0"
!endif

Name "${APP} ${VERSION}"
OutFile "..\dist\CommanderDeckbuilder-${VERSION}-windows-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\Commander Deckbuilder"
InstallDirRegKey HKCU "${REGAPP}" "InstallDir"
RequestExecutionLevel user   ; installa solo per l'utente corrente: nessun prompt di amministratore
SetCompressor /SOLID lzma

!define MUI_ICON   "icon.ico"
!define MUI_UNICON "icon.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Avvia ${APP}"

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Italian"

; L'app è un server: dopo aver chiuso la scheda del browser resta in esecuzione.
; Windows non lascia cancellare un eseguibile in uso, quindi senza questo la
; disinstallazione fallisce in silenzio e lascia cartella, exe e app viva.
!macro StopApp
  DetailPrint "Chiudo ${APP} se è in esecuzione..."
  nsExec::Exec 'taskkill /F /IM "${EXE}"'
  Pop $0
  Sleep 700
!macroend

Section "Install"
  !insertmacro StopApp   ; anche in aggiornamento: l'exe vecchio va liberato
  SetOutPath "$INSTDIR"
  File "..\dist\windows\${EXE}"
  File "icon.ico"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateDirectory "$SMPROGRAMS\${APP}"
  CreateShortcut "$SMPROGRAMS\${APP}\${APP}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\icon.ico"
  CreateShortcut "$SMPROGRAMS\${APP}\Disinstalla ${APP}.lnk" "$INSTDIR\uninstall.exe"

  WriteRegStr   HKCU "${REGAPP}"   "InstallDir"      "$INSTDIR"
  WriteRegStr   HKCU "${REGUNIN}"  "DisplayName"     "${APP}"
  WriteRegStr   HKCU "${REGUNIN}"  "DisplayVersion"  "${VERSION}"
  WriteRegStr   HKCU "${REGUNIN}"  "DisplayIcon"     "$INSTDIR\icon.ico"
  WriteRegStr   HKCU "${REGUNIN}"  "InstallLocation" "$INSTDIR"
  WriteRegStr   HKCU "${REGUNIN}"  "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "${REGUNIN}"  "NoModify" 1
  WriteRegDWORD HKCU "${REGUNIN}"  "NoRepair" 1
SectionEnd

Section "Uninstall"
  !insertmacro StopApp

  ; /REBOOTOK: se qualcosa è ancora bloccato, Windows lo toglie al riavvio
  ; invece di lasciarlo lì per sempre.
  Delete /REBOOTOK "$INSTDIR\${EXE}"
  Delete /REBOOTOK "$INSTDIR\icon.ico"
  Delete /REBOOTOK "$INSTDIR\uninstall.exe"
  RMDir /r /REBOOTOK "$INSTDIR"

  Delete "$SMPROGRAMS\${APP}\${APP}.lnk"
  Delete "$SMPROGRAMS\${APP}\Disinstalla ${APP}.lnk"
  RMDir  "$SMPROGRAMS\${APP}"

  DeleteRegKey HKCU "${REGUNIN}"
  DeleteRegKey HKCU "${REGAPP}"

  ; I mazzi stanno in %APPDATA% e sopravvivono alla disinstallazione:
  ; li cancello solo se me lo chiede esplicitamente.
  MessageBox MB_YESNO|MB_ICONQUESTION \
    "Vuoi cancellare anche i mazzi salvati?$\n$\n$APPDATA\commander-deckbuilder" \
    /SD IDNO IDNO keep
  RMDir /r "$APPDATA\commander-deckbuilder"
  keep:
SectionEnd
