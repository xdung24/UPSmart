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

Outputs: `bin\upsdesktop.exe` and `bin\upsdaemon.exe`.
The script works from any directory, stops on errors, and does not start monitoring
or install a service.

## Run

Safe offline window check (no USB or daemon connection):

```bat
start "" /wait bin\upsdesktop.exe --smoke-test
```

For normal use, close legacy UPSmart before direct USB polling:

```bat
bin\upsdesktop.exe
```

The desktop connects to a configured daemon through its protected named pipe, or
starts direct USB monitoring when no other process owns the UPS. No Windows
service is required. Service installation configures desktop pipe access for
the installing Windows account automatically. The desktop runs normally without
Administrator rights. Console daemons can enable IPC with `desktop-settings-file`
and optional `desktop-user-sid`. Closing hides to the tray; Quit stops only desktop-owned monitoring.
Remove legacy `listen` and `web_enabled` keys from existing configuration;
the current schema rejects these retired web settings.

See the [desktop guide](project/F-018-native-desktop.md) for configuration and limitations.

## Configuration

Use [config.example.yaml](config.example.yaml) as the monitoring-disabled template.
Copy it to `config.yaml` only if no local configuration already exists.
The daemon follows rdpproxy's option model: **defaults < YAML < CLI**.
`config.yaml` is resolved from the current directory. Select another file with
`--config`, `--conf` or `-c`. Missing files produce a notice and use safe defaults
(UPS polling, notifications and shutdown disabled); malformed files fail.

```bat
bin\upsdaemon.exe --validate -c config.yaml
bin\upsdaemon.exe --config config.yaml --poll-interval 3s
bin\upsdesktop.exe --config config.yaml
```

YAML keys match daemon long flags, using flat hyphenated names:

```yaml
schema-version: 1
poll-interval: 2s
transaction-timeout: 1s
stale-after: 10s
monitoring-enabled: false
```

Use `--monitoring-enabled` to enable polling or `--monitoring-enabled=false` to
override YAML `true`. Repeat list flags to append entries. Run
`bin\upsdaemon.exe --help` for all parameters and `--version` for the build version.
Valid legacy nested/underscore configuration remains readable; newly generated
files use the flat format. `serve` and `validate` remain console aliases.

The desktop's `--config` applies only to direct USB mode; an attached daemon
keeps its own configuration. Without `--config`, standalone desktop defaults
remain unchanged. An explicit missing desktop config still fails, rather than
enabling direct USB defaults. Configuration allows YAML comments and requires one document;
unknown fields and duplicate keys are rejected. Desktop-saved policy state and
IPC payloads remain JSON.

## Optional Windows service

From an Administrator terminal, after deliberately stopping console monitoring:

```bat
bin\upsdaemon.exe --install --config config.yaml
bin\upsdaemon.exe --start
bin\upsdaemon.exe --status
bin\upsdaemon.exe --stop
bin\upsdaemon.exe --remove
```

These are separate commands, not a script to run together. Service `UPSmart`
uses **LocalSystem**, as explicitly selected to match rdpproxy. Installation
validates merged settings and writes flat YAML, including CLI overrides, beside
`%ProgramFiles%\UPSmart\upsdaemon.exe`, retaining the selected config basename.
SCM receives only `--config` and that absolute path. Logs remain under
`%ProgramData%\UPSmart\logs`. Windows folder permissions are inherited/preserved;
the previous exact-ACL enforcement and LocalService deployment are retired.

The installed config automatically includes desktop IPC authorization for the
installing account and the service policy path. If installation uses a different
Administrator account, explicitly set `desktop-user-sid` to the intended desktop
user and `desktop-settings-file` to `C:\ProgramData\UPSmart\logs\desktop-policy.json`.
No Everyone/Users grant or desktop elevation is required. Existing saved service
power policy requires explicit review/selection rather than automatic activation.
This setup does not enable shutdown controls.

Installation does not start the service. An existing registration must be
stopped/removed before reinstalling. Removal deletes the installed binary but
preserves configuration/history; run it from the portable binary shown above.
If the source config is missing while an installed copy exists, installation
requires selecting that existing copy explicitly rather than resetting it.
The old `service ...` subcommands are replaced by the flags above.
See [service setup and verification limits](project/F-006-windows-service.md).
An older installed config without desktop settings needs a one-time elevated
update/restart or reinstall; running the desktop as Administrator is not a fix.

## Notifications

Optional Telegram notifications can use direct bot access or rdpwatcher's gRPC
relay. For the relay, configure `notify-addr` (for example
`127.0.0.1:50061`) and `telegram-events`; omit direct bot credentials, chat ID and command
allowlist. rdpwatcher chooses the destination group. Its relay is plaintext and
unauthenticated, so prefer loopback. Restart UPSmart after configuration changes.
See [notification setup and limitations](project/F-017-telegram-notifications.md#optional-rdpwatcher-relay---2026-11-16).
