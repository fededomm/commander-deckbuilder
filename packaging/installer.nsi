; Installer Windows per utente singolo: niente UAC, niente Program Files.
; Si costruisce da Linux con `makensis -DVERSION=x.y.z packaging/installer.nsi`.
Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"

!define APP     "Commander Deckbuilder"
!define EXE     "deckbuilder.exe"
!define REGAPP  "Software\Commander Deckbuilder"
!define REGUNIN "Software\Microsoft\Windows\CurrentVersion\Uninstall\CommanderDeckbuilder"
!define URL     "https://github.com/fededomm/commander-deckbuilder"
!ifndef VERSION
  !define VERSION "0.0.0"
!endif

Name "${APP} ${VERSION}"
OutFile "..\dist\CommanderDeckbuilder-${VERSION}-windows-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\Commander Deckbuilder"
InstallDirRegKey HKCU "${REGAPP}" "InstallDir"
RequestExecutionLevel user   ; installa solo per l'utente corrente: nessun prompt di amministratore
SetCompressor /SOLID lzma
BrandingText "${APP} ${VERSION}"

!define MUI_ICON   "icon.ico"
!define MUI_UNICON "icon.ico"
!define MUI_ABORTWARNING   ; "Vuoi davvero annullare?" invece di chiudere al primo clic
!define MUI_COMPONENTSPAGE_SMALLDESC

; Il wizard: benvenuto → cartella → componenti → installazione → fine.
!define MUI_WELCOMEPAGE_TITLE "Installazione di ${APP} ${VERSION}"
!define MUI_WELCOMEPAGE_TEXT "${APP} ti aiuta a costruire mazzi Commander: cerchi le carte su Scryfall, spunti quelle che hai comprato e tieni d'occhio i prezzi Cardmarket.$\r$\n$\r$\nL'app si apre nel browser e salva i mazzi sul tuo computer. Non servono permessi di amministratore.$\r$\n$\r$\nFai clic su Avanti per continuare."
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Avvia ${APP}"
!define MUI_FINISHPAGE_LINK "Pagina del progetto su GitHub"
!define MUI_FINISHPAGE_LINK_LOCATION "${URL}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
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

Section "${APP}" SecApp
  SectionIn RO   ; l'app stessa non si può deselezionare
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
  WriteRegStr   HKCU "${REGUNIN}"  "Publisher"       "Federico Domesi"
  WriteRegStr   HKCU "${REGUNIN}"  "URLInfoAbout"    "${URL}"
  WriteRegDWORD HKCU "${REGUNIN}"  "NoModify" 1
  WriteRegDWORD HKCU "${REGUNIN}"  "NoRepair" 1
  ; la dimensione in "App installate", in KB
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${REGUNIN}"  "EstimatedSize" "$0"
SectionEnd

Section "Collegamento sul desktop" SecDesktop
  CreateShortcut "$DESKTOP\${APP}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\icon.ico"
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecApp}     "L'applicazione e il collegamento nel menu Start."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "Un'icona sul desktop per aprire ${APP}."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
  !insertmacro StopApp

  ; /REBOOTOK: se qualcosa è ancora bloccato, Windows lo toglie al riavvio
  ; invece di lasciarlo lì per sempre.
  Delete /REBOOTOK "$INSTDIR\${EXE}"
  Delete /REBOOTOK "$INSTDIR\icon.ico"
  Delete /REBOOTOK "$INSTDIR\uninstall.exe"
  RMDir /r /REBOOTOK "$INSTDIR"

  Delete "$DESKTOP\${APP}.lnk"
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
