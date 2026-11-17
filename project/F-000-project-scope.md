# F-000 - Scope and delivery

Updated: 2026-11-16 | Priority: P0 | Status: Desktop and daemon modes retained; web support removed

## Purpose and user journey

Mục tiêu: thay UPSmart cũ bằng ứng dụng Windows ổn định với giao diện desktop native; có thể tự giám sát UPS hoặc kết nối tới daemon để vận hành nền.

The user launches the desktop standalone or alongside a daemon. The first release is monitoring-focused; optional shutdown policy remains separately gated.

## Scope and decisions

- Original request: support the ARES AR2000 over USB on Windows with background operation and web support. The user later requested web removal; see the current decision below.
- Confirmed on 2026-11-12: Go. Implemented: Windows amd64 and embedded browser assets; no external database or frontend build system. The first milestone used only the standard library; the service candidate adds pinned `golang.org/x/sys/windows` APIs.
- Confirmed on 2026-11-12: extend the plan with CPU, GPU and hard disk monitoring. The originally requested PC temperature monitoring was later explicitly removed by the user from the app and plan. Current optional PC telemetry is tracked in [F-016](F-016-pc-hardware-monitoring.md); UPS temperature is unaffected.
- Confirmed on 2026-11-12: add Telegram messages during monitoring to the plan. Proposed alerts, optional summaries, protected credentials and delivery limitations are tracked in [F-017](F-017-telegram-notifications.md); no messages have been sent.
- Historical decision on 2026-11-12: web listeners were restricted to localhost. Superseded on 2026-11-16 when the user requested removal of web support.
- Confirmed on 2026-11-16: user-edited app configuration uses YAML. Desktop-saved policy state, IPC and event records keep their existing formats. Migration decisions and actual verification are in [F-008](F-008-configuration.md#yaml-configuration---2026-11-16).
- Confirmed on 2026-11-16: Windows service installation must validate/copy configuration beside the installed binary and register the service to use that copy; writable logs remain separate. Delivery evidence and elevated-install limits are in [F-006](F-006-windows-service.md#configuration-beside-the-installed-daemon---2026-11-16).
- Subsequently confirmed on 2026-11-16: adapt UPSmart to rdpproxy's CLI/config/service approach, replacing nonmatching code. User explicitly selected flat hyphenated YAML, defaults/file/CLI precedence, local format conversion and LocalSystem rather than LocalService. Current delivery/limits are authoritative in [F-006](F-006-windows-service.md#current-service-model-rdpproxy-alignment---2026-11-16) and [F-008](F-008-configuration.md#current-rdpproxy-style-options---2026-11-16); this did not authorize executing service or hardware actions.
- Confirmed on 2026-11-12: desktop supports daemon-first startup or desktop-owned direct USB when no monitor owns the device. Shared-engine selection/lifecycle checks and offline native startup passed; final GUI live USB/handover remains pending in [F-018](F-018-native-desktop.md). This does not authorize deployment or live power testing.
- Reaffirmed on 2026-11-16: a normal-user desktop must connect to an authorized running daemon without Administrator rights. Any one-time service setup/configuration elevation belongs to the backend installer, never the GUI. Automatic installation authorization and the observed missing-IPC setup are tracked in [F-006](F-006-windows-service.md#standard-user-desktop-access---2026-11-16) and [E-010](E-010-service-desktop-ipc-configuration.md).
- No shutdown threshold, persistent retention period or LAN exposure has been approved.
- User explicitly confirmed UPSmart was closed and approved read-only live status polling. No approval was requested or granted for changing mains, installing a service or issuing power-control commands.
- Excluded initially: scheduling, email/SMS, remote control, battery self-tests, beeper control and multiple UPS devices.
- Do not stop or uninstall UPSmart, replace drivers, cut mains or issue power commands without explicit approval for that action.
- Never copy proprietary UPSmart implementation code. Public protocol references may inform an independent implementation; assess licenses before reusing third-party code or dependencies.

## Evidence and uncertainty

Read-only inspection earlier in this session found a connected HID device with VID `0001`, PID `0000`, product description `MEC0003`. UPSmart imports Windows HID functions. The public [NUT driver](https://github.com/networkupstools/nut/blob/master/drivers/nutdrv_qx.c) contains this identity with several transport variants.

Initial identification alone was not proof of protocol compatibility. Subsequent read-only tests confirmed Q1 status through HID indexed string 3. Telemetry accuracy against UPSmart, temperature sensor location, shutdown behavior and battery-capacity extensions remain unverified.

## Delivery gates and acceptance

- SCOPE-01: monitoring can operate independently of UPSmart after the user deliberately hands over device access.
- SCOPE-02: no power-changing command is reachable in monitoring-only mode.
- SCOPE-03: implementation and simulated tests precede approved live power tests.
- Gate A: identify transport and obtain repeatable read-only responses.
- Gate B: verify parser, stale-state behavior, native desktop, reconnect and service lifecycle.
- Gate C: approve policy and validate PC shutdown; UPS output control remains separately gated.
- Gate D: verify UPS delayed-off and restart behavior on a safe test setup, or explicitly leave the feature unavailable.

Architecture, backend contracts and test governance live in [F-013](F-013-architecture.md), [F-014](F-014-backend-api.md) and [F-015](F-015-quality.md). Each feature distinguishes existing entry points from planned ones.

## Tests and actual results

2026-11-12: Go monitoring core, bounded native helper, parser, cached APIs and embedded UI were implemented. Targeted tests, vet and Windows amd64 build passed; actual status polling and browser rendering passed. Detailed evidence belongs to the corresponding features and [F-015](F-015-quality.md).

Gate A passed for read-only Q1. Gate B is partial: physical USB reconnect, service lifecycle and long-duration stability remain pending. Gates C/D remain closed; the binary contains no OS/UPS shutdown implementation.

## Progress, blockers and next steps

2026-11-12: F-000 through F-015 specifications and PLAN/TEST indexes created before code. Documentation checks are recorded in [F-015](F-015-quality.md).

Next: compare stable readings with UPSmart in separate sessions, validate physical reconnect, then implement persistent history and Windows service support. No automatic PC protection is provided yet. Temporary test monitoring is stopped after validation; the user may restart UPSmart when not testing the replacement.

### Persistence/service continuation - 2026-11-12

- Implemented rotated JSONL history, restart recovery and Windows SCM lifecycle/deployment commands. Built `bin/upsdaemon.exe` without replacing the original executable.
- The project entry point moved concurrently from `cmd/upsmart/main.go` to root `main.go`; the user explicitly approved adapting to that layout. Preserve the accompanying Air configuration; its setup is outside this milestone.
- The user selected **build only** and declined restarting/replacing the monitor or changing local configuration. No process was stopped/restarted, event logging enabled, Windows service registered, or Administrator elevation attempted by this continuation.
- The current token is not Administrator. Service handler tests use fake applications; actual SCM registration, protected-file creation and LocalService HID access remain unverified.
- Next: obtain separate deployment approval, test history across a controlled real-process restart, then perform an explicitly approved elevated service install and lifecycle check. Shutdown controls remain absent.

### Hardware/Telegram continuation - 2026-11-12

- When asked which workstream to implement next, the user chose all: optional PC hardware telemetry, Telegram event notifications and continued service/persistence validation.
- Added opt-in configuration, independent workers, cached hardware API/dashboard and a live-only Telegram event sink to the shared console/SCM application. Existing configurations remain unchanged.
- Full tests/vet/build and five repeated history/service test runs passed. PC-only read checks verified CPU/GPU utilization, GPU memory and disk/volume inventory/space; sensor/provider limitations are documented in [F-016](F-016-pc-hardware-monitoring.md).
- No live Telegram traffic, active configuration edits, monitor replacement/restart, elevated service installation or power-changing operation was performed. Real Telegram setup, periodic-summary cadence and LocalService integration remain pending. The temperature-provider work considered at this milestone was subsequently removed from scope by the user.

### Desktop-only interface decision - 2026-11-16

- The user requested: "Revemo the web support. I only need the desktop app. Desktop app will run as stand alone or together with the daemon, depend on the setup."
- Removed the TCP dashboard/API and browser assets. The desktop now selects a configured daemon through account-restricted named-pipe IPC, or starts the shared monitor directly when no other process owns the UPS. No web fallback or automatic ownership transfer remains.
- `listen` and `web_enabled` are no longer supported configuration fields; existing JSON must remove them. A daemon requires its `desktop` configuration group to expose the named pipe. Windows service installation remains optional.
- Existing live browser/API evidence is historical and does not describe the current product. Verification of this source/configuration change is recorded in [F-018](F-018-native-desktop.md) and [F-015](F-015-quality.md).
