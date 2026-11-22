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
if not exist "winres" mkdir "winres"
if errorlevel 1 goto failed

where magick >nul 2>nul
if errorlevel 1 goto missing_magick
where go-winres >nul 2>nul
if errorlevel 1 goto missing_winres
if not exist "winres\winres.json" goto missing_winres_config

echo Generating desktop icons...
magick -background none car-battery.svg -resize 256x256 -strip internal\desktop\car-battery.png
if errorlevel 1 goto failed
magick -background none car-battery.svg -define icon:auto-resize=16,32,48,64,128,256 car-battery.ico
if errorlevel 1 goto failed
copy /Y car-battery.ico winres\car-battery.ico >nul
if errorlevel 1 goto failed
go-winres make --in winres\winres.json --arch amd64 --out cmd\upsdesktop\rsrc
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

:missing_magick
echo Build failed: ImageMagick "magick" is not available. See ICON.md. 1>&2
popd
exit /b 1

:missing_winres
echo Build failed: "go-winres" is not available. See ICON.md. 1>&2
popd
exit /b 1

:missing_winres_config
echo Build failed: winres\winres.json is missing. See ICON.md. 1>&2
popd
exit /b 1
