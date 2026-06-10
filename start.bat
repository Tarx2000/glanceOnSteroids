@echo off
cd /d "%~dp0"
set /p BUILD=<BUILD_NUMBER
echo Building b%BUILD%...
go build -ldflags "-s -w -X github.com/glanceapp/glance/internal/glance.buildNumber=%BUILD%" -trimpath -o glance.exe .
if %errorlevel% neq 0 (
    echo Build failed!
    pause
    exit /b 1
)
powershell -Command "$c = Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Subject -eq 'CN=Taris Dashboard' } | Select-Object -First 1; if ($c) { & 'C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\signtool.exe' sign /sha1 $c.Thumbprint /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 '%~dp0glance.exe' } else { Write-Host 'No code signing cert found, skipping signing' }" 2>nul
powershell -Command "Unblock-File -LiteralPath '%~dp0glance.exe'" 2>nul
echo Starting b%BUILD%...
glance.exe
