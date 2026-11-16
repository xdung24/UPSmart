# UPSmart Monitor

Windows x64 UPS monitor with a Fyne desktop and optional daemon.

## Requirements

- Go 1.26 or newer.
- Windows x64 MinGW-w64 GCC on `PATH` (for example `C:\mingw64\bin`).
- Internet access for the first dependency download.

## Build both apps

In Command Prompt, run:

```bat
build.cmd
```

Outputs: `bin\upsmart-desktop.exe` and `bin\upsmart-next.exe`.
The script works from any directory, stops on errors, and does not start monitoring
or install a service.

## Run

Safe offline window check (no USB or daemon connection):

```bat
start "" /wait bin\upsmart-desktop.exe --smoke-test
```

For normal use, close legacy UPSmart before direct USB polling:

```bat
bin\upsmart-desktop.exe
```

The desktop tries named pipe, localhost web API, then direct USB with exclusive
ownership. No Windows service is required. Web mode is read-only. The web address
is discovered from adjacent/local configuration or set with `--web-address 127.0.0.1:18765`.
Closing hides to the tray; Quit stops only desktop-owned monitoring.

See the [desktop guide](project/F-018-native-desktop.md) for configuration and limitations.
