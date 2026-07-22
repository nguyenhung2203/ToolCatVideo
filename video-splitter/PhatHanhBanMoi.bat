@echo off
chcp 65001 > nul
title Phat Hanh Ban Moi - TrafficTool

REM ============================================================================
REM  PhatHanhBanMoi.bat - Tu dong tang version (v1.0.0 -> v1.0.1), build 
REM  va upload len GitHub Release.
REM ============================================================================

cd /d "%~dp0"

set BUMP=%1
if "%BUMP%"=="" set BUMP=patch

echo.
echo ===========================================================
echo   PHAT HANH BAN MOI (tang version: %BUMP%)
echo ===========================================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0release.ps1" -Bump %BUMP%

echo.
if %ERRORLEVEL% NEQ 0 (
    echo [X] Phat hanh THAT BAI. Xem loi o tren.
) else (
    echo [OK] Da phat hanh xong! Khach hang bam "Kiem tra cap nhat" la co ban moi.
)
echo.
pause
