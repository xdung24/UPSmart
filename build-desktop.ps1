$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    $target = & go env GOOS
    if ($LASTEXITCODE -ne 0 -or $target -ne 'windows') {
        throw 'The desktop build requires a Windows Go toolchain.'
    }
    $cgo = & go env CGO_ENABLED
    if ($LASTEXITCODE -ne 0 -or $cgo -ne '1') {
        throw 'Enable CGO and provide a working GCC compiler before building the desktop.'
    }
    & go build -ldflags '-H=windowsgui -linkmode=internal' -o .\bin\upsmart-desktop.exe .\cmd\upsmart-desktop
    if ($LASTEXITCODE -ne 0) {
        throw 'Desktop build failed.'
    }
    Write-Output 'Built bin\upsmart-desktop.exe. Backend configuration and running processes were not changed.'
}
finally {
    Pop-Location
}
