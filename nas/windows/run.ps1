#Requires -Version 5.1
# Loads data\config.env (KEY="value" lines, same format as the Synology/QNAP
# config.env files) into environment variables, then launches
# pkg-sender-nas.exe in the foreground. Intended to be run directly, or
# wrapped by a Scheduled Task for auto-start (see install-task.ps1).

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$dataDir = Join-Path $scriptDir "data"
$configFile = Join-Path $dataDir "config.env"
$exe = Join-Path $scriptDir "pkg-sender-nas.exe"

if (-not (Test-Path $dataDir)) {
    New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
}

if (-not (Test-Path $configFile)) {
    Write-Error "Config file not found: $configFile"
    exit 1
}

function ConvertFrom-GoQuotedString {
    # Reverses exactly the escaping Go's strconv.Quote applies (used by
    # updateConfigEnvValue when the Web UI persists settings back into this
    # file). A naive raw capture breaks as soon as a value contains an
    # escaped backslash or quote, e.g. a UNC path or a JSON-encoded array
    # written by /api/settings/libraries.
    param([string]$Value)
    $sb = [System.Text.StringBuilder]::new()
    $i = 0
    $len = $Value.Length
    while ($i -lt $len) {
        $ch = $Value[$i]
        if ($ch -eq '\' -and ($i + 1) -lt $len) {
            $next = $Value[$i + 1]
            if ($next -eq '"') { [void]$sb.Append('"'); $i += 2 }
            elseif ($next -eq '\') { [void]$sb.Append('\'); $i += 2 }
            elseif ($next -eq 'n') { [void]$sb.Append([char]10); $i += 2 }
            elseif ($next -eq 't') { [void]$sb.Append([char]9); $i += 2 }
            elseif ($next -eq 'r') { [void]$sb.Append([char]13); $i += 2 }
            else { [void]$sb.Append($ch); $i += 1 }
        } else {
            [void]$sb.Append($ch); $i += 1
        }
    }
    return $sb.ToString()
}

Get-Content $configFile | ForEach-Object {
    $line = $_.Trim()
    if ($line -eq "" -or $line.StartsWith("#")) { return }
    if ($line -match '^([A-Za-z_][A-Za-z0-9_]*)="(.*)"$') {
        $value = ConvertFrom-GoQuotedString $Matches[2]
        [System.Environment]::SetEnvironmentVariable($Matches[1], $value, "Process")
    }
}

if (-not $env:PKGSENDER_HISTORY_FILE) {
    $env:PKGSENDER_HISTORY_FILE = Join-Path $dataDir "history.json"
}
if (-not $env:PKGSENDER_TITLE_ALIASES_FILE) {
    $env:PKGSENDER_TITLE_ALIASES_FILE = Join-Path $dataDir "aliases.json"
}
$env:PKGSENDER_CONFIG_FILE = $configFile

if (-not (Test-Path $env:PKGSENDER_PACKAGE_DIR)) {
    New-Item -ItemType Directory -Force -Path $env:PKGSENDER_PACKAGE_DIR | Out-Null
}

& $exe
