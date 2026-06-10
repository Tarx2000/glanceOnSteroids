@echo off
echo Adding Taris Dashboard certificate to Trusted Root Store...
echo.
certutil -addstore Root "C:\Users\Tarik\AppData\Local\Temp\opencode\taris-cert.cer"
echo.
if %errorlevel% equ 0 (
    echo Success! You can now close this window.
) else (
    echo This must be run as Administrator. Right-click this file and select "Run as administrator".
)
pause
