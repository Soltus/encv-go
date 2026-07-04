# Windows 构建脚本
param(
    [string]$OutputDir = "dist",
    [bool]$OpenExplorer = $true
)

Write-Host "Starting build process... Output directory: $OutputDir" -ForegroundColor Green
# 检查并创建输出目录
if (-not (Test-Path -Path $OutputDir)) {
    Write-Host "Creating '$OutputDir' directory..."
    New-Item -ItemType Directory -Force -Path $OutputDir
}

# 构建所有交付产物
# 编译 encv
Write-Host "Building encv.exe..."
go build -o "$OutputDir\encv.exe" ./cmd/encv
if ($LASTEXITCODE -ne 0) {
    Write-Host "Failed to build encv.exe" -ForegroundColor Red
    exit 1
}

# 复制配置和文档文件
Write-Host "Copying necessary files..."
Copy-Item -Path "config.user.json" -Destination "$OutputDir\config.user.json"
Copy-Item -Path "README.md" -Destination "$OutputDir\README.md"

Write-Host "--------------------------------------------------" -ForegroundColor Green
Write-Host "All binaries and files built successfully in ./$OutputDir/" -ForegroundColor Green
Write-Host "--------------------------------------------------" -ForegroundColor Green
if ($OpenExplorer) {
    Write-Host "Opening output folder..."
    Invoke-Item "$OutputDir"
}
