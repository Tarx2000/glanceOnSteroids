@echo off
:: Set working directory to the batch script location
cd /d "%~dp0"

echo [Prep] Checking for running instances of glance.exe...
:: Forcefully terminate any existing glance.exe background process to release the port (8086) and unlock the binary file.
taskkill /f /im glance.exe >nul 2>&1

:: Read the current build version number from the BUILD_NUMBER file
set /p BUILD=<BUILD_NUMBER
echo Building b%BUILD%...

:: Compile the Go binary with build flags to embed version details
go build -ldflags "-s -w -X github.com/glanceapp/glance/internal/glance.buildNumber=%BUILD%" -trimpath -o glance.exe .
if %errorlevel% neq 0 (
    echo [Error] Build failed!
    pause
    exit /b 1
)

:: Try to code-sign the binary if a development signing certificate is present in the CurrentUser certificate store
powershell -Command "$c = Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Subject -eq 'CN=Taris Dashboard' } | Select-Object -First 1; if ($c) { & 'C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\signtool.exe' sign /sha1 $c.Thumbprint /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 '%~dp0glance.exe' } else { Write-Host 'No code signing cert found, skipping signing' }" 2>nul

:: Unblock the file to bypass Windows Defender SmartScreen warning on launch
powershell -Command "Unblock-File -LiteralPath '%~dp0glance.exe'" 2>nul

echo Starting b%BUILD%...

:: Extract the server port from glance.yml dynamically (fallback to 8086)
set "PORT=8086"
for /f "usebackq delims=" %%p in (`powershell -Command "$p = (Get-Content 'glance.yml' | Select-String -Pattern '^\s*port:\s*(\d+)' | Select-Object -First 1).Matches.Groups[1].Value; if ($p) { $p } else { '8086' }"`) do set "PORT=%%p"

echo.
echo ==================================================
echo   Dashboard is available at http://localhost:%PORT%
echo ==================================================
echo.
:: Launch the dashboard application
glance.exe

:: If glance.exe exits or crashes, catch the exit status and keep the window open so logs can be inspected
echo.
echo ==================================================
echo [Status] Application exited with code %errorlevel%
echo ==================================================
pause
