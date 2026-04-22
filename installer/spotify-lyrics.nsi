!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "Sections.nsh"
!include "x64.nsh"

; ---------------------------------------------------------------------------
; Defines
; ---------------------------------------------------------------------------
!ifndef VERSION
  !define VERSION "dev"
!endif

Name "Spotify Lyrics for OBS"
OutFile "..\obs-spotify-lyrics-${VERSION}-setup.exe"
Unicode True

RequestExecutionLevel admin

InstallDir "$PROGRAMFILES64\Spotify Lyrics Widget"

; ---------------------------------------------------------------------------
; Variables
; ---------------------------------------------------------------------------
Var OBSPath
Var OBSWasRunning

; ---------------------------------------------------------------------------
; MUI settings
; ---------------------------------------------------------------------------
!define MUI_ABORTWARNING

; ---------------------------------------------------------------------------
; Pages
; ---------------------------------------------------------------------------
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES

; MUI_PAGE_CUSTOMFUNCTION_SHOW must be defined immediately before the page it
; applies to, otherwise it binds to the wrong page.
!define MUI_FINISHPAGE_TEXT " "
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FinishPageShow
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

; ---------------------------------------------------------------------------
; .onInit: architecture guard, OBS detection, running-process checks
; ---------------------------------------------------------------------------
Function .onInit
  ; Require 64-bit Windows; this installer and plugin are amd64-only.
  ${IfNot} ${RunningX64}
    MessageBox MB_OK|MB_ICONSTOP "This installer requires 64-bit Windows (x86_64). 32-bit Windows is not supported."
    Abort
  ${EndIf}

  StrCpy $OBSWasRunning "0"

  ; Read OBS install path from the registry (default value under HKLM\SOFTWARE\OBS Studio).
  SetRegView 64
  ReadRegStr $OBSPath HKLM "SOFTWARE\OBS Studio" ""

  ${If} $OBSPath == ""
    ; 64-bit OBS not found; check whether a 32-bit installation exists.
    SetRegView 32
    ReadRegStr $0 HKLM "SOFTWARE\OBS Studio" ""
    SetRegView 64
    ${If} $0 != ""
      MessageBox MB_OK|MB_ICONSTOP "A 32-bit OBS Studio installation was detected.$\n$\nThis plugin only supports 64-bit OBS Studio. Please install the 64-bit version of OBS Studio and run this installer again."
      Abort
    ${EndIf}

    ; No OBS installation found at all; fall back to default path and warn.
    StrCpy $OBSPath "$PROGRAMFILES64\obs-studio"
    MessageBox MB_OKCANCEL|MB_ICONINFORMATION "OBS Studio could not be detected automatically.$\n$\nThe plugin will be installed to the default location:$\n$OBSPath$\n$\nClick OK to continue or Cancel to abort." IDOK +2
    Abort
  ${EndIf}

  ; Restore prior install dir if the software was installed before.
  ReadRegStr $0 HKLM "Software\Spotify Lyrics Widget" "InstallDir"
  ${If} $0 != ""
    StrCpy $INSTDIR $0
  ${EndIf}

  ; Check whether OBS is currently running.
  nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq obs64.exe" /NH 2>NUL | find /I "obs64.exe" >NUL 2>&1'
  Pop $0
  ${If} $0 == 0
    StrCpy $OBSWasRunning "1"

    ; If the plugin DLL is already present, OBS has it loaded; must close first.
    IfFileExists "$OBSPath\obs-plugins\64bit\spotify-lyrics.dll" 0 DoneInit

    CheckOBSRunning:
      MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "OBS Studio is currently running and the plugin is already loaded.$\n$\nPlease close OBS Studio before continuing the installation." IDRETRY VerifyOBSClosed IDCANCEL AbortInit

      VerifyOBSClosed:
        nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq obs64.exe" /NH 2>NUL | find /I "obs64.exe" >NUL 2>&1'
        Pop $0
        ${If} $0 == 0
          Goto CheckOBSRunning
        ${Else}
          StrCpy $OBSWasRunning "0"
          Goto DoneInit
        ${EndIf}

      AbortInit:
        Abort
  ${EndIf}

  DoneInit:
FunctionEnd

; ---------------------------------------------------------------------------
; Finish page: inject dynamic reminder text
; ---------------------------------------------------------------------------
Function FinishPageShow
  ${If} $OBSWasRunning == "1"
    SendMessage $mui.FinishPage.Text ${WM_SETTEXT} 0 "STR:OBS Studio is currently running.$\nPlease restart OBS Studio to activate the Spotify Lyrics plugin."
  ${Else}
    SendMessage $mui.FinishPage.Text ${WM_SETTEXT} 0 "STR:Installation complete. You may now start OBS Studio."
  ${EndIf}
FunctionEnd

; ---------------------------------------------------------------------------
; Sections
; ---------------------------------------------------------------------------
Section "OBS Plugin" SecPlugin
  SectionIn RO

  SetRegView 64
  SetOutPath "$OBSPath\obs-plugins\64bit"
  File "/oname=spotify-lyrics.dll" "..\spotify-lyrics-windows-amd64.dll"

  ; Store OBS path so the uninstaller can remove the DLL later.
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS" "OBSPath" "$OBSPath"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS" "DisplayName" "Spotify Lyrics for OBS"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS" "Publisher" "Carl Kittelberger (Icedream)"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "Software\Spotify Lyrics Widget" "InstallDir" "$INSTDIR"

  SetOutPath "$INSTDIR"
  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Section /o "Lyrics Server" SecServer
  SetRegView 64
  SetOutPath "$INSTDIR"
  File "/oname=lyrics-server.exe" "..\lyrics-windows-amd64.exe"

  CreateDirectory "$SMPROGRAMS\Spotify Lyrics Widget"
  CreateShortcut "$SMPROGRAMS\Spotify Lyrics Widget\Lyrics Server.lnk" "$INSTDIR\lyrics-server.exe" "serve"
  CreateShortcut "$SMPROGRAMS\Spotify Lyrics Widget\Uninstall.lnk" "$INSTDIR\uninstall.exe"

  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS" "ServerInstalled" 1
SectionEnd

; ---------------------------------------------------------------------------
; Section descriptions (shown in component page tooltip)
; ---------------------------------------------------------------------------
LangString DESC_SecPlugin ${LANG_ENGLISH} "The OBS Studio plugin that displays Spotify lyrics as a text source."
LangString DESC_SecServer ${LANG_ENGLISH} "The standalone lyrics HTTP server for use with browser sources and custom integrations (optional)."

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecPlugin} $(DESC_SecPlugin)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecServer} $(DESC_SecServer)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ---------------------------------------------------------------------------
; Uninstaller
; ---------------------------------------------------------------------------
Function un.onInit
  ; If OBS is running, the DLL will be locked; ask the user to close it first.
  nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq obs64.exe" /NH 2>NUL | find /I "obs64.exe" >NUL 2>&1'
  Pop $0
  ${If} $0 == 0
    UnCheckOBSRunning:
      MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "OBS Studio is currently running.$\n$\nPlease close OBS Studio before uninstalling the Spotify Lyrics plugin." IDRETRY UnVerifyOBSClosed IDCANCEL UnAbortUninstall

      UnVerifyOBSClosed:
        nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq obs64.exe" /NH 2>NUL | find /I "obs64.exe" >NUL 2>&1'
        Pop $0
        ${If} $0 == 0
          Goto UnCheckOBSRunning
        ${Else}
          Goto UnDoneInit
        ${EndIf}

      UnAbortUninstall:
        Abort

    UnDoneInit:
  ${EndIf}
FunctionEnd

Section "Uninstall"
  SetRegView 64

  ; Remove the OBS plugin DLL.
  ReadRegStr $0 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS" "OBSPath"
  ${If} $0 != ""
    Delete "$0\obs-plugins\64bit\spotify-lyrics.dll"
  ${EndIf}

  ; Remove lyrics server and shortcuts if it was installed.
  ReadRegDWORD $1 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS" "ServerInstalled"
  ${If} $1 = 1
    Delete "$INSTDIR\lyrics-server.exe"
    Delete "$SMPROGRAMS\Spotify Lyrics Widget\Lyrics Server.lnk"
    Delete "$SMPROGRAMS\Spotify Lyrics Widget\Uninstall.lnk"
    RMDir  "$SMPROGRAMS\Spotify Lyrics Widget"
  ${EndIf}

  Delete "$INSTDIR\uninstall.exe"
  RMDir  "$INSTDIR"

  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\SpotifyLyricsOBS"
  DeleteRegKey HKLM "Software\Spotify Lyrics Widget"
SectionEnd
