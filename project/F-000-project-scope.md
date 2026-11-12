# F-000 - Scope and delivery

Updated: 2026-11-12 | Priority: P0 | Status: Monitoring delivered; persistence/service candidate built only

## Purpose and user journey

Mục tiêu: thay UPSmart cũ bằng ứng dụng nền ổn định, giúp người dùng xem trạng thái UPS qua trình duyệt và chủ động cấu hình tắt máy an toàn.

The user installs the replacement, verifies readings against UPSmart, then optionally enables shutdown policy. The first release is monitoring-only.

## Scope and decisions

- Confirmed request: support the ARES AR2000 connected over USB on Windows, with background operation and a web server in Go or Rust.
- Confirmed on 2026-11-12: Go. Implemented: Windows amd64 and embedded browser assets; no external database or frontend build system. The first milestone used only the standard library; the service candidate adds pinned `golang.org/x/sys/windows` APIs.
- Confirmed on 2026-11-12: extend the plan with CPU, GPU and hard disk monitoring. The originally requested PC temperature monitoring was later explicitly removed by the user from the app and plan. Current optional PC telemetry is tracked in [F-016](F-016-pc-hardware-monitoring.md); UPS temperature is unaffected.
- Confirmed on 2026-11-12: add Telegram messages during monitoring to the plan. Proposed alerts, optional summaries, protected credentials and delivery limitations are tracked in [F-017](F-017-telegram-notifications.md); no messages have been sent.
- Confirmed on 2026-11-12: listen on localhost only, including future dashboard/API additions. Existing implementation restricts binding to `127.0.0.1`; Telegram is outbound only, with no public webhook.
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
- Gate B: verify parser, stale-state behavior, dashboard, reconnect and service lifecycle.
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

- Implemented rotated JSONL history, restart recovery and Windows SCM lifecycle/deployment commands. Built `bin/upsmart-next.exe` without replacing the original executable.
- The project entry point moved concurrently from `cmd/upsmart/main.go` to root `main.go`; the user explicitly approved adapting to that layout. Preserve the accompanying Air configuration; its setup is outside this milestone.
- The user selected **build only** and declined restarting/replacing the monitor or changing local configuration. No process was stopped/restarted, event logging enabled, Windows service registered, or Administrator elevation attempted by this continuation.
- The current token is not Administrator. Service handler tests use fake applications; actual SCM registration, protected-file creation and LocalService HID access remain unverified.
- Next: obtain separate deployment approval, test history across a controlled real-process restart, then perform an explicitly approved elevated service install and lifecycle check. Shutdown controls remain absent.

### Hardware/Telegram continuation - 2026-11-12

- When asked which workstream to implement next, the user chose all: optional PC hardware telemetry, Telegram event notifications and continued service/persistence validation.
- Added opt-in configuration, independent workers, cached hardware API/dashboard and a live-only Telegram event sink to the shared console/SCM application. Existing configurations remain unchanged.
- Full tests/vet/build and five repeated history/service test runs passed. PC-only read checks verified CPU/GPU utilization, GPU memory and disk/volume inventory/space; sensor/provider limitations are documented in [F-016](F-016-pc-hardware-monitoring.md).
- No live Telegram traffic, active configuration edits, monitor replacement/restart, elevated service installation or power-changing operation was performed. Real Telegram setup, periodic-summary cadence and LocalService integration remain pending. The temperature-provider work considered at this milestone was subsequently removed from scope by the user.
