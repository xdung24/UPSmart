# F-006 - Windows service

Updated: 2026-11-16 | Priority: P1 | Status: Automatic desktop authorization tested; existing-installation desktop retest pending

## Standard-user desktop access - 2026-11-16

Người dùng xác nhận desktop không cần quyền Administrator khi daemon đã chạy. Cài service là bước quản trị một lần; chạy/xem dữ liệu/điều khiển qua IPC dùng tài khoản Windows thường đã được cho phép. Không nâng quyền desktop để sửa kết nối.

- New installations always configure desktop IPC. When absent, `serviceConfiguration` supplies the fixed service policy path and installing token's human user SID. An explicitly configured target SID is preserved; omitted SID on an explicit settings path also resolves to the installing account. A different-account/UAC-credentials installation must explicitly select the intended desktop account.
- The pipe remains restricted to SYSTEM/backend and that one user, not Everyone/Users. No power options are enabled by this default. To avoid activating an old saved power policy merely by adding IPC, automatic setup rejects an existing saved policy until its path/state is explicitly reviewed and selected.
- This changes the earlier optional-IPC install default below. Console-only configuration remains optional; the GUI never elevates or changes installed service configuration.
- Regression checks cover default authorization/path and disabled power options in installed flat YAML, preservation of an explicit SID, rejection of unreviewed existing saved policy, and real temporary pipe requests with both implicit/explicit current-user authorization in a non-elevated test session.
- Actual 2026-11-16 validation: five repeated service/control/config/monitor/daemon test runs, corresponding service/control/daemon vet, daemon rebuild and portable offline validation of example/preserved installed YAML passed. Defaults are applied before service semantic validation; invalid source settings are still rejected before normalization. The desktop executable/process was not elevated, modified or restarted.
- Read-only inspection verified a real LocalSystem service registration in Running state, but no desktop configuration or pipe. The service and installed executable subsequently disappeared during investigation; the agent did not stop/remove them. The original observation and current repair/retest limits are in [E-010](E-010-service-desktop-ipc-configuration.md). Runtime USB access and actual desktop-to-service connection are not inferred from SCM Running.

## Current service model: rdpproxy alignment - 2026-11-16

Người dùng chọn sửa UPSmart theo rdpproxy trong checkout `D:\project\rdp-ex`, được phép bỏ cách cũ không phù hợp. Sau khi đối chiếu, người dùng xác nhận YAML phẳng/defaults/CLI override và **LocalSystem**, thay cho LocalService. Thay đổi này chỉ triển khai trong code; chưa được dùng để cài hoặc chạy service thật.

### User journey and selected contract

1. Edit YAML or supply equivalent flags, then run `bin\upsdaemon.exe --validate -c config.yaml`.
2. From an elevated terminal with monitoring deliberately stopped, run `bin\upsdaemon.exe --install -c config.yaml`.
3. Use separate `--start`, `--status`, `--stop`, `--remove` commands. Installation does not start monitoring; stopping/removal never issues a PC/UPS power command.
4. Service name remains `UPSmart`; binary becomes `%ProgramFiles%\UPSmart\upsdaemon.exe`. Configuration retains the chosen basename in that directory. Logs/policy remain under `%ProgramData%\UPSmart\logs`.
5. Registration contains only `--config ABSOLUTE_INSTALLED_PATH`, never installation/control flags. `kardianos/service` supplies automatic Windows startup and LocalSystem by its default account. The old custom delayed-start/recovery schedule is not carried over.

### Developer contracts and boundaries

- Entry points: daemon `config.Parse`/main dispatch, `internal/service/manage_windows.go` (`definition`, `Manage`), `install_windows.go` (`serviceConfiguration`, `deployFiles`, `writeAtomic`) and `handler_windows.go` (`program.Start/Stop`, `Run`).
- Uses `github.com/kardianos/service v1.3.0`, matching the reference. Service/console detection no longer requires a `service run` subcommand. Old `service ...` CLI commands return migration guidance.
- Existing service registrations are rejected before deployment writes; stop/remove deliberately before reinstalling. The existing named ownership guard remains enforced during installation and monitoring.
- Deliberate reliability differences from the reference: serialize the validated **effective** flat YAML so CLI overrides survive service startup; atomically replace individual files, reject reparse targets and executable/config filename collision; reject ambiguous missing-source/existing-installed-config installs with instructions to select that copy explicitly. This avoids silently discarding overrides or resetting installed settings.
- Source comments/layout are not preserved. A valid explicitly selected source can replace existing destination settings when creating a new registration. Missing source with no installed copy generates validated safe defaults/CLI settings. A failure can leave completed deployment files; the binary/config pair is not a filesystem-wide transaction.
- Exact SDDL matching and automatic protected-owner creation from the earlier installer are retired. Files/directories inherit or retain Windows permissions; no ownership/ACL reset is performed. Keep Program Files deployment administrator-managed; configurations contain secret environment-variable names, never bot tokens.
- Deployment prerequisite: an administrator must inspect existing Program Files/ProgramData directories and prevent untrusted writes, particularly to saved power policy. This reference-style installer does not audit arbitrary existing ACLs; CFG-04 and actual service-policy permissions are not claimed verified by temporary-file tests.
- LocalSystem is substantially more privileged than the prior account. This is the explicit user choice, not a claim that UPS access requires it. No privilege is enabled merely by parsing options. Notification credentials still need separate service-context provisioning; actual account/hardware behavior remains unverified.
- Desktop IPC requires a concrete human SID and the fixed service policy path; installation now supplies the installing user's SID/path by default as described above. SID-based pipe authorization, opt-in power mode/countdown validation and exclusive UPS ownership are unchanged.
- Windows ignores the library's `WorkingDirectory` field, so the SCM runner explicitly selects the installed config directory before starting the application. Runtime data paths remain absolute.
- Startup waits for monitor readiness, with a 10-second budget; stop cancels and waits up to 10 seconds. Startup/stop failures are returned; an unexpected post-ready monitor exit is logged and terminates the service process nonzero rather than leaving an empty service reported healthy. Startup errors are also reported to Windows Event Log.
- `--remove` requires a stopped service, removes its registration/binary and preserves config/history. Run from the portable binary, since Windows can refuse deleting the currently executing installed binary. Existing legacy `upsmart-monitor.exe` files are not proactively deleted.
- Status/removal use a read-only native SCM query so pending/paused states are not collapsed into "stopped" by the library; removal requires the actual Stopped state. `--status` reports the concrete state and PID.

### Actual verification and next steps

2026-11-16: full `go test .\...` and `go vet .\...` passed. `build.cmd` rebuilt both Windows apps. Built daemon help and offline `--validate -c` passed for the example and both migrated local files; all report PC shutdown disabled. Five repeated service/config/daemon/monitor runs passed. Focused tests cover flat configuration precedence/serialization, copied/replaced executable/effective config under test-owned temporary paths, no writes for invalid settings, config/binary collision, atomic failure preservation, reparse-target rejection (actual temporary symlink, not skipped), service definition/account/argument shape, readiness, cancellation, bounded startup/stop and fatal runtime reporting. Local documentation targets/anchors and whitespace checks passed.

Assessment: implementation, temporary-filesystem behavior and builds are verified, not real SCM installation. Elevated deployment/start/stop/remove, LocalSystem USB access, service environment provisioning and real power controls remain pending. No service/registry/installed ACL changes, hardware queries or real notifications were executed. Independent UPS initialization [E-008](E-008-q1-startup-control-response.md) is unchanged. Earlier ACL comparison evidence [E-009](E-009-service-acl-normalization.md) is historical: that installer has now been replaced by this explicit user decision.

## Historical implementation records

The remaining sections preserve prior decisions/results. Their LocalService, exact-ACL, executable-name and subcommand contracts are superseded by the current model above.

## Protected ACL comparison fix - 2026-11-16

Lệnh cài do người dùng chạy đã dừng ở bước kiểm tra quyền thư mục, dù quyền thực tế đúng chính sách. Đã sửa cách so sánh, không nới quyền và không tự sửa ACL. Chi tiết lỗi, bằng chứng và kiểm tra hồi quy: [E-009](E-009-service-acl-normalization.md).

- Compare exact owner/group SIDs, DACL presence/protection and the full multiset of allow rules (SID, all ACE flags and file-mapped access mask), rather than serialized SDDL text. Only allow-rule ordering and equivalent generic/file rights are normalized.
- Extra/missing/duplicate grants, changed owner/group, writable Users, changed/inherited ACE flags, unprotected/null DACLs and unsupported ACE types remain rejected. Existing reparse-point and overwrite protections are unchanged.
- Actual 2026-11-16 checks: targeted service/config/daemon tests and vet passed; ACL regression tests and temporary-directory/file creation/replacement passed five consecutive runs. A temporary read-only diagnostic using production `verifySecurity` accepted the existing `%ProgramFiles%\UPSmart` directory five times; the diagnostic source was removed afterwards. The daemon was rebuilt.
- Assessment: the reported directory-check failure is fixed and verified without modifying installed permissions/files. Test-owned temporary objects use the current owner's SID, not an Administrator-owned deployment. Full elevated installation, SCM registration, LocalService startup/USB access and independent UPS initialization remain pending; [E-008](E-008-q1-startup-control-response.md) is unchanged. No service operation or hardware polling was executed.

## Configuration beside the installed daemon - 2026-11-16

Người dùng chọn: khi cài Windows service, kiểm tra cấu hình trước, sao chép cấu hình hợp lệ vào cùng thư mục với binary đã cài, rồi cho service dùng bản sao đó. Service không phụ thuộc thư mục chạy lệnh cài hoặc đường dẫn cấu hình nguồn sau khi cài xong.

- From an elevated terminal, run `bin\upsdaemon.exe service install --config config.yaml` (or choose another source with `--config`). This is an instruction, not a command executed during this change.
- Source loading/validation still precedes deployment writes. Invalid configuration aborts installation. The existing installer serializes the validated effective configuration as YAML, keeping its established normalization of service event history to the protected logs directory; this is not a byte-for-byte/comments-preserving copy.
- Installed executable: `%ProgramFiles%\UPSmart\upsmart-monitor.exe`. Installed configuration: `%ProgramFiles%\UPSmart\config.yaml`, now in the **same directory**. SCM receives the copied configuration's absolute path through `service run --config PATH`; it does not retain the source path.
- Configuration keeps the executable directory's protected Administrator-owned read-only policy: SYSTEM/Administrators full access, LocalService/Users read/execute only. Writable logs and desktop-saved policy remain under `%ProgramData%\UPSmart\logs`; no write access to the binary directory is granted.
- Existing invalid/different destination configuration still fails explicitly and is preserved, rather than silently overwritten. Uninstall still preserves deployment files.
- Management recognizes the new co-located config path and the exact older protected ProgramData `config.yaml`/`config.json` paths, so older service registrations can still be managed deliberately. Other executables/configuration paths and extra arguments remain rejected. Existing registrations/files are not automatically migrated.
- Entry points: `deploymentPaths` and `install` in `internal/service/install_windows.go`, command verification in `internal/service/manage_windows.go`, and daemon installation help.

2026-11-16 actual checks: `go test .\internal\service .\internal\config .\cmd\upsdaemon` and corresponding `go vet` passed. The command-verification test uses real Windows deployment-path resolution without deployment writes, checks YAML beside the installed binary independently of the configured path field, permits both historical paths and rejects unrelated paths/executables/extra arguments. Existing invalid-config and unprotected-overwrite tests still pass. The Windows amd64 daemon rebuilt successfully with the existing CGO-disabled/internal-linker flags; built `--help` describes the copied absolute path.

Assessment: destination selection, command validation and build are verified; the actual protected copy, SCM registration and LocalService startup were **not** executed. No Administrator/service/configuration changes, USB polling, Telegram message or power operation occurred. An approved elevated installation/retest remains pending; independent UPS startup limitation [E-008](E-008-q1-startup-control-response.md) is unchanged.

## Purpose and user journey

Mục tiêu: giám sát UPS từ lúc khởi động Windows, không cần đăng nhập hoặc giữ cửa sổ chương trình mở.

An administrator installs the service once. Normal monitoring starts automatically; the native desktop explains any missing-device or permission error.

## Implementation

- Current entry points: `cmd/upsdaemon/main.go`, shared `internal/monitor/monitor.go` and `internal/service/{handler,manage,install}_windows.go`; earlier root-entry references below are historical.
- Provide console mode for diagnosis and explicit `service install`, `service start`, `service stop`, `service uninstall` operations.
- Use Windows Service Control Manager APIs through a maintained Windows Go package if Go is selected.
- Service and console modes share one application lifecycle; no tray application or interactive service desktop dependency.
- Enforce one active poller across service and console mode using a restricted named mutex. Report an already-running instance clearly.
- Use absolute executable/configuration paths; quote the registered service executable path and never rely on the current working directory.
- Select the least-privileged service identity after verifying HID access. Add shutdown privilege only when the enabled policy requires it; do not assume LocalSystem is necessary.
- Read configuration before starting workers. Invalid configuration or named-pipe setup failure is an explicit startup failure; missing hardware may run in degraded mode with retries.
- Proposed orderly stop budget: 10 seconds, with SCM progress reporting. Stop polling, cancel I/O, close desktop IPC and flush bounded event output. Stopping/uninstalling must never issue PC or UPS shutdown.
- Configure bounded SCM restart recovery for process crashes, not normal stops; no broad panic recovery that presents a healthy service.

## Acceptance and tests

- SVC-01: service starts without user login and supports the configured desktop IPC when enabled.
- SVC-02: console/service duplication is rejected without disturbing the owner.
- SVC-03: install/uninstall reports permission failures and preserves user configuration/history unless explicitly requested otherwise.
- SVC-04: stop during blocked device access completes within the verified stop budget.
- SVC-05: controlled crash recovery does not enable automation or repeat a completed shutdown action.
- SVC-06: valid configuration is deployed beside the installed daemon and SCM uses that copy's absolute path; invalid source settings cannot produce an installation copy.

## Decisions and boundaries

Depends on [F-005](F-005-connection-resilience.md), [F-008](F-008-configuration.md) and [F-018](F-018-native-desktop.md) for optional desktop IPC. Exact account and Windows version support need verification. Installation and service registration modify Windows and require explicit user approval.

## Results and next steps

First milestone: console monitoring and the named instance guard were live verified. The current shared application lifecycle is in root `main.go`; the user approved adapting to that moved entry point.

Persistence/service candidate adds `internal/service/handler_windows.go`, `manage_windows.go` and `install_windows.go`, using pinned `golang.org/x/sys v0.50.0`.

- `service install --config PATH`: requires an elevated Administrator token and no active monitor owner. Validates/copies configuration, deploys to fixed protected paths, and registers service `UPSmart` without starting it.
- Service identity: `NT AUTHORITY\LocalService`, not LocalSystem. Actual USB permission under this identity is still unverified.
- Fixed paths: `%ProgramFiles%\UPSmart\upsmart-monitor.exe` and adjacent `config.yaml`; writable data uses `%ProgramData%\UPSmart\logs`. Configuration placement supersedes the earlier ProgramData location. The installer serializes YAML; see the [configuration migration](F-008-configuration.md#yaml-configuration---2026-11-16). These isolate deployment from the legacy ARES application.
- Executable/configuration directories and files use Administrator ownership and protected ACLs: SYSTEM/Administrators full access; LocalService and local Users read/execute only. Only the logs directory grants LocalService full access. Unexpected existing ownership/ACLs or reparse points are rejected, not changed implicitly.
- Registration uses a quoted absolute command, delayed automatic startup, crash restart after 5 seconds and 30 seconds, then no further action until the failure count resets after one day. Normal/non-crash service failures do not trigger recovery loops.
- Handler publishes StartPending/Running/StopPending, accepts stop/shutdown, reports progress, and enforces 10-second startup/stop budgets. Application cleanup shares an 8-second IPC/journal budget. No PC or UPS power command exists.
- `service start/stop/status/uninstall` validates the registered command before managing it. Stop/start waits are bounded to 15 seconds. Uninstall requires an already-stopped service and preserves binary/configuration/history.
- Startup/configuration and runtime diagnostics go to Windows Application Event Log. Configuration or bind failures never report Running.
- Existing configuration is preserved: installation rejects differing settings rather than overwriting them. A failed install reports errors and may leave protected deployment files for inspection; it does not recursively delete data.

Future approved installation, from an elevated terminal with console monitoring deliberately stopped:

```powershell
.\bin\upsdaemon.exe service install --config .\config.yaml
& "$env:ProgramFiles\UPSmart\upsmart-monitor.exe" service start
& "$env:ProgramFiles\UPSmart\upsmart-monitor.exe" service status
& "$env:ProgramFiles\UPSmart\upsmart-monitor.exe" service stop
& "$env:ProgramFiles\UPSmart\upsmart-monitor.exe" service uninstall
```

These are instructions, not executed changes. Source configuration keeps its monitoring flag; service history uses the protected logs directory, preserving requested retention limits or choosing 5 MiB x 5 when omitted.

2026-11-12 tests passed for fake-application start/stop/interrogate, startup failure, explicit startup/stop timeout, console rejection of SCM-only mode, and rejection of an unprotected existing deployment executable without overwriting it. Event/service packages passed five consecutive test runs.

Assessment: implementation and handler-level verification only. The session has no Administrator token, and the user selected build-only. No service/registry/ACL deployment or LocalService USB test was performed. Next: separately approved elevated install, controlled start/stop/uninstall, account access, crash recovery and real console Ctrl+C validation.

### Optional worker integration - 2026-11-12

The shared console/SCM application now starts optional hardware sampling and Telegram delivery workers with coordinated cancellation. Both groups are validated before acquiring hardware ownership or opening desktop IPC. Invalid/missing enabled Telegram credentials fail startup explicitly; disabled optional groups preserve previous behavior.

Existing handler/deployment tests and event/service regressions passed, including five repeated runs. This is not an elevated SCM or LocalService integration test. Telegram environment provisioning, PC metric-provider permissions and actual combined service shutdown timing remain unverified; no service registration or active configuration was changed.

### Opt-in PC policy candidate - 2026-11-12

Shared startup now validates/initializes the optional policy and authorized Telegram command worker before starting their loops. Existing monitoring-only configurations remain unchanged. Console/SCM cancellation stops pending evaluation/polling; stopping the application is not a shutdown trigger and cannot retract a Windows request already accepted.

Live mode uses the existing account's fixed System32 shutdown utility with no forced application termination. The installer does not grant shutdown rights, change account identity, provision bot secrets or automatically enable controls. LocalService shutdown privilege, utility behavior and actual bot access are unverified and must be checked in a separately approved deployment. Unit tests intercept the OS adapter; no service or Windows shutdown was performed.

### Desktop service-client configuration - 2026-11-12

The optional Fyne client uses account-restricted local IPC, without ordinary-operation UAC prompts. Installation still requires Administrator rights as enforced by Windows. When desktop IPC is selected, installation requires an explicit human user SID and exact protected `%ProgramData%\UPSmart\logs\desktop-policy.json` settings path. The existing logs ACL grants LocalService write access, not desktop-user direct file edits. No account right or global permission is automatically granted.

Web may be disabled independently; closing/quitting the desktop does not stop service workers. Telegram is now checking-only. Existing handler/install tests pass; actual service IPC ACLs, shutdown rights and combined native client operation under LocalService remain unverified. No service/configuration deployment occurred.
