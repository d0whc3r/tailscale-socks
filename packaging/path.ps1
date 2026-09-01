# Adds or removes one directory in the per-user PATH. Run by the NSIS
# installer and its uninstaller.
#
# PowerShell instead of NSIS: the default makensis build caps a string at 1024
# characters, so reading a long PATH into a register would silently truncate
# it and writing it back would destroy it. SetEnvironmentVariable also
# broadcasts WM_SETTINGCHANGE, which NSIS would have to do by hand.

param(
  [Parameter(Mandatory = $true)][string]$Dir,
  [Parameter(Mandatory = $true)][ValidateSet('add', 'remove')][string]$Action
)

$ErrorActionPreference = 'Stop'

$current = [Environment]::GetEnvironmentVariable('Path', 'User')
$parts = @()
if ($current) { $parts = $current -split ';' | Where-Object { $_ -ne '' } }

$kept = $parts | Where-Object { $_.TrimEnd('\') -ne $Dir.TrimEnd('\') }
if ($Action -eq 'add') { $kept = @($kept) + $Dir }

$updated = ($kept -join ';')
if ($updated -ne $current) {
  [Environment]::SetEnvironmentVariable('Path', $updated, 'User')
}
