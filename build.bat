@echo off
setlocal

REM Detect build metadata.
for /f %%i in ('git rev-parse --short HEAD 2^>nul') do set COMMIT=%%i
if "%COMMIT%"=="" set COMMIT=none

for /f %%d in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-dd" 2^>nul') do set BUILD_DATE=%%d
if "%BUILD_DATE%"=="" set BUILD_DATE=unknown

if "%VERSION%"=="" set VERSION=2.0.1

set LDFLAGS=-s -w -X cc-switch-cli/internal/version.Version=%VERSION% -X cc-switch-cli/internal/version.Commit=%COMMIT% -X cc-switch-cli/internal/version.Date=%BUILD_DATE%

set GOOS=windows
set GOARCH=amd64

go build -ldflags="%LDFLAGS%" -o cc-switch-cli.exe .
if not %errorlevel% equ 0 (
    echo Build failed for cc-switch-cli.exe
    exit /b 1
)

go build -ldflags="%LDFLAGS%" -o ccs.exe .
if not %errorlevel% equ 0 (
    echo Build failed for ccs.exe
    exit /b 1
)

echo Built cc-switch-cli.exe and ccsc.exe ^(v%VERSION%, commit %COMMIT%, %BUILD_DATE%^)
endlocal
