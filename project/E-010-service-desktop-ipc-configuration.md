# E-010 - Running service has no desktop IPC configuration

Updated: 2026-11-16 | Priority: P1 | Status: Cause confirmed; installation defaults tested; live repair/retest pending

## Features, impact and classification

Linked features: [F-006 service](F-006-windows-service.md#standard-user-desktop-access---2026-11-16), [F-008 configuration](F-008-configuration.md), [F-018 desktop](F-018-native-desktop.md). Acceptance references: SVC-01, SVC-06, DESK-04, DESK-08/09.

Người dùng báo đã cài/chạy daemon nhưng desktop vẫn báo unavailable, và làm rõ desktop phải chạy bằng tài khoản thường, không cần Administrator. Nguyên nhân xác nhận là cấu hình service không bật IPC, không phải bằng chứng desktop cần nâng quyền hoặc pipe implementation bị lỗi.

Classification: configuration/default-setup observation. Earlier design made IPC optional, including during installation; the clarified primary-desktop journey now requires automatic installation setup. Do not describe the earlier intentional optional setting as a confirmed transport defect.

## Environment and reproduction

Windows amd64, 2026-11-16. Read-only inspection from a non-elevated terminal:

1. User installed/started service `UPSmart` from a minimal monitoring configuration.
2. SCM reported Running as LocalSystem, PID 23880, command `"C:\Program Files\UPSmart\upsdaemon.exe" --config "C:\Program Files\UPSmart\config.yaml"`.
3. Installed configuration had `monitoring-enabled: true`, but neither `desktop-settings-file` nor `desktop-user-sid`.
4. No UPSmart named pipe was present. A desktop process used the same Windows account as the inspecting terminal; no actual user SID is recorded here.
5. Actual: desktop reports daemon unavailable; direct USB fallback cannot take ownership from the daemon. Expected primary-service setup: installer supplies IPC authorization and the desktop connects without UAC.

SCM Running does not verify UPS initialization/freshness or successful GUI IPC. The independent initialization issue [E-008](E-008-q1-startup-control-response.md) is unchanged.

## Confirmed cause and fix

`monitor.Run` only creates the pipe when `Config.Desktop` is present. `serviceConfiguration` previously left that field nil if the source omitted desktop settings, so a valid service installation had no desktop endpoint. Desktop elevation cannot create the missing endpoint.

- Installation now supplies the fixed service policy path and current installing Windows account SID; explicit target SID is preserved. Authorization remains SYSTEM/backend plus one human account, never Everyone/Users.
- Desktop code still has no elevation/deployment behavior. A different-account installation must explicitly authorize the intended desktop user rather than assuming the Administrator account is the target.
- Automatic setup refuses an existing saved service power policy until explicitly reviewed/selected; adding IPC cannot silently reactivate old controls. Existing power validation remains intact.
- Existing services need a one-time elevated config change/restart or corrected reinstall. Launch/relaunch the desktop normally afterwards; running the desktop elevated is not the repair.

Entry points: `internal/service/install_windows.go` (`serviceConfiguration`), `manage_windows.go` (install output), existing `internal/control/pipe_windows.go` and monitor connection selection.

## Actual checks and operational limits

- Five consecutive focused runs of service/control/config/monitor/daemon tests passed in the non-elevated session. Service/control/daemon `go vet` passed.
- Tests cover generated installed account/path with power disabled, explicit-account preservation, blocking unreviewed saved LIVE policy, applying IPC defaults before validating explicit manual policy, preserving invalid-source rejection before history normalization, and real temporary pipe requests for implicit/explicit account configuration with fake state/controller. These tests do not represent a LocalSystem-to-desktop live connection.
- Rebuilt `bin\upsdaemon.exe` with the established CGO-disabled/internal-linker flags. Portable offline validation passed for the example, root/bin local files and preserved installed YAML; all report PC shutdown disabled. No desktop production code changed and its existing process/binary was not replaced.
- Affected documentation checks passed: 93 resolving local file/anchor targets and clean whitespace.
- No active configuration, ACL, user process, service or hardware state was changed by the agent.
- During investigation the service registration, daemon process and installed executable disappeared; read-only rechecks found only preserved `config.yaml`, still without desktop settings or a pipe. The agent did not remove/stop them. A planned installed-binary offline validation could not run because that executable no longer existed; later validation used the rebuilt portable daemon against the preserved file instead.

Assessment: configuration cause and corrected future installation defaults are verified by inspection/tests. Runtime reconnection remains open because no service/pipe is currently available to retest and no elevated repair was approved. User choice confirmed: normal desktop use must not require Administrator rights; earlier UAC question concerned daemon-side maintenance only, not desktop execution.
