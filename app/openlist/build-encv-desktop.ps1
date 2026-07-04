# OpenList + ENCV 桌面应用构建脚本
# 用法: .\build-encv-desktop.ps1

$ErrorActionPreference = "Stop"

# 【关键】立即保存所有路径，防止后续命令修改变量
$BuildRoot = if ($PSScriptRoot) { $PSScriptRoot } else { (Get-Location).Path }
$DesktopPath = Join-Path $BuildRoot "OpenList-Desktop"
$ExeSourcePath = Join-Path $DesktopPath "src-tauri\target\release\openlist-desktop.exe"
$BinarySourcePath = Join-Path $DesktopPath "src-tauri\binary"
$PortableDir = Join-Path $BuildRoot "dist\OpenList-Desktop-Portable"
$PortableBinaryDir = Join-Path $PortableDir "binary"
$ExeTargetPath = Join-Path $PortableDir "openlist-desktop.exe"

# 【关键】查找已构建的 OpenList exe 文件
$OpenListDistDir = Join-Path $BuildRoot "OpenList\dist\windows"
$OpenListExe = Get-ChildItem -Path $OpenListDistDir -Filter "*.exe" | Select-Object -First 1
if (-not $OpenListExe) {
    throw "No .exe file found in $OpenListDistDir"
}
$OpenListBuildPath = $OpenListExe.FullName

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "OpenList + ENCV Desktop Build Script" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "OpenListBuildPath: $OpenListBuildPath"
Write-Host "DesktopPath: $DesktopPath"
Write-Host "ExeSourcePath: $ExeSourcePath"
Write-Host "PortableDir: $PortableDir"

# 步骤 1: 验证 OpenList 构建文件
Write-Host "`n[1/5] Verifying OpenList build..." -ForegroundColor Yellow
if (-not (Test-Path $OpenListBuildPath)) {
    throw "OpenList build not found at: $OpenListBuildPath"
}
Write-Host "OpenList build found: $OpenListBuildPath" -ForegroundColor Green

# 步骤 2: 准备桌面应用依赖
Write-Host "`n[2/5] Preparing OpenList-Desktop dependencies..." -ForegroundColor Yellow
Set-Location $DesktopPath

$nodeVersion = node --version
Write-Host "Node.js version: $nodeVersion"

if (-not (Test-Path (Join-Path $DesktopPath "node_modules"))) {
    Write-Host "Installing npm dependencies..."
    yarn install
}

# 步骤 3: 使用本地 OpenList 构建桌面应用
Write-Host "`n[3/5] Preparing OpenList-Desktop with local OpenList..." -ForegroundColor Yellow

$env:USE_LOCAL_OPENLIST = "true"
$env:LOCAL_OPENLIST_PATH = $OpenListBuildPath

node scripts/prepare.js --local-openlist

# 步骤 4: 构建生产版本（只编译，不打包）
Write-Host "`n[4/5] Building production binary..." -ForegroundColor Yellow
yarn tauri build --no-bundle

# 步骤 5: 打包便携版
Write-Host "`n[5/5] Creating portable version..." -ForegroundColor Yellow
Write-Host "ExeSourcePath: $ExeSourcePath" -ForegroundColor Gray
Write-Host "BinarySourcePath: $BinarySourcePath" -ForegroundColor Gray

if (-not (Test-Path $ExeSourcePath)) {
    throw "Build failed: openlist-desktop.exe not found at $ExeSourcePath"
}
if (-not (Test-Path $BinarySourcePath)) {
    throw "Build failed: binary directory not found at $BinarySourcePath"
}

New-Item -ItemType Directory -Force -Path $PortableDir | Out-Null
New-Item -ItemType Directory -Force -Path $PortableBinaryDir | Out-Null

Copy-Item -Path $ExeSourcePath -Destination $ExeTargetPath -Force
Write-Host "Copied: openlist-desktop.exe" -ForegroundColor Green

Copy-Item -Path "$BinarySourcePath\*" -Destination $PortableBinaryDir -Recurse -Force
Write-Host "Copied: binary/" -ForegroundColor Green

Write-Host "`n========================================" -ForegroundColor Green
Write-Host "Build complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "Portable version: $PortableDir" -ForegroundColor Cyan
Write-Host "`nFiles:" -ForegroundColor White
Write-Host "  - openlist-desktop.exe" -ForegroundColor White
Write-Host "  - binary/openlist-x86_64-pc-windows-msvc.exe" -ForegroundColor White
Write-Host "  - binary/rclone-x86_64-pc-windows-msvc.exe" -ForegroundColor White

Set-Location $BuildRoot
