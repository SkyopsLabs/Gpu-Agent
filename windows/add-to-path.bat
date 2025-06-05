@echo off
REM Add SkyOps to PATH environment variable
REM This script is run by the MSI installer

echo Adding SkyOps to PATH...

REM Get current PATH value
for /f "usebackq tokens=2*" %%A in (`reg query "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v PATH 2^>nul`) do set CurrentPath=%%B

REM Check if SkyOps is already in PATH
echo %CurrentPath% | findstr /i /c:"%~1" >nul
if %errorlevel% equ 0 (
    echo SkyOps is already in PATH
    exit /b 0
)

REM Add SkyOps to PATH
set NewPath=%CurrentPath%;%~1
reg add "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v PATH /t REG_EXPAND_SZ /d "%NewPath%" /f >nul

if %errorlevel% equ 0 (
    echo SkyOps successfully added to PATH
    echo Please restart your command prompt to use the skyops command
) else (
    echo Failed to add SkyOps to PATH. You may need to add it manually:
    echo %~1
)

exit /b %errorlevel%
