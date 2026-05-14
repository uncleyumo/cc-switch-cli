@echo off
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o cc-switch-cli.exe .
if %errorlevel% equ 0 (
    echo Built cc-switch-cli.exe
) else (
    echo Build failed
    exit /b 1
)
