$ErrorActionPreference = "Stop"

$Repo = "GongJS/agym"
$InstallDir = if ($env:AGYM_INSTALL_DIR) { $env:AGYM_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "agym\bin" }
$Version = $env:AGYM_VERSION

if ($env:OS -ne "Windows_NT") {
    Write-Error "agym install.ps1 is for Windows only."
    exit 1
}

$archRaw = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
switch ($archRaw.ToUpperInvariant()) {
    "X64"   { $Arch = "amd64" }
    "AMD64" { $Arch = "amd64" }
    default {
        Write-Error "Unsupported Windows architecture: $archRaw (currently only amd64 is published)."
        exit 1
    }
}

if (-not $Version) {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "agym-installer" }
    $Version = $release.tag_name
    if (-not $Version) {
        Write-Error "Failed to resolve latest release version from GitHub."
        exit 1
    }
}

$VerNoV = $Version.TrimStart("v")
$Archive = "agym_${VerNoV}_windows_${Arch}.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/$Version"

$TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("agym-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null

try {
    $ArchivePath = Join-Path $TmpDir $Archive
    $ChecksumsPath = Join-Path $TmpDir "checksums.txt"

    Write-Host "Downloading agym $Version (windows/$Arch)..."
    Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile $ArchivePath -UseBasicParsing
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumsPath -UseBasicParsing

    $expectedLine = Get-Content $ChecksumsPath | Where-Object { $_ -match "\s+$([regex]::Escape($Archive))$" } | Select-Object -First 1
    if (-not $expectedLine) {
        Write-Error "Checksum entry not found for $Archive"
        exit 1
    }
    $expectedHash = ($expectedLine -split "\s+")[0].ToLowerInvariant()
    $actualHash = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expectedHash -ne $actualHash) {
        Write-Error "SHA256 mismatch for $Archive (expected $expectedHash, got $actualHash)"
        exit 1
    }

    $ExtractDir = Join-Path $TmpDir "extract"
    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force
    $ExeSource = Join-Path $ExtractDir "agym.exe"
    if (-not (Test-Path $ExeSource)) {
        Write-Error "agym.exe not found in archive."
        exit 1
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $ExeTarget = Join-Path $InstallDir "agym.exe"
    Copy-Item -Path $ExeSource -Destination $ExeTarget -Force
    Write-Host "Installed agym to $ExeTarget"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $parts = @()
    if ($userPath) {
        $parts = $userPath -split ";" | Where-Object { $_.Trim() -ne "" }
    }
    $normalizedTarget = $InstallDir.TrimEnd("\").ToLowerInvariant()
    $alreadyInUserPath = $false
    foreach ($p in $parts) {
        if ($p.TrimEnd("\").ToLowerInvariant() -eq $normalizedTarget) {
            $alreadyInUserPath = $true
            break
        }
    }
    if (-not $alreadyInUserPath) {
        $newUserPath = if ($parts.Count -gt 0) { ($parts + $InstallDir) -join ";" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
        Write-Host "Added $InstallDir to user PATH."
    }

    $sessionParts = ($env:Path -split ";") | Where-Object { $_.Trim() -ne "" }
    $alreadyInSession = $false
    foreach ($p in $sessionParts) {
        if ($p.TrimEnd("\").ToLowerInvariant() -eq $normalizedTarget) {
            $alreadyInSession = $true
            break
        }
    }
    if (-not $alreadyInSession) {
        $env:Path = "$InstallDir;$env:Path"
    }

    Write-Host "Run 'agym' (or 'agym ui') to launch the tray UI, or 'agym --help' for CLI usage."
    Write-Host "Optional: run 'agym install-hook' only if you want automatic account handoff during 'agym supervise' sessions."
}
finally {
    Remove-Item -Path $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
}
