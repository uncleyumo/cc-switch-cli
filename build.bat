@echo off
setlocal EnableExtensions

REM Build metadata (override VERSION from outside: set VERSION=2.0.2 && build.bat)
for /f %%i in ('git rev-parse --short HEAD 2^>nul') do set "COMMIT=%%i"
if not defined COMMIT set "COMMIT=none"

for /f %%d in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-dd" 2^>nul') do set "BUILD_DATE=%%d"
if not defined BUILD_DATE set "BUILD_DATE=unknown"

if not defined VERSION set "VERSION=2.1.2"

set "LDFLAGS=-s -w -X cc-switch-cli/internal/version.Version=%VERSION% -X cc-switch-cli/internal/version.Commit=%COMMIT% -X cc-switch-cli/internal/version.Date=%BUILD_DATE%"
set "CGO_ENABLED=0"

REM Remove legacy root-level binaries (dev leftovers, not for distribution)
if exist "%~dp0cc-switch-cli.exe" del /f /q "%~dp0cc-switch-cli.exe"
if exist "%~dp0ccs.exe" del /f /q "%~dp0ccs.exe"
if exist "%~dp0ccsc.exe" del /f /q "%~dp0ccsc.exe"

if not exist "%~dp0dist" mkdir "%~dp0dist"
del /f /q "%~dp0dist\*" 2>nul

echo Building v%VERSION% (commit %COMMIT%, %BUILD_DATE%)
echo.

call :build windows amd64 .exe
if errorlevel 1 goto :fail
call :build windows arm64 .exe
if errorlevel 1 goto :fail

call :build linux amd64
if errorlevel 1 goto :fail
call :build linux arm64
if errorlevel 1 goto :fail

call :build darwin amd64
if errorlevel 1 goto :fail
call :build darwin arm64
if errorlevel 1 goto :fail

echo.
echo Artifacts:
dir /b "%~dp0dist"
echo.
echo Done. Output directory: dist\
endlocal
exit /b 0

:build
set "GOOS=%~1"
set "GOARCH=%~2"
set "EXT=%~3"
set "OUT=%~dp0dist\cc-switch-cli_%VERSION%_%GOOS%_%GOARCH%%EXT%"
echo   %GOOS%/%GOARCH%  -^>  dist\cc-switch-cli_%VERSION%_%GOOS%_%GOARCH%%EXT%
go build -ldflags="%LDFLAGS%" -o "%OUT%" "%~dp0."
if errorlevel 1 (
    echo Build failed: %GOOS%/%GOARCH%
    exit /b 1
)
exit /b 0

:fail
echo.
echo Build failed.
endlocal
exit /b 1
