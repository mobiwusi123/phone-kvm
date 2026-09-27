# Build script: produces outputs\phonekvm.exe
# Needs Go 1.21+ only. No network access, stdlib only.
# NOTE: keep this file pure ASCII. Windows PowerShell 5.1 reads .ps1 files as ANSI
# unless they have a UTF-8 BOM, so non-ASCII characters here would break parsing.
$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
if (-not $here) { $here = Split-Path -Parent $MyInvocation.MyCommand.Path }
$outputs = Join-Path (Split-Path -Parent $here) "outputs"
New-Item -ItemType Directory -Force -Path $outputs | Out-Null
$exe = Join-Path $outputs "phonekvm.exe"
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
