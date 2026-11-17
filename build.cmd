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
go build -ldflags "-H=windowsgui -linkmode=internal -extldflags=-static -s -w" -o ".\bin\upsdesktop.exe" .\cmd\upsdesktop
if errorlevel 1 goto failed

echo Building daemon...
set "CGO_ENABLED=0"
go build  -ldflags "-linkmode=internal -extldflags=-static -s -w" -o ".\bin\upsdaemon.exe" .\cmd\upsdaemon
if errorlevel 1 goto failed

echo Built bin\upsdesktop.exe and bin\upsdaemon.exe
popd
exit /b 0

:failed
echo Build failed. See the error above. 1>&2
popd
exit /b 1
