#Requires -Version 5.1
# Builds pkg-sender-nas.exe for this machine (native windows/amd64).
# Requires Go on PATH, or set $env:GO_BIN to a full path to go.exe.

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$nasDir = Split-Path -Parent $scriptDir
$goBin = if ($env:GO_BIN) { $env:GO_BIN } else { "go" }

Push-Location $nasDir
try {
    Write-Output "Running native Go tests..."
    & $goBin test ./...
    if ($LASTEXITCODE -ne 0) {
        throw "go test failed with exit code $LASTEXITCODE"
    }

    Write-Output "Building pkg-sender-nas.exe..."
    & $goBin build -trimpath -ldflags="-s -w" -o (Join-Path $scriptDir "pkg-sender-nas.exe") ./cmd/pkg-sender-nas
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
} finally {
    Pop-Location
}

Write-Output "Built: $(Join-Path $scriptDir 'pkg-sender-nas.exe')"
