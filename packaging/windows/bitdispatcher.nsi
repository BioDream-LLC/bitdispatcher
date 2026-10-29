; bitdispatcher installer for Windows (NSIS 3, Modern UI 2). Build with `make windows`.
;
; Installs bitdispatcher.exe as a Windows service that starts at boot and restarts on failure,
; keeps its data in %ProgramData%\bitdispatcher (SYSTEM and Administrators only), optionally opens
; its port in Windows Firewall for domain and private networks, and shows the first administrator
; password once. Upgrades keep the data, the port and the data folder.
;
; Silent install, for IT: bitdispatcher-setup.exe /S [/PORT=8443] [/DATA=D:\bitdispatcher] [/NOFIREWALL]
; A silent first install writes the first administrator password to <data>\logs\bitdispatcher.log.
; Silent uninstall keeps the data: "C:\Program Files\bitdispatcher\uninstall.exe" /S

Unicode true
ManifestDPIAware true
RequestExecutionLevel admin
SetCompressor /SOLID lzma

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef EXE
  !define EXE "..\..\bin\bitdispatcher-windows-amd64.exe"
!endif
!ifndef OUT
  !define OUT "..\..\bin\bitdispatcher-setup.exe"
!endif
!ifndef MANUAL
  !define MANUAL "..\..\docs\manual\bitdispatcher-manual.pdf"
!endif

!define PRODUCT "bitdispatcher"
!define PUBLISHER "BioDream LLC"
!define SERVICE "bitdispatcher"
!define REGKEY "Software\BioDream\bitdispatcher"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\bitdispatcher"
!define FWRULE "bitdispatcher (HTTPS)"

Name "${PRODUCT}"
Caption "${PRODUCT} ${VERSION} Setup"
OutFile "${OUT}"
InstallDir "$PROGRAMFILES64\bitdispatcher"
InstallDirRegKey HKLM "${REGKEY}" "InstallDir"
BrandingText "${PUBLISHER}"
ShowInstDetails show
ShowUninstDetails show

VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${PRODUCT}"
VIAddVersionKey "CompanyName" "${PUBLISHER}"
VIAddVersionKey "LegalCopyright" "Copyright BioDream LLC. Apache License 2.0."
VIAddVersionKey "FileDescription" "${PRODUCT} Setup"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

!include "MUI2.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "FileFunc.nsh"
!include "Sections.nsh"
!include "TextFunc.nsh"

!define MUI_ICON "bitdispatcher.ico"
!define MUI_UNICON "bitdispatcher.ico"
!define MUI_WELCOMEFINISHPAGE_BITMAP "wizard.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "wizard.bmp"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_RIGHT
!define MUI_HEADERIMAGE_BITMAP "header.bmp"
!define MUI_HEADERIMAGE_UNBITMAP "header.bmp"
!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_SMALLDESC

Var Port
Var DataDir
Var Upgrade
Var Password
Var PortField
Var DataField

; ---------- pages ----------

!define MUI_WELCOMEPAGE_TITLE "Welcome to ${PRODUCT} ${VERSION}"
!define MUI_WELCOMEPAGE_TEXT "This installs ${PRODUCT}, open-source computer-aided dispatch for 911 centers, on this computer.$\r$\n$\r$\nIt runs as a Windows service that starts with Windows. Dispatchers open it in a web browser; nothing is installed on their computers.$\r$\n$\r$\nRecords stay on this computer. Nothing is sent to BioDream.$\r$\n$\r$\nClick Next to continue."
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\LICENSE"
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
Page custom SettingsPage SettingsLeave
!insertmacro MUI_PAGE_INSTFILES
Page custom PasswordPage
!define MUI_FINISHPAGE_TITLE "${PRODUCT} is installed"
!define MUI_FINISHPAGE_TEXT "The ${PRODUCT} service is running.$\r$\n$\r$\nFrom other computers, open https://<this computer's name>:$Port/ in a browser.$\r$\n$\r$\nBrowsers warn about its self-signed certificate until it has one your network trusts (the -tls-cert option, in the manual). Before accepting, compare the fingerprint the browser shows with certificate-sha256 in the log."
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Open ${PRODUCT} now"
!define MUI_FINISHPAGE_RUN_FUNCTION OpenConsole
!define MUI_FINISHPAGE_SHOWREADME
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Open the manual"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION OpenManual
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

; ---------- settings page: port and data folder ----------

Function SettingsPage
  !insertmacro MUI_HEADER_TEXT "Settings" "Where dispatchers connect, and where the records are kept."
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 24u "Port for the web interface (HTTPS). Consoles open https://<this computer>:<port>/."
  Pop $0
  ${NSD_CreateNumber} 0 26u 60u 12u "$Port"
  Pop $PortField
  ${NSD_CreateLabel} 0 52u 100% 24u "Data folder: the ledger of every call and change, users, settings and the recorded map. Only SYSTEM and Administrators can open it."
  Pop $0
  ${NSD_CreateDirRequest} 0 78u 82% 12u "$DataDir"
  Pop $DataField
  ${NSD_CreateBrowseButton} 84% 77u 16% 14u "Browse..."
  Pop $0
  ${NSD_OnClick} $0 BrowseData
  ${If} $Upgrade == "1"
    ${NSD_CreateLabel} 0 104u 100% 24u "An earlier installation was found. Its records are kept and used."
    Pop $0
  ${EndIf}
  nsDialogs::Show
FunctionEnd

Function BrowseData
  ${NSD_GetText} $DataField $0
  nsDialogs::SelectFolderDialog "Data folder" $0
  Pop $0
  ${If} $0 != "error"
    ${NSD_SetText} $DataField $0
  ${EndIf}
FunctionEnd

Function SettingsLeave
  ${NSD_GetText} $PortField $Port
  ${NSD_GetText} $DataField $DataDir
  ${If} $Port < 1
  ${OrIf} $Port > 65535
    MessageBox MB_ICONEXCLAMATION|MB_OK "The port must be a number from 1 to 65535."
    Abort
  ${EndIf}
  ; Another program on the port stops the service from starting (BitVoice also defaults to 8443).
  ; An upgrade keeping its port skips this, since the old service holds the port until it stops.
  ReadRegStr $1 HKLM "${REGKEY}" "Port"
  ${If} $1 != $Port
    nsExec::ExecToStack 'cmd.exe /c netstat.exe -ano -p TCP | findstr.exe /R /C:":$Port .*LISTENING"'
    Pop $0
    Pop $1
    ${If} $0 == 0
      MessageBox MB_ICONEXCLAMATION|MB_OK "Another program on this computer is already using port $Port.$\r$\n$\r$\nChoose another port, such as 9443."
      Abort
    ${EndIf}
  ${EndIf}
  StrCpy $0 $DataDir 1 -1
  ${If} $0 == "\"
    StrCpy $DataDir $DataDir -1
  ${EndIf}
  ${If} $DataDir == ""
    MessageBox MB_ICONEXCLAMATION|MB_OK "Choose a data folder."
    Abort
  ${EndIf}
FunctionEnd

; ---------- sections ----------

Section "${PRODUCT} (required)" SecCore
  SectionIn RO
  SetOutPath "$INSTDIR"
  ; An upgrade stops the running service before its program is replaced.
  nsExec::ExecToLog 'sc.exe query "${SERVICE}"'
  Pop $0
  ${If} $0 == 0
    DetailPrint "Stopping the ${PRODUCT} service..."
    nsExec::ExecToLog 'net.exe stop "${SERVICE}"'
    Pop $0
  ${EndIf}
  File "/oname=bitdispatcher.exe" "${EXE}"
  File "bitdispatcher.ico"
  File "reset-admin.ps1"
  File "/oname=LICENSE.txt" "..\..\LICENSE"
  File "/oname=bitdispatcher-manual.pdf" "${MANUAL}"

  ; The data folder holds criminal justice and personal information: SYSTEM and Administrators only.
  CreateDirectory "$DataDir"
  nsExec::ExecToLog 'icacls.exe "$DataDir" /inheritance:r /grant:r *S-1-5-18:(OI)(CI)F /grant:r *S-1-5-32-544:(OI)(CI)F'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION|MB_OK "Could not limit the data folder to Administrators (icacls $0). Check its permissions before use." /SD IDOK
  ${EndIf}

  ; The first administrator is made here, so its password can be shown once instead of logged.
  StrCpy $Password ""
  ${IfNot} ${Silent}
    nsExec::ExecToStack '"$INSTDIR\bitdispatcher.exe" init-admin -data "$DataDir"'
    Pop $0
    Pop $1
    ${If} $0 == 0
      ${TrimNewLines} $1 $Password
    ${Else}
      DetailPrint "init-admin failed ($0): $1"
    ${EndIf}
  ${EndIf}

  WriteRegStr HKLM "${REGKEY}" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "${REGKEY}" "Port" "$Port"
  WriteRegStr HKLM "${REGKEY}" "DataDir" "$DataDir"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  WriteRegStr HKLM "${UNINSTKEY}" "DisplayName" "${PRODUCT}"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINSTKEY}" "Publisher" "${PUBLISHER}"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\bitdispatcher.ico"
  WriteRegStr HKLM "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTKEY}" "URLInfoAbout" "https://bitdispatcher.com/"
  WriteRegStr HKLM "${UNINSTKEY}" "HelpLink" "https://github.com/BioDream-LLC/bitdispatcher"
  WriteRegStr HKLM "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKLM "${UNINSTKEY}" "EstimatedSize" "$0"
SectionEnd

Section "Windows service, started with Windows" SecService
  SectionIn RO
  ; Registered (or updated) with this program, data folder and port; restarts after a failure.
  nsExec::ExecToLog 'sc.exe query "${SERVICE}"'
  Pop $0
  ${If} $0 == 0
    nsExec::ExecToLog 'sc.exe config "${SERVICE}" binPath= "\"$INSTDIR\bitdispatcher.exe\" -data \"$DataDir\" -addr :$Port" start= auto'
  ${Else}
    nsExec::ExecToLog 'sc.exe create "${SERVICE}" binPath= "\"$INSTDIR\bitdispatcher.exe\" -data \"$DataDir\" -addr :$Port" start= auto DisplayName= "${PRODUCT} CAD"'
  ${EndIf}
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP|MB_OK "Could not register the ${PRODUCT} service (sc.exe $0)." /SD IDOK
    Abort
  ${EndIf}
  nsExec::ExecToLog 'sc.exe description "${SERVICE}" "bitdispatcher computer-aided dispatch: web interface, API and the event ledger."'
  Pop $0
  nsExec::ExecToLog 'sc.exe failure "${SERVICE}" reset= 86400 actions= restart/5000/restart/10000/restart/30000'
  Pop $0
  nsExec::ExecToLog 'sc.exe failureflag "${SERVICE}" 1'
  Pop $0
  DetailPrint "Starting the ${PRODUCT} service..."
  nsExec::ExecToLog 'net.exe start "${SERVICE}"'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION|MB_OK "The ${PRODUCT} service did not start (net start $0). The reason is in $DataDir\logs\bitdispatcher.log; a common one is another program already using port $Port." /SD IDOK
  ${EndIf}
SectionEnd

Section "Allow other computers through Windows Firewall" SecFirewall
  ; Domain and private networks only, never public ones.
  nsExec::ExecToLog 'netsh.exe advfirewall firewall delete rule name="${FWRULE}"'
  Pop $0
  nsExec::ExecToLog 'netsh.exe advfirewall firewall add rule name="${FWRULE}" dir=in action=allow protocol=TCP localport=$Port program="$INSTDIR\bitdispatcher.exe" profile=domain,private'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION|MB_OK "Could not add the firewall rule (netsh $0). Consoles on other computers may not connect." /SD IDOK
  ${EndIf}
SectionEnd

Section "Start menu shortcuts" SecShortcuts
  CreateDirectory "$SMPROGRAMS\${PRODUCT}"
  WriteINIStr "$INSTDIR\bitdispatcher.url" "InternetShortcut" "URL" "https://localhost:$Port/"
  WriteINIStr "$INSTDIR\bitdispatcher.url" "InternetShortcut" "IconFile" "$INSTDIR\bitdispatcher.ico"
  WriteINIStr "$INSTDIR\bitdispatcher.url" "InternetShortcut" "IconIndex" "0"
  CreateShortcut "$SMPROGRAMS\${PRODUCT}\${PRODUCT}.lnk" "$INSTDIR\bitdispatcher.url" "" "$INSTDIR\bitdispatcher.ico"
  CreateShortcut "$SMPROGRAMS\${PRODUCT}\${PRODUCT} manual.lnk" "$INSTDIR\bitdispatcher-manual.pdf"
  CreateShortcut "$SMPROGRAMS\${PRODUCT}\Reset admin password.lnk" "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" '-NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\reset-admin.ps1"' "$INSTDIR\bitdispatcher.ico"
  CreateShortcut "$SMPROGRAMS\${PRODUCT}\Uninstall ${PRODUCT}.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} "The program, its manual and licence."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecService} "Runs ${PRODUCT} in the background from boot, and restarts it if it stops."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecFirewall} "Lets consoles on other computers connect, on domain and private networks only. Leave off for one computer."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecShortcuts} "Start menu entries to open ${PRODUCT} and its manual, and to reset the admin password."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ---------- start-up checks ----------

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP|MB_OK "${PRODUCT} needs 64-bit Windows." /SD IDOK
    Abort
  ${EndIf}
  SetRegView 64
  SetShellVarContext all
  StrCpy $Port "8443"
  StrCpy $DataDir "$APPDATA\bitdispatcher"
  StrCpy $Upgrade "0"
  ReadRegStr $0 HKLM "${REGKEY}" "Port"
  ${If} $0 != ""
    StrCpy $Port $0
    StrCpy $Upgrade "1"
  ${EndIf}
  ReadRegStr $0 HKLM "${REGKEY}" "DataDir"
  ${If} $0 != ""
    StrCpy $DataDir $0
  ${EndIf}
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/PORT=" $0
  ${IfNot} ${Errors}
    StrCpy $Port $0
  ${EndIf}
  ClearErrors
  ${GetOptions} $R0 "/DATA=" $0
  ${IfNot} ${Errors}
    StrCpy $DataDir $0
  ${EndIf}
  ClearErrors
  ${GetOptions} $R0 "/NOFIREWALL" $0
  ${IfNot} ${Errors}
    !insertmacro UnselectSection ${SecFirewall}
  ${EndIf}
FunctionEnd

; ---------- first sign-in page ----------

Function PasswordPage
  ${If} $Password == ""
    Abort ; an upgrade, or the administrator already exists
  ${EndIf}
  !insertmacro MUI_HEADER_TEXT "First sign-in" "Write this down now. It is shown only once."
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 20u "Sign in to ${PRODUCT} as the administrator with:"
  Pop $0
  ${NSD_CreateLabel} 0 26u 60u 12u "User name"
  Pop $0
  ${NSD_CreateText} 64u 24u 70% 12u "admin"
  Pop $0
  SendMessage $0 ${EM_SETREADONLY} 1 0
  ${NSD_CreateLabel} 0 44u 60u 12u "Password"
  Pop $0
  ${NSD_CreateText} 64u 42u 70% 12u "$Password"
  Pop $0
  SendMessage $0 ${EM_SETREADONLY} 1 0
  ${NSD_CreateLabel} 0 66u 100% 40u "The password is not saved anywhere you can read it later. Change it, and add your own users, in Admin › Users. If it is lost: Start menu › ${PRODUCT} › Reset admin password."
  Pop $0
  nsDialogs::Show
FunctionEnd

Function OpenConsole
  ExecShell "open" "https://localhost:$Port/"
FunctionEnd

Function OpenManual
  ExecShell "open" "$INSTDIR\bitdispatcher-manual.pdf"
FunctionEnd

; ---------- uninstall ----------

Function un.onInit
  SetRegView 64
  SetShellVarContext all
  ReadRegStr $DataDir HKLM "${REGKEY}" "DataDir"
FunctionEnd

Section "Uninstall"
  DetailPrint "Stopping and removing the ${PRODUCT} service..."
  nsExec::ExecToLog 'net.exe stop "${SERVICE}"'
  Pop $0
  nsExec::ExecToLog 'sc.exe delete "${SERVICE}"'
  Pop $0
  nsExec::ExecToLog 'netsh.exe advfirewall firewall delete rule name="${FWRULE}"'
  Pop $0
  Delete "$SMPROGRAMS\${PRODUCT}\${PRODUCT}.lnk"
  Delete "$SMPROGRAMS\${PRODUCT}\${PRODUCT} manual.lnk"
  Delete "$SMPROGRAMS\${PRODUCT}\Reset admin password.lnk"
  Delete "$SMPROGRAMS\${PRODUCT}\Uninstall ${PRODUCT}.lnk"
  RMDir "$SMPROGRAMS\${PRODUCT}"
  Delete "$INSTDIR\bitdispatcher.exe"
  Delete "$INSTDIR\bitdispatcher.ico"
  Delete "$INSTDIR\reset-admin.ps1"
  Delete "$INSTDIR\bitdispatcher.url"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\bitdispatcher-manual.pdf"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  ; The records are kept unless the administrator asks, twice, to delete them. Silent uninstall keeps them.
  ${If} $DataDir != ""
  ${AndIf} ${FileExists} "$DataDir\*.*"
    MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Also delete the records in $DataDir?$\r$\n$\r$\nThis includes the ledger of every call and dispatch. Keep them unless you have a copy and your retention rules allow it." /SD IDNO IDNO keep
    MessageBox MB_YESNO|MB_ICONEXCLAMATION|MB_DEFBUTTON2 "Delete $DataDir permanently? This cannot be undone." /SD IDNO IDNO keep
    RMDir /r "$DataDir"
    DeleteRegKey HKLM "${REGKEY}"
    Goto done
  keep:
    DetailPrint "Kept the records in $DataDir"
    DeleteRegValue HKLM "${REGKEY}" "InstallDir"
  done:
  ${Else}
    DeleteRegKey HKLM "${REGKEY}"
  ${EndIf}
  DeleteRegKey HKLM "${UNINSTKEY}"
SectionEnd
