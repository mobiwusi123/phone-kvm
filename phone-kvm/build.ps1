# Build script: produces phonekvm.exe in the repo root.
# Needs Go 1.26+ only. No network access, stdlib only.
# NOTE: keep this file pure ASCII. Windows PowerShell 5.1 reads .ps1 files as ANSI
# unless they have a UTF-8 BOM, so non-ASCII characters here would break parsing.
# NOTE: the binary lands in the repo root on purpose, so it is one click away from
# the GitHub file list and can be attached to a Release as-is.
$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
if (-not $here) { $here = Split-Path -Parent $MyInvocation.MyCommand.Path }
$exe = Join-Path (Split-Path -Parent $here) "phonekvm.exe"
Push-Location $here
try {
    Write-Host "Building phonekvm.exe ..."
    go build -trimpath -ldflags "-s -w" -o $exe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    $size = [math]::Round((Get-Item $exe).Length / 1MB, 1)
    Write-Host ("OK -> {0}  ({1} MB)" -f $exe, $size)
} finally {
    Pop-Location
}
