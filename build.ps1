$ErrorActionPreference = "Stop"

$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

$out = Join-Path $PSScriptRoot "bin"
New-Item -ItemType Directory -Force -Path $out | Out-Null

Write-Host "go mod tidy..."
go mod tidy

Write-Host "Building gonv.exe..."
go build -trimpath -o (Join-Path $out "gonv.exe") ./cmd/gonv

Write-Host "Building gonv-shim.exe..."
go build -trimpath -o (Join-Path $out "gonv-shim.exe") ./cmd/gonv-shim

Write-Host ""
Write-Host "Built:"
Get-ChildItem $out | ForEach-Object { Write-Host "  $($_.FullName)" }
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. .\bin\gonv.exe install 20.10.0"
Write-Host "  2. Add %USERPROFILE%\.gonv\shims to your PATH"
Write-Host "  3. cd into a project and run: gonv use 20.10.0"
