param(
    [string]$Version = $env:GDBG_VERSION,
    [string]$InstallDir = $env:GDBG_INSTALL_DIR
)

$ErrorActionPreference = "Stop"
$Repository = "Dluck-Games/godot-debug-bridge"

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Release = Invoke-RestMethod -Headers @{ Accept = "application/vnd.github+json" } `
        -Uri "https://api.github.com/repos/$Repository/releases/latest"
    $Version = $Release.tag_name
}
$Version = $Version.TrimStart("v")

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $HOME ".local\bin"
}

$Architecture = switch ($env:PROCESSOR_ARCHITECTURE) {
    "ARM64" { "arm64" }
    default { "amd64" }
}
$Asset = "gdbg-v$Version-windows-$Architecture.zip"
$BaseUrl = "https://github.com/$Repository/releases/download/v$Version"
$InstallTemp = Join-Path ([IO.Path]::GetTempPath()) ("gdbg-" + [Guid]::NewGuid().ToString("N"))

try {
    New-Item -ItemType Directory -Path $InstallTemp | Out-Null
    $Archive = Join-Path $InstallTemp $Asset
    $Checksums = Join-Path $InstallTemp "checksums.txt"
    Invoke-WebRequest -Uri "$BaseUrl/$Asset" -OutFile $Archive
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $Checksums

    $ExpectedLine = Get-Content $Checksums | Where-Object { $_ -match "\s+$([regex]::Escape($Asset))$" } | Select-Object -First 1
    if (-not $ExpectedLine) {
        throw "No checksum published for $Asset"
    }
    $Expected = ($ExpectedLine -split "\s+")[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 $Archive).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) {
        throw "Checksum verification failed for $Asset"
    }

    Expand-Archive -Path $Archive -DestinationPath $InstallTemp -Force
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item -Force (Join-Path $InstallTemp "gdbg.exe") (Join-Path $InstallDir "gdbg.exe")
    Write-Host "Installed gdbg v$Version to $(Join-Path $InstallDir 'gdbg.exe')"
    if (-not (($env:PATH -split ";") -contains $InstallDir)) {
        Write-Host "Add $InstallDir to PATH to run gdbg."
    }
}
finally {
    if (Test-Path $InstallTemp) {
        Remove-Item -Recurse -Force $InstallTemp
    }
}
