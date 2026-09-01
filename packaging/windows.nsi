; Windows installer for tailscale-socks. Built by packaging/windows-nsis.sh,
; which stages the payload and passes STAGE, VERSION, VIVERSION, ARCH and
; OUTFILE. Per-user: no administrator, no Program Files, no service — the
; node still runs through ts_install, like on every other platform.

Unicode true
ManifestDPIAware true
RequestExecutionLevel user
SetCompressor /SOLID lzma

!define NAME "tailscale-socks"
!define PUBLISHER "d0whc3r"
!define URL "https://github.com/d0whc3r/tailscale-socks"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${NAME}"

Name "${NAME} ${VERSION} (${ARCH})"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\${NAME}"
InstallDirRegKey HKCU "Software\${NAME}" "InstallDir"

VIProductVersion "${VIVERSION}"
VIAddVersionKey "ProductName" "${NAME}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "CompanyName" "${PUBLISHER}"
VIAddVersionKey "LegalCopyright" "MIT"
VIAddVersionKey "FileDescription" "${NAME} installer"

!include LogicLib.nsh
!include MUI2.nsh

!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_LICENSE "${STAGE}/LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\service.md"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Read how to run it as a service"
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "install"
  SetOutPath "$INSTDIR"
  File "${STAGE}/tailscale-socks.exe"
  File "${STAGE}/LICENSE"
  File "${STAGE}/README.md"
  File "${STAGE}/service.md"
  File "${STAGE}/path.ps1"

  SetOutPath "$INSTDIR\contrib"
  File "${STAGE}/contrib/tailscale-socks.zsh"
  SetOutPath "$INSTDIR\contrib\platform"
  File "${STAGE}/contrib/platform/windows.zsh"

  ; The binary reads $INSTDIR\.env, so after the first install this file is
  ; the user's configuration and may hold TS_AUTHKEY. Reinstalling must not
  ; overwrite it.
  SetOutPath "$INSTDIR"
  SetOverwrite off
  File "${STAGE}/.env"
  SetOverwrite on

  WriteUninstaller "$INSTDIR\uninstall.exe"

  WriteRegStr HKCU "Software\${NAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "${NAME}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "${PUBLISHER}"
  WriteRegStr HKCU "${UNINSTKEY}" "URLInfoAbout" "${URL}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\tailscale-socks.exe"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1

  nsExec::ExecToLog 'powershell -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\path.ps1" -Dir "$INSTDIR" -Action add'
  Pop $0
  ${If} $0 != 0
    DetailPrint "could not add $INSTDIR to PATH; add it by hand"
  ${EndIf}

  DetailPrint "config: edit $INSTDIR\.env"
  DetailPrint "helpers: source $INSTDIR\contrib\tailscale-socks.zsh from ~/.zshrc"
SectionEnd

Section "uninstall"
  nsExec::ExecToLog 'powershell -NoProfile -ExecutionPolicy Bypass -File "$INSTDIR\path.ps1" -Dir "$INSTDIR" -Action remove'
  Pop $0

  Delete "$INSTDIR\contrib\platform\windows.zsh"
  Delete "$INSTDIR\contrib\tailscale-socks.zsh"
  RMDir "$INSTDIR\contrib\platform"
  RMDir "$INSTDIR\contrib"
  Delete "$INSTDIR\tailscale-socks.exe"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\service.md"
  Delete "$INSTDIR\path.ps1"
  Delete "$INSTDIR\uninstall.exe"

  ; $INSTDIR\.env is the user's configuration and may hold an auth key, so it
  ; stays. RMDir without /r leaves the directory alone when it is still there.
  RMDir "$INSTDIR"
  ${If} ${FileExists} "$INSTDIR\.env"
    DetailPrint "kept: $INSTDIR\.env"
  ${EndIf}

  DeleteRegKey HKCU "${UNINSTKEY}"
  DeleteRegKey HKCU "Software\${NAME}"
SectionEnd
