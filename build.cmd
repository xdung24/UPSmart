@echo off
setlocal
pushd "%~dp0"
if errorlevel 1 exit /b 1

set "GOOS=windows"
set "GOARCH=amd64"
set "CC=gcc"
set "CGO_ENABLED=1"
if not exist "bin" mkdir "bin"
if errorlevel 1 goto failed

echo Building desktop...
go build -ldflags "-H=windowsgui -linkmode=internal" -o ".\bin\upsmart-desktop.exe" .\cmd\upsmart-desktop
if errorlevel 1 goto failed

echo Building daemon...
set "CGO_ENABLED=0"
go build -o ".\bin\upsmart-next.exe" .\cmd\ups-daemon
if errorlevel 1 goto failed

echo Built bin\upsmart-desktop.exe and bin\upsmart-next.exe
popd
exit /b 0

:failed
echo Build failed. See the error above. 1>&2
popd
exit /b 1
