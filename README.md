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

The desktop connects to the daemon first; otherwise it polls USB directly with
exclusive ownership. Closing hides to the tray; Quit stops only desktop-owned monitoring.

See the [desktop guide](project/F-018-native-desktop.md) for configuration and limitations.
