# SVG, PNG, WebP and ICO command-line tool

## Downloads

- ImageMagick: https://github.com/imagemagick/imagemagick
- Windows packages: https://imagemagick.org/script/download.php

ImageMagick is used for SVG rendering and for producing PNG, WebP and
multi-resolution Windows ICO files.

## Verify installation

```cmd
magick -version
```

If the command is not found, reopen Command Prompt after installation or add the
ImageMagick installation directory to `PATH`.

## SVG to PNG

Create a transparent 256×256 PNG with ImageMagick:

```cmd
magick -background none car-battery.svg -resize 256x256 -strip car-battery.png
```

Create a 32×32 PNG:

```cmd
magick -background none car-battery.svg -resize 32x32 -strip car-battery-32.png
```

## SVG to WebP

Create an optimized transparent 256×256 WebP:

```cmd
magick -background none car-battery.svg -resize 256x256 -quality 90 -strip car-battery.webp
```

Create lossless WebP, recommended for crisp line icons:

```cmd
magick -background none car-battery.svg -resize 256x256 -define webp:lossless=true -strip car-battery.webp
```

Convert an existing PNG to WebP:

```cmd
magick car-battery.png -quality 90 -strip car-battery.webp
```

## SVG to multi-resolution ICO

Let ImageMagick generate standard Windows icon sizes directly:

```cmd
magick -background none car-battery.svg -define icon:auto-resize=16,32,48,64,128,256 car-battery.ico
```

## Explicit multi-resolution ICO workflow

Use this workflow when each size needs to be inspected separately:

```cmd
magick -background none car-battery.svg -resize 16x16 -strip icon-16.png
magick -background none car-battery.svg -resize 32x32 -strip icon-32.png
magick -background none car-battery.svg -resize 48x48 -strip icon-48.png
magick -background none car-battery.svg -resize 64x64 -strip icon-64.png
magick -background none car-battery.svg -resize 128x128 -strip icon-128.png
magick -background none car-battery.svg -resize 256x256 -strip icon-256.png

magick icon-16.png icon-32.png icon-48.png icon-64.png icon-128.png icon-256.png car-battery.ico
```

## Inspect generated files

Inspect the PNG size:

```cmd
magick identify car-battery.png
```

Inspect the WebP:

```cmd
magick identify car-battery.webp
```

List every image contained in the ICO:

```cmd
magick identify car-battery.ico
```

## Recommended UPSmart icon workflow

```cmd
magick -background none car-battery.svg -resize 256x256 -strip internal\desktop\car-battery.png
magick -background none car-battery.svg -define icon:auto-resize=16,32,48,64,128,256 car-battery.ico
magick -background none car-battery.svg -resize 256x256 -define webp:lossless=true -strip car-battery.webp
```

## Generate the Windows resource with go-winres

The `go-winres` tool is available at:
https://github.com/tc-hib/go-winres

Install it:

```cmd
go install github.com/tc-hib/go-winres@latest
```

From the UPSmart repository root, create the `winres` directory and icon
configuration explicitly. The installed `go-winres` version may not create a
JSON file with the same command on every shell:

```cmd
if not exist winres mkdir winres
copy /Y car-battery.ico winres\car-battery.ico
(
echo {
echo   "RT_GROUP_ICON": {
echo     "APP": {
echo       "0000": "car-battery.ico"
echo     }
echo   }
echo }
)>winres\winres.json
```

The resulting `winres\winres.json` must contain:

```json
{
  "RT_GROUP_ICON": {
    "APP": {
      "0000": "car-battery.ico"
    }
  }
}
```

The icon path is relative to the `winres` directory.

Generate Windows resource objects for the desktop package:

```cmd
go-winres make --in winres\winres.json --arch amd64 --out cmd\upsdesktop\rsrc
dir cmd\upsdesktop\rsrc_windows_*.syso
```

This creates files named `rsrc_windows_*.syso`; Go automatically links them into
Windows builds of `cmd\upsdesktop`.

Build and verify the desktop:

```cmd
go build -ldflags "-H=windowsgui -linkmode=internal -extldflags=-static -s -w" -o bin\upsdesktop.exe .\cmd\upsdesktop
```

When only the ICO changes, regenerate the resource before rebuilding:

```cmd
copy /Y car-battery.ico winres\car-battery.ico
go-winres make --in winres\winres.json --arch amd64 --out cmd\upsdesktop\rsrc
go build -ldflags "-H=windowsgui -linkmode=internal -extldflags=-static -s -w" -o bin\upsdesktop.exe .\cmd\upsdesktop
```
