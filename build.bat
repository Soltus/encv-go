@echo off
setlocal enabledelayedexpansion

echo Starting build via PowerShell...

powershell.exe -ExecutionPolicy Bypass -File "%~dp0build.ps1" -OutputDir "dist" -OpenExplorer

echo.
pause
