@echo off
chcp 65001 > nul
title Phat Hanh Ban Moi - TrafficTool

REM ============================================================================
REM  PhatHanhBanMoi.bat - Tu dong tang version (v1.0.0 -> v1.0.1), build 
REM  va upload len GitHub Release.
REM ============================================================================

cd /d "%~dp0"

set BUMP=%1

if "%BUMP%"=="" (
    echo.
    echo ===========================================================
    echo           CHON LOAI PHAT HANH PHIEN BAN (SemVer)
    echo ===========================================================
    echo.
    echo   [1] PATCH (v1.0.x) - Sua loi, Fix bug, Chinh giao dien nho [Mac dinh]
    echo   [2] MINOR (v1.x.0) - Them tinh nang/chuc nang moi
    echo   [3] MAJOR (vX.0.0) - Dai tu toan bo app, thay doi lon
    echo.
    set /p CHOICE="Nhap luon chon (1, 2, 3) hoac an Enter de chon [1]: "
    
    if "%CHOICE%"=="2" (
        set BUMP=minor
    ) else if "%CHOICE%"=="3" (
        set BUMP=major
    ) else (
        set BUMP=patch
    )
)

echo.
echo ===========================================================
echo   PHAT HANH BAN MOI (loai: %BUMP%)
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
