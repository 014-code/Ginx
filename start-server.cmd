@echo off
setlocal
where pwsh.exe >nul 2>nul
if errorlevel 1 (
    echo PowerShell 7 is required. Install it, then run this script again.
    pause
    exit /b 1
)
pwsh.exe -NoLogo -NoProfile -File "%~dp0start-server.ps1" %*
set "launchExit=%ERRORLEVEL%"
if not "%launchExit%"=="0" echo Server startup failed. See the error above.
pause
exit /b %launchExit%
