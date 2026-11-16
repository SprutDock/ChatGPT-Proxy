@echo off
setlocal
powershell.exe -NoLogo -NoProfile -File "%~dp0Start-ChatGPT-Proxy.ps1"
set "launcherExitCode=%ERRORLEVEL%"
if not "%launcherExitCode%"=="0" (
    echo.
    echo ChatGPT Proxy launcher failed with exit code %launcherExitCode%.
    pause
)
exit /b %launcherExitCode%
