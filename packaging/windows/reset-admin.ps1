# Reset the bitdispatcher administrator's password (Start menu > bitdispatcher > Reset admin password).
# Asks Windows for administrator rights, stops the service, sets a new generated password for "admin",
# starts the service again and shows the password in this window only. It is not saved anywhere.
$ErrorActionPreference = 'Stop'
$me = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
if (-not $me.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Start-Process powershell.exe -Verb RunAs -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`"")
    exit
}
$Host.UI.RawUI.WindowTitle = 'bitdispatcher: reset admin password'
$reg = Get-ItemProperty -Path 'HKLM:\Software\BioDream\bitdispatcher'
$exe = Join-Path $reg.InstallDir 'bitdispatcher.exe'
Write-Host ''
Write-Host 'This sets a new password for the bitdispatcher user "admin".' -ForegroundColor Cyan
Write-Host 'Dispatch stops for a few seconds while the service restarts.'
$ok = Read-Host 'Type YES to continue'
if ($ok -ne 'YES') { Write-Host 'Nothing changed.'; Read-Host 'Press Enter to close'; exit }
Write-Host 'Stopping the bitdispatcher service...'
& net.exe stop bitdispatcher | Out-Null
try {
    $out = & $exe reset-password -data $reg.DataDir admin 2>&1
    if ($LASTEXITCODE -ne 0) { throw ($out -join ' ') }
    $pw = ($out | Select-String -Pattern 'New password for admin: (\S+)').Matches[0].Groups[1].Value
} finally {
    Write-Host 'Starting the bitdispatcher service...'
    & net.exe start bitdispatcher | Out-Null
}
Write-Host ''
Write-Host '  User name:  admin'
Write-Host "  Password:   $pw" -ForegroundColor Green
Write-Host ''
Write-Host "Sign in at https://localhost:$($reg.Port)/ and change it in Admin > Users. It is shown only here."
Read-Host 'Press Enter to close'
