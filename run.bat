@echo off
title Spectra Server Monitor - Port 5050
cd /d "%~dp0"

echo ==================================================
echo   Starting Spectra Server Monitor...
echo ==================================================
echo.

if not exist "spectra.exe" (
    echo [INFO] Compiling spectra.exe...
    go build -o spectra.exe .
    if %ERRORLEVEL% NEQ 0 (
        echo [ERROR] Failed to compile spectra.exe
        pause
        exit /b %ERRORLEVEL%
    )
)

.\spectra.exe -port 5050

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [ERROR] Spectra stopped with exit code %ERRORLEVEL%
)

echo.
pause
