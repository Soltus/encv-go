<#
.SYNOPSIS
    Build the OpenList Android AAR (gomobile bind product) from the Hi-Sillot
    OpenList fork on Windows (CI matrix uses this for the windows-latest leg).

.DESCRIPTION
    Drop-in PowerShell 7+ equivalent of scripts/build-openlist-aar.sh. Produces
    <output>/openlist.aar (gomobile bind product) for ComboLite to consume via
    app/encv-mobile/plugin-openlist/libs/openlist.aar.

.PARAMETER Output
    Output directory for openlist.aar (required).

.PARAMETER Fork
    Hi-Sillot fork URL. Default: read from scripts/openlist-fork.env
    (OPENLIST_FORK_URL) or https://github.com/Hi-Sillot/OpenList.

.PARAMETER Branch
    Git branch / tag. Default: read from scripts/openlist-fork.env
    (OPENLIST_FORK_BRANCH) or "dev".

.PARAMETER Ndk
    Android NDK install path. Default: $env:ANDROID_HOME\ndk\26.3.11579264

.PARAMETER EncvGoRoot
    Local encv-go checkout (encv-go replace target). Default: C:\workspace

.PARAMETER FrontendVersion
    Pin OpenList-Frontend version (e.g. v4.0.0). Highest precedence.

.PARAMETER LocalFrontendDist
    Skip the GitHub download; copy a pre-staged dist directory into
    public/dist/ directly.

.EXAMPLE
    pwsh -File scripts/build-openlist-aar.ps1 `
        -Output  C:\workspace\app\encv-mobile\plugin-openlist\libs `
        -EncvGoRoot C:\workspace

.EXAMPLE
    pwsh -File scripts/build-openlist-aar.ps1 `
        -Output  C:\workspace\app\encv-mobile\plugin-openlist\libs `
        -FrontendVersion v4.0.0

.EXAMPLE
    pwsh -File scripts/build-openlist-aar.ps1 `
        -Output  C:\workspace\app\encv-mobile\plugin-openlist\libs `
        -LocalFrontendDist C:\staging\openlist-frontend-dist

.NOTES
    Required environment:
      - Go 1.25.x          (matches Hi-Sillot fork go.mod)
      - NDK r25c+          (r26b / 26.3.11579264 recommended)
      - Java 17            (Temurin / OpenJDK)
      - cmake, git, curl, tar (tar via bsdtar / 7-Zip)
      - jq                 (only when fork ships public/dist/i18n-overlay/)

    Configuration precedence (highest first):
      1. PowerShell parameters
      2. $env:OPENLIST_FORK_URL / OPENLIST_FORK_BRANCH / OPENLIST_FRONTEND_VERSION
      3. scripts/openlist-fork.env (auto-loaded)
      4. scripts/openlist-fork.env.local (auto-loaded, gitignored)
#>
# TODO: keep NDK version in sync with .github/workflows/build-mpv-lib.yml.
# TODO: Hi-Sillot fork must already contain `openlistlib/` (see
#       .trae/specs/integrate-openlist-as-combolite-plugin/spec.md §一).
# TODO: when adding new ENCV setting items in the fork, bump the version
#       recorded in Hi-Sillot/OpenList/frontend-pinned.txt.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Output,

    [string]$Fork,

    [string]$Branch,

    [string]$Ndk = "$env:ANDROID_HOME\ndk\26.3.11579264",

    [string]$EncvGoRoot = 'C:\workspace',

    [string]$FrontendVersion = '',

    [string]$LocalFrontendDist = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$trackedEnv = Join-Path $scriptDir 'openlist-fork.env'
$localEnv   = Join-Path $scriptDir 'openlist-fork.env.local'

if (Test-Path -LiteralPath $trackedEnv) {
    Get-Content -LiteralPath $trackedEnv | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith('#')) {
            $eq = $line.IndexOf('=')
            if ($eq -gt 0) {
                $k = $line.Substring(0, $eq).Trim()
                $v = $line.Substring($eq + 1).Trim()
                if (-not [string]::IsNullOrEmpty($k) -and -not (Test-Path "env:$k")) {
                    Set-Item -Path "env:$k" -Value $v
                }
            }
        }
    }
}
if (Test-Path -LiteralPath $localEnv) {
    Get-Content -LiteralPath $localEnv | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith('#')) {
            $eq = $line.IndexOf('=')
            if ($eq -gt 0) {
                $k = $line.Substring(0, $eq).Trim()
                $v = $line.Substring($eq + 1).Trim()
                if (-not [string]::IsNullOrEmpty($k)) {
                    Set-Item -Path "env:$k" -Value $v
                }
            }
        }
    }
}

if ([string]::IsNullOrEmpty($Fork))   { $Fork   = if ($env:OPENLIST_FORK_URL)     { $env:OPENLIST_FORK_URL }     else { 'https://github.com/Hi-Sillot/OpenList' } }
if ([string]::IsNullOrEmpty($Branch)) { $Branch = if ($env:OPENLIST_FORK_BRANCH)  { $env:OPENLIST_FORK_BRANCH }  else { 'dev' } }
if ([string]::IsNullOrEmpty($FrontendVersion)) { $FrontendVersion = if ($env:OPENLIST_FRONTEND_VERSION) { $env:OPENLIST_FRONTEND_VERSION } else { '' } }

function Write-Status {
    param([string]$Message)
    Write-Host "[openlist-aar] $Message" -ForegroundColor Cyan
}

function Write-Fatal {
    param([string]$Message)
    Write-Host "[openlist-aar] $Message" -ForegroundColor Red
    exit 1
}

function Resolve-Tool {
    param([string]$Command)
    $found = (Get-Command $Command -ErrorAction SilentlyContinue)
    if (-not $found) {
        Write-Fatal "Required tool '$Command' not found in PATH"
    }
    return $found.Source
}

Write-Status "== fork env =="
Write-Status "  OPENLIST_FORK_BRANCH=$($env:OPENLIST_FORK_BRANCH)"
Write-Status "  OPENLIST_FRONTEND_VERSION=$($env:OPENLIST_FRONTEND_VERSION)"

Write-Status "== Environment check =="
$null = Resolve-Tool 'go'
$null = Resolve-Tool 'java'
$null = Resolve-Tool 'git'
$null = Resolve-Tool 'curl'
$null = Resolve-Tool 'tar'
$null = Resolve-Tool 'cmake'

$hasJq = [bool] (Get-Command 'jq' -ErrorAction SilentlyContinue)
if (-not $hasJq) {
    Write-Status "  (jq not found, will skip i18n overlay merge if fork ships it)"
}

$hasSha256 = [bool] (Get-Command 'sha256sum' -ErrorAction SilentlyContinue)

if (-not (Test-Path -LiteralPath $EncvGoRoot)) {
    Write-Fatal "encv-go root not found: $EncvGoRoot"
}
$EncvGoRoot = (Resolve-Path -LiteralPath $EncvGoRoot).ProviderPath.TrimEnd('\', '/')

if (-not (Test-Path -LiteralPath $Ndk)) {
    $candidates = @(
        (Join-Path $env:ANDROID_HOME 'ndk\26.3.11579264'),
        (Join-Path $env:ANDROID_HOME 'ndk\25.2.9519653'),
        'C:\Android\ndk\26.3.11579264',
        'C:\Android\ndk\25.2.9519653'
    )
    foreach ($cand in $candidates) {
        if ($cand -and (Test-Path -LiteralPath $cand)) {
            $Ndk = $cand
            break
        }
    }
}
if (-not (Test-Path -LiteralPath $Ndk)) {
    Write-Fatal "NDK not found at: $Ndk"
}
$Ndk = (Resolve-Path -LiteralPath $Ndk).ProviderPath
$ndkBuild = Join-Path $Ndk 'ndk-build.cmd'
if (-not (Test-Path -LiteralPath $ndkBuild)) {
    Write-Fatal "ndk-build.cmd not found under $Ndk"
}

if (-not (Test-Path -LiteralPath $Output)) {
    New-Item -ItemType Directory -Path $Output -Force | Out-Null
}
$Output = (Resolve-Path -LiteralPath $Output).ProviderPath

Write-Status "== Toolchain =="
Write-Status "  go         : $(go version)"
Write-Status "  java       : $((java -version) 2>&1 | Select-Object -First 1)"
Write-Status "  NDK        : $Ndk"
Write-Status "  encv-go    : $EncvGoRoot"
Write-Status "  fork       : $Fork@$Branch"
Write-Status "  output dir : $Output"
Write-Status "  frontend-version CLI : $(if ([string]::IsNullOrEmpty($FrontendVersion)) { '<none>' } else { $FrontendVersion })"
Write-Status "  local-frontend-dist  : $(if ([string]::IsNullOrEmpty($LocalFrontendDist)) { '<none>' } else { $LocalFrontendDist })"

# Default fork work dir: <repo-root>\app\openlist\Hi-Sillot-OpenList\. This
# location matches the fork's own go.mod line
# `replace github.com/Soltus/encv-go => ../../../` so the relative replace
# resolves naturally to the encv-go root (no sed patching needed). Override
# with $env:OPENLIST_FORK_WORK_DIR for CI runners that want to reuse a cached
# clone on a separate volume (e.g. D:\cache\fork).
if (-not [string]::IsNullOrEmpty($env:OPENLIST_FORK_WORK_DIR)) {
    $workDir = $env:OPENLIST_FORK_WORK_DIR
} else {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $workDir  = Join-Path (Join-Path $repoRoot 'app\openlist') 'Hi-Sillot-OpenList'
}
$srcDir   = $workDir
if (Test-Path -LiteralPath $srcDir) {
    Remove-Item -LiteralPath $srcDir -Recurse -Force
}
if (-not (Test-Path -LiteralPath $workDir)) {
    New-Item -ItemType Directory -Path $workDir -Force | Out-Null
}

Write-Status "== Clone Hi-Sillot fork (--depth 1) =="
git clone --depth 1 --branch $Branch $Fork $srcDir

$goMod = Join-Path $srcDir 'go.mod'
if (-not (Test-Path -LiteralPath $goMod)) {
    Write-Fatal "go.mod not found in $srcDir"
}

Write-Status "== Verify fork go.mod relative replace resolves correctly =="
# Fork is expected at <repo-root>\app\openlist\Hi-Sillot-OpenList\, so go.mod's
# `replace github.com/Soltus/encv-go => ../../../` resolves to the encv-go
# root. If fork moves to a non-standard location, sed-patch the replace back
# to an absolute path. See D4 in
# .trae/documents/fork-clone-path-refactor-to-app-openlist.md.
$pattern = '^[ \t]*replace[ \t]+github\.com/Soltus/encv-go[ \t]+=>[ \t]+([^\s]+)'
$relReplace = $null
$lines = Get-Content -LiteralPath $goMod
for ($i = 0; $i -lt $lines.Count; $i++) {
    if ($lines[$i] -match $pattern) {
        $relReplace = $lines[$i]
        break
    }
}
if ($relReplace -and ($relReplace -match '^[ \t]*replace[ \t]+github\.com/Soltus/encv-go[ \t]+=>[ \t]+\.\./\.\./\.\./')) {
    Write-Status "  (relative replace detected -> resolves from fork to encv-go root)"
} else {
    if ($relReplace) {
        Write-Status "  WARN: non-relative replace found, fork go.mod has been modified upstream:"
        Write-Status "        $relReplace"
        Write-Status "        sed-patching back to absolute path '$EncvGoRoot' as safety net"
        for ($i = 0; $i -lt $lines.Count; $i++) {
            if ($lines[$i] -match $pattern) {
                $lines[$i] = "replace github.com/Soltus/encv-go => $EncvGoRoot"
                break
            }
        }
        Set-Content -LiteralPath $goMod -Value $lines -Encoding UTF8
    } else {
        Write-Status "  WARN: no encv-go replace line found at all, appending one"
        $lines += "replace github.com/Soltus/encv-go => $EncvGoRoot"
        Set-Content -LiteralPath $goMod -Value $lines -Encoding UTF8
    }
}

Write-Status "== Resolve frontend version =="
$distDir = Join-Path $srcDir 'public\dist'
if (-not (Test-Path -LiteralPath $distDir)) {
    New-Item -ItemType Directory -Path $distDir -Force | Out-Null
}

$pinnedFile = Join-Path $srcDir 'frontend-pinned.txt'
$resolvedFrontendVersion = ''

if (-not [string]::IsNullOrEmpty($LocalFrontendDist)) {
    if (-not (Test-Path -LiteralPath $LocalFrontendDist)) {
        Write-Fatal "--local-frontend-dist path not found: $LocalFrontendDist"
    }
    Write-Status "  source: local dist at $LocalFrontendDist"
    if (Test-Path -LiteralPath $distDir) {
        Remove-Item -LiteralPath $distDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $distDir -Force | Out-Null
    Copy-Item -Path (Join-Path $LocalFrontendDist '*') -Destination $distDir -Recurse -Force
    if (-not (Test-Path -LiteralPath (Join-Path $distDir 'index.html'))) {
        Write-Fatal "local frontend dist missing index.html after copy"
    }
    $resolvedFrontendVersion = if (-not [string]::IsNullOrEmpty($FrontendVersion)) { $FrontendVersion } `
        elseif ($env:OPENLIST_FRONTEND_VERSION) { $env:OPENLIST_FRONTEND_VERSION } `
        else { 'local' }
    Write-Status "  version: $resolvedFrontendVersion (label, not a real upstream tag)"
} else {
    if (Test-Path -LiteralPath $pinnedFile) {
        $pinnedText = Get-Content -LiteralPath $pinnedFile -Raw -ErrorAction SilentlyContinue
        if ($pinnedText) {
            $m = [regex]::Match($pinnedText, 'v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?')
            if ($m.Success) {
                $resolvedFrontendVersion = $m.Value
                Write-Status "  source: fork frontend-pinned.txt"
            }
        }
    }
    if ([string]::IsNullOrEmpty($resolvedFrontendVersion) -and -not [string]::IsNullOrEmpty($FrontendVersion)) {
        $resolvedFrontendVersion = $FrontendVersion
        Write-Status "  source: -FrontendVersion parameter"
    }
    if ([string]::IsNullOrEmpty($resolvedFrontendVersion) -and $env:OPENLIST_FRONTEND_VERSION) {
        $resolvedFrontendVersion = $env:OPENLIST_FRONTEND_VERSION
        Write-Status "  source: OPENLIST_FRONTEND_VERSION env"
    }
    if ([string]::IsNullOrEmpty($resolvedFrontendVersion)) {
        $resolvedFrontendVersion = 'latest'
        Write-Host "[WARN] no frontend pin, using latest" -ForegroundColor Yellow
        Write-Status "  source: fallback (releases/latest) — pin via frontend-pinned.txt to silence this warning"
    }
    Write-Status "  version: $resolvedFrontendVersion"

    if ($resolvedFrontendVersion -eq 'latest') {
        $feApi = 'https://api.github.com/repos/OpenListTeam/OpenList-Frontend/releases/latest'
    } else {
        $feApi = "https://api.github.com/repos/OpenListTeam/OpenList-Frontend/releases/tags/$resolvedFrontendVersion"
    }
    $releaseInfo = (Invoke-RestMethod -Uri $feApi -TimeoutSec 15 -Headers @{ Accept = 'application/vnd.github.v3+json' })
    $asset = $releaseInfo.assets |
        Where-Object { $_.browser_download_url -match 'openlist-frontend-dist.*\.tar\.gz$' } |
        Where-Object { $_.browser_download_url -notmatch 'openlist-frontend-dist-lite' } |
        Select-Object -First 1

    if (-not $asset) {
        Write-Fatal "could not resolve frontend tarball URL for $resolvedFrontendVersion from $feApi"
    }
    $dlUrl = $asset.browser_download_url
    Write-Status "  frontend: $dlUrl"

    $tmpTar = Join-Path $workDir 'openlist-frontend-dist.tar.gz'
    Invoke-WebRequest -Uri $dlUrl -OutFile $tmpTar -TimeoutSec 60
    tar -xzf $tmpTar -C $distDir --strip-components 1
    Remove-Item -LiteralPath $tmpTar -Force

    if (-not (Test-Path -LiteralPath (Join-Path $distDir 'index.html'))) {
        Write-Fatal "frontend dist extraction failed (no index.html)"
    }
}

Write-Status "== Apply i18n overlay =="
$overlayDir = Join-Path $srcDir 'public\dist\i18n-overlay'
if (Test-Path -LiteralPath $overlayDir) {
    if (-not $hasJq) {
        Write-Fatal "i18n-overlay/ exists in fork but jq is not installed"
    }
    $assetsDir = Join-Path $distDir 'assets'
    if (Test-Path -LiteralPath $assetsDir) {
        Get-ChildItem -LiteralPath $overlayDir -Filter 'translation.json' -Recurse -File | ForEach-Object {
            $overlayFile = $_.FullName
            $rel = $overlayFile.Substring($overlayDir.Length).TrimStart('\', '/')
            $lang = $rel.Split([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)[0]
            $target = Join-Path $assetsDir ($lang + '.json')
            if (Test-Path -LiteralPath $target) {
                $tmpFile = [IO.Path]::GetTempFileName()
                try {
                    $merged = (Get-Content -LiteralPath $target -Raw), (Get-Content -LiteralPath $overlayFile -Raw) | ConvertFrom-Json
                    $merged | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath $target -Encoding UTF8
                    Remove-Item -LiteralPath $tmpFile -Force -ErrorAction SilentlyContinue
                    Write-Status "  merged ${lang}: $(Split-Path -Leaf $overlayFile)"
                } catch {
                    if (Test-Path -LiteralPath $tmpFile) { Remove-Item -LiteralPath $tmpFile -Force -ErrorAction SilentlyContinue }
                    Write-Fatal "jq merge failed for ${lang}: $($_.Exception.Message)"
                }
            } else {
                Write-Status "  skipped ${lang}: $target not present in frontend dist"
            }
        }
    } else {
        Write-Status "  (no $assetsDir in frontend dist, skipping overlay merge)"
    }
} else {
    Write-Status "  (no i18n-overlay/ in fork, nothing to merge)"
}

Write-Status "== Write public/dist/VERSION =="
$versionFile = Join-Path $distDir 'VERSION'
Set-Content -LiteralPath $versionFile -Value ($resolvedFrontendVersion + '-encv') -Encoding ascii -NoNewline
Write-Status "  $versionFile = $((Get-Content -LiteralPath $versionFile -Raw).Trim())"

Write-Status "== Set up NDK env =="
if (-not (Test-Path env:ANDROID_HOME) -or [string]::IsNullOrEmpty($env:ANDROID_HOME)) {
    $env:ANDROID_HOME = Split-Path -Path (Split-Path -Path $Ndk -Parent) -Parent
}
$env:ANDROID_NDK_HOME = $Ndk
Write-Status "  ANDROID_HOME=$env:ANDROID_HOME"
Write-Status "  ANDROID_NDK_HOME=$env:ANDROID_NDK_HOME"

Write-Status "== Install / update gomobile =="
$gopathBin = Join-Path (go env GOPATH) 'bin'
if (-not (Test-Path -LiteralPath $gopathBin)) {
    New-Item -ItemType Directory -Path $gopathBin -Force | Out-Null
}
$env:PATH = "$gopathBin;$env:PATH"
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
gomobile init -ndk $Ndk

Set-Location -LiteralPath $srcDir
$bindPkg = $null
$direct = Join-Path $srcDir 'openlistlib'
$cmdSub = Join-Path $srcDir 'cmd\openlistlib'
if ((Test-Path -LiteralPath $direct) -and (Get-ChildItem -LiteralPath $direct -Filter '*.go' -ErrorAction SilentlyContinue | Select-Object -First 1)) {
    $bindPkg = './openlistlib'
} elseif ((Test-Path -LiteralPath $cmdSub) -and (Get-ChildItem -LiteralPath $cmdSub -Filter '*.go' -ErrorAction SilentlyContinue | Select-Object -First 1)) {
    $bindPkg = './cmd/openlistlib'
} else {
    Write-Fatal "Hi-Sillot fork is missing openlistlib/ (see spec §一) and no fallback exists"
}
Write-Status "== gomobile bind (bind pkg: $bindPkg) =="

$builtAt  = Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz'
$gitHash  = (git -C $srcDir rev-parse --short HEAD)
$ldFlags  = '-s -w'
$ldFlags += " -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.Version=$Branch'"
$ldFlags += " -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.WebVersion=$resolvedFrontendVersion'"
$ldFlags += " -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.BuiltAt=$builtAt'"
$ldFlags += " -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.GitAuthor=The OpenList Projects Contributors <noreply@openlist.team>'"
$ldFlags += " -X 'github.com/OpenListTeam/OpenList/v4/internal/conf.GitCommit=$gitHash'"

$outputAar = Join-Path $Output 'openlist.aar'
gomobile bind `
    -ldflags $ldFlags `
    -v `
    -androidapi 19 `
    -target 'android/arm64' `
    -o $outputAar `
    $bindPkg

if (-not (Test-Path -LiteralPath $outputAar) -or ((Get-Item -LiteralPath $outputAar).Length -le 0)) {
    Write-Fatal "openlist.aar was not produced"
}

Write-Status "== Checksum =="
$shaFile = Join-Path $Output 'openlist.aar.sha256'
if ($hasSha256) {
    Push-Location -LiteralPath $Output
    sha256sum openlist.aar | Out-File -FilePath $shaFile -Encoding ascii
    Pop-Location
} else {
    $h = (Get-FileHash -LiteralPath $outputAar -Algorithm SHA256).Hash.ToLower()
    "$h  openlist.aar" | Out-File -FilePath $shaFile -Encoding ascii
}
Get-Content -LiteralPath $shaFile

Write-Status "== Done =="
Write-Status "  AAR  : $outputAar"
$sizeBytes = (Get-Item -LiteralPath $outputAar).Length
$sizeMb    = '{0:N2} MB' -f ($sizeBytes / 1MB)
Write-Status "  SIZE : $sizeMb"
