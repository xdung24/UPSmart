# F-006 - Windows service

Updated: 2026-11-12 | Priority: P1 | Status: Candidate implemented; actual SCM installation unverified

## Purpose and user journey

Mục tiêu: giám sát UPS từ lúc khởi động Windows, không cần đăng nhập hoặc giữ cửa sổ chương trình mở.

An administrator installs the service once. Normal monitoring starts automatically; the dashboard explains any missing-device or permission error.

## Implementation

- Current entry points: root `main.go` and `internal/service/{handler,manage,install}_windows.go`.
- Provide console mode for diagnosis and explicit `service install`, `service start`, `service stop`, `service uninstall` operations.
- Use Windows Service Control Manager APIs through a maintained Windows Go package if Go is selected.
- Service and console modes share one application lifecycle; no tray application or interactive service desktop dependency.
- Enforce one active poller across service and console mode using a restricted named mutex. Report an already-running instance clearly.
- Use absolute executable/configuration paths; quote the registered service executable path and never rely on the current working directory.
- Select the least-privileged service identity after verifying HID access. Add shutdown privilege only when the enabled policy requires it; do not assume LocalSystem is necessary.
- Read configuration before starting workers. Invalid configuration or HTTP bind failure is an explicit startup failure; missing hardware may run in degraded mode with retries.
- Proposed orderly stop budget: 10 seconds, with SCM progress reporting. Stop polling, cancel I/O, close HTTP and flush bounded event output. Stopping/uninstalling must never issue PC or UPS shutdown.
- Configure bounded SCM restart recovery for process crashes, not normal stops; no broad panic recovery that presents a healthy service.

## Acceptance and tests

- SVC-01: service starts without user login and serves loopback status.
- SVC-02: console/service duplication is rejected without disturbing the owner.
- SVC-03: install/uninstall reports permission failures and preserves user configuration/history unless explicitly requested otherwise.
- SVC-04: stop during blocked device access completes within the verified stop budget.
- SVC-05: controlled crash recovery does not enable automation or repeat a completed shutdown action.

## Decisions and boundaries

Depends on [F-005](F-005-connection-resilience.md), [F-008](F-008-configuration.md) and [F-011](F-011-web-access-safety.md). Exact account and Windows version support need verification. Installation and service registration modify Windows and require explicit user approval.

## Results and next steps

First milestone: console monitoring and the named instance guard were live verified. The current shared application lifecycle is in root `main.go`; the user approved adapting to that moved entry point.

Persistence/service candidate adds `internal/service/handler_windows.go`, `manage_windows.go` and `install_windows.go`, using pinned `golang.org/x/sys v0.50.0`.

- `service install --config PATH`: requires an elevated Administrator token and no active monitor owner. Validates/copies configuration, deploys to fixed protected paths, and registers service `UPSmartMonitor` without starting it.
- Service identity: `NT AUTHORITY\LocalService`, not LocalSystem. Actual USB permission under this identity is still unverified.
- Fixed paths: `%ProgramFiles%\UPSmartMonitor\upsmart-monitor.exe`, `%ProgramData%\UPSmartMonitor\config.json`, and the latter directory's `logs` child. These isolate deployment from the legacy ARES application.
- Executable/configuration directories and files use Administrator ownership and protected ACLs: SYSTEM/Administrators full access; LocalService and local Users read/execute only. Only the logs directory grants LocalService full access. Unexpected existing ownership/ACLs or reparse points are rejected, not changed implicitly.
- Registration uses a quoted absolute command, delayed automatic startup, crash restart after 5 seconds and 30 seconds, then no further action until the failure count resets after one day. Normal/non-crash service failures do not trigger recovery loops.
- Handler publishes StartPending/Running/StopPending, accepts stop/shutdown, reports progress, and enforces 10-second startup/stop budgets. Application cleanup shares an 8-second HTTP/journal budget. No PC or UPS power command exists.
- `service start/stop/status/uninstall` validates the registered command before managing it. Stop/start waits are bounded to 15 seconds. Uninstall requires an already-stopped service and preserves binary/configuration/history.
- Startup/configuration and runtime diagnostics go to Windows Application Event Log. Configuration or bind failures never report Running.
- Existing configuration is preserved: installation rejects differing settings rather than overwriting them. A failed install reports errors and may leave protected deployment files for inspection; it does not recursively delete data.

Future approved installation, from an elevated terminal with console monitoring deliberately stopped:

```powershell
.\bin\upsmart-next.exe service install --config .\config.local.json
& "$env:ProgramFiles\UPSmartMonitor\upsmart-monitor.exe" service start
& "$env:ProgramFiles\UPSmartMonitor\upsmart-monitor.exe" service status
& "$env:ProgramFiles\UPSmartMonitor\upsmart-monitor.exe" service stop
& "$env:ProgramFiles\UPSmartMonitor\upsmart-monitor.exe" service uninstall
```

These are instructions, not executed changes. Source configuration keeps its monitoring flag; service history uses the protected logs directory, preserving requested retention limits or choosing 5 MiB x 5 when omitted.

2026-11-12 tests passed for fake-application start/stop/interrogate, startup failure, explicit startup/stop timeout, console rejection of SCM-only mode, and rejection of an unprotected existing deployment executable without overwriting it. Event/service packages passed five consecutive test runs.

Assessment: implementation and handler-level verification only. The session has no Administrator token, and the user selected build-only. No service/registry/ACL deployment or LocalService USB test was performed. Next: separately approved elevated install, controlled start/stop/uninstall, account access, crash recovery and real console Ctrl+C validation.

### Optional worker integration - 2026-11-12

The shared console/SCM application now starts optional hardware sampling and Telegram delivery workers with coordinated cancellation. Both groups are validated before acquiring hardware ownership or opening the listener. Invalid/missing enabled Telegram credentials fail startup explicitly; disabled optional groups preserve previous behavior.

Existing handler/deployment tests and event/service regressions passed, including five repeated runs. This is not an elevated SCM or LocalService integration test. Telegram environment provisioning, PC metric-provider permissions and actual combined service shutdown timing remain unverified; no service registration or active configuration was changed.

### Opt-in PC policy candidate - 2026-11-12

Shared startup now validates/initializes the optional policy and authorized Telegram command worker before starting their loops. Existing monitoring-only configurations remain unchanged. Console/SCM cancellation stops pending evaluation/polling; stopping the application is not a shutdown trigger and cannot retract a Windows request already accepted.

Live mode uses the existing account's fixed System32 shutdown utility with no forced application termination. The installer does not grant shutdown rights, change account identity, provision bot secrets or automatically enable controls. LocalService shutdown privilege, utility behavior and actual bot access are unverified and must be checked in a separately approved deployment. Unit tests intercept the OS adapter; no service or Windows shutdown was performed.

### Desktop service-client configuration - 2026-11-12

The optional Fyne client uses account-restricted local IPC, without ordinary-operation UAC prompts. Installation still requires Administrator rights as enforced by Windows. When desktop IPC is selected, installation requires an explicit human user SID and exact protected `%ProgramData%\UPSmartMonitor\logs\desktop-policy.json` settings path. The existing logs ACL grants LocalService write access, not desktop-user direct file edits. No account right or global permission is automatically granted.

Web may be disabled independently; closing/quitting the desktop does not stop service workers. Telegram is now checking-only. Existing handler/install tests pass; actual service IPC ACLs, shutdown rights and combined native client operation under LocalService remain unverified. No service/configuration deployment occurred.
