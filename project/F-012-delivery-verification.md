# F-012 - Delivery and verification

Updated: 2026-11-16 | Priority: P1 | Status: Separate Windows amd64 desktop and daemon builds verified

## Purpose and user journey

Mục tiêu: cài, kiểm tra và gỡ ứng dụng dễ dàng; biết rõ phần nào đã chạy thật với UPS và phần nào mới mô phỏng.

The user downloads/builds one executable, runs monitoring in console mode, compares readings, then approves service installation.

## Implementation

- Current backend entry point: `cmd/upsdaemon/main.go`; desktop: `cmd/upsdesktop/main.go`. Earlier root-entry commands below record the previous layout.
- Current daemon delivery follows rdpproxy-style direct flags, flat YAML and LocalSystem service registration as explicitly chosen on 2026-11-16; see [F-006](F-006-windows-service.md#current-service-model-rdpproxy-alignment---2026-11-16) and [F-008](F-008-configuration.md#current-rdpproxy-style-options---2026-11-16). Full tests/vet, both Windows builds, built help and offline validation of example/root/bin configs passed. Local polling values were preserved; no service or monitor was started.
- Target Windows amd64 with Go modules; commit dependency checksums if third-party dependencies are introduced. Prefer native Windows HID calls and avoid mandatory vendor DLLs, external web assets or CGO unless protocol evidence requires them.
- Provide version/build information, example monitoring-only configuration, CLI help and administrator install/uninstall instructions.
- Keep instructions as flat project feature documents; put lifecycle usage in [F-006](F-006-windows-service.md), configuration usage in [F-008](F-008-configuration.md) and quality evidence in [F-015](F-015-quality.md).
- Verification compares protocol replies and the new UI against timestamped UPSmart observations. If simultaneous polling is unsafe, compare separate stable-state sessions and note the limitation.
- Record actual Windows version, architecture, UPS identity, selected protocol, relevant firmware if available and test date.
- Publish known unsupported fields and operations. No production mock data, silent simulation fallback or "all models supported" claim.
- Update by explicitly stopping the service, preserving configuration/history, replacing the executable and restarting; rollback must preserve compatible data or clearly report schema incompatibility.

## Acceptance and tests

- REL-01: amd64 executable runs without UPSmart DLLs and without internet.
- REL-02: clean monitoring startup with hardware absent still serves explicit disconnected status.
- REL-03: validated hardware readings match independently decoded captures within device precision; discrepancies are documented.
- REL-04: approved service install, restart, update and uninstall preserve intended configuration/history.
- REL-05: release notes distinguish simulated, hardware-verified and unavailable functionality.
- REL-06: no arguments starts console mode using working-directory YAML over monitoring-disabled defaults. A missing file emits a notice; invalid existing configuration fails. Help/version/validation do not start hardware access.

## Decisions and boundaries

Go is confirmed. Supported Windows-version range, signing and distribution format remain open. No installer, administrative action or dependency installation is authorized by this specification alone.

## Results and next steps

First milestone, 2026-11-12: built `bin/upsmart.exe` with Go 1.26.5 on Windows 10 Pro 10.0.19045 amd64 using only the standard library/system DLLs. Its live monitoring/browser evidence is historical and does not establish service-account behavior.

Persistence/service candidate: built `bin/upsdaemon.exe` without replacing the original. It adds pinned `golang.org/x/sys v0.50.0` and `go.sum` for maintained Windows service/security APIs. Assets remain embedded; no vendor DLL or external frontend runtime is required.

```powershell
go build -o .\bin\upsdaemon.exe .
# Future approved test only: deliberately stop the current monitor/legacy UPSmart first.
.\bin\upsdaemon.exe serve --config .\config.local.json
```

Use the configuration instructions in [F-008](F-008-configuration.md). With this machine's local configuration, open `http://127.0.0.1:18765`. Do not run legacy UPSmart and this monitor together; the replacement refuses detected legacy processes. No automatic shutdown protection is provided by this milestone.

Actual live status/API/browser tests passed. Capacity remains unavailable; temperature sensor location and UPSmart parity are unverified. Temporary test monitoring was stopped afterward; users can resume legacy UPSmart when not testing the replacement.

The user selected build-only for this continuation: no executable swap, monitor restart, local configuration edit, persistent logging enablement or service registration was performed. Instructions for future elevated service use are in [F-006](F-006-windows-service.md); actual deployment remains unverified.

Assessment: candidate code/build and filesystem/handler tests pass; this is not an installed background service or production release. Version metadata, clean-machine delivery, dependency license notices for redistribution, signing, runtime update verification and REL-04 remain pending.

### Default startup and usage - 2026-11-12

Historical default-path/missing-file behavior below was superseded by the 2026-11-16 rdpproxy alignment linked above. Current console commands consistently use working-directory `config.yaml` over safe defaults.

The user explicitly chose to start monitoring when no arguments are supplied. After the 2026-11-16 [YAML migration](F-008-configuration.md#yaml-configuration---2026-11-16), current no-argument daemon startup selects `serve --config .\config.yaml`:

```powershell
go run .\cmd\upsdaemon
.\bin\upsdaemon.exe
```

Air's existing empty argument list also follows this behavior; no Air configuration change was needed. Paths are relative to the process's current working directory, not the executable directory. The configuration's `monitoring_enabled` setting remains authoritative. Missing/invalid local configuration is an error, never a silent fallback to another policy.

`serve`/`validate` without `--config` now use `%ProgramData%\UPSmart\config.yaml`. Root `help`, `-h` and `--help` show readable commands and safety notes. Subcommand flag help also exits successfully, without a timestamped error. Older JSON paths in the dated test records below describe historical runs, not current defaults.

Four focused root-package tests passed: valid default startup invokes the injected monitor exactly once and propagates its outcome; invalid/missing local files produce errors; explicit validation uses its chosen file despite a broken local file. Tests do not start an actual listener or touch the UPS. Root vet, candidate build and actual `--help`/`serve --help` checks passed with exit status zero.

The candidate was rebuilt only. No configuration edits, running-monitor replacement/restart or service registration was performed.

### Optional monitoring candidate - 2026-11-12

The separate candidate now includes opt-in hardware/Telegram workers and their cached UI/API integration. Existing local/example configurations are unchanged. See [F-008](F-008-configuration.md) for proposed additions; do not enable Telegram without deliberately configuring its destination and token environment.

`upsmart read-hardware` reads PC metrics without acquiring USB ownership, starting a server or querying the UPS. CPU delta warm-up is bounded; helper collection has an eight-second total deadline. Partial provider results are printed with explicit reasons; provider errors also produce a nonzero exit status, not a false success. This diagnostic can run while the existing UPS monitor remains unchanged.

Full tests, vet and Windows candidate build passed. Existing persistence/service regression tests passed five repeated runs. No installed-service upgrade or actual background-monitor deployment was performed.

The historical cputemp adaptation and its embedded notice/footer link were subsequently removed with all PC temperature monitoring, as explicitly requested by the user. No cputemp-derived code remains in the current candidate. Its previous unsupported-source evidence is retained in [E-005](E-005-acpi-temperature-unsupported.md), closed by scope removal rather than a claimed sensor fix.

### Fyne native desktop candidate - 2026-11-12

Separate `bin/upsdesktop.exe` builds using Fyne/CGO and the supported Go internal linker; the current direct command is in [F-018](F-018-native-desktop.md). The older external MinGW link fails Windows loading in this environment ([E-006](E-006-native-desktop-linker.md)). An actual native-window smoke launch exited 0 with backend polling disabled. Backend `bin/upsdaemon.exe` also builds with CGO disabled.

Build/startup evidence and exact connection configuration are in [F-018](F-018-native-desktop.md). GUI startup is not evidence that the existing backend was upgraded or a service installed: local configuration/processes remain unchanged. Daemon mode uses the configured named pipe; direct mode can monitor independently. Windows service installation is optional. Charts, general settings editors, visual/tray interaction, clean-machine redistribution/signing and LocalService rights remain pending; shutdown policy settings/controls are implemented.

The concurrent daemon relocation was preserved. Build the current backend with `go build -o .\bin\upsdaemon.exe .\cmd\upsdaemon`, not the historical root-package command.

Windows build shortcut - 2026-11-12: root [build.cmd](../build.cmd) builds both candidates with local environment settings, the verified GUI linker flags and CGO disabled for the daemon. The shortened [README](../README.md) documents this entry point. Actual invocation from outside the project directory built both executables successfully; a missing-Go check returned exit 1 without a success message. The script starts no monitor/service and restores the caller's directory/environment.

Later dual-mode delivery: the desktop now tries the daemon first, otherwise hosts the shared monitor engine with exclusive USB ownership. No-argument direct startup uses documented USB-only defaults and per-user saved policy; `--config PATH` selects explicit direct settings. Quit stops only desktop-owned monitoring, never a daemon. The earlier UI-only launch description applies only to daemon attachment or `--smoke-test`. Full tests/vet, repeated affected regressions, both native builds and offline GUI/helper startup checks passed; live GUI USB/handover remains untested. See [F-018](F-018-native-desktop.md) for current commands and limitations.
