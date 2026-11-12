# F-009 - Graceful PC shutdown

Updated: 2026-11-12 | Priority: P1 | Status: Opt-in candidate mock-tested; disabled locally; no real shutdown

## Purpose and user journey

Mục tiêu: cho Windows đủ thời gian đóng ứng dụng khi mất điện kéo dài, nhưng tránh tắt máy do mất USB hoặc dữ liệu sai.

The authorized Windows desktop user opts into an explicit policy. During a verified outage the UI shows why shutdown is pending; restored mains cancels the pending action before Windows shutdown is requested.

## Current desktop control boundary

The later [F-018](F-018-native-desktop.md) decision supersedes Telegram manual power commands below. Manual shutdown/cancel and policy editing now use the account-restricted local desktop connection with no runtime UAC prompt. Web and Telegram are read-only; old manual Telegram sender IDs do not authorize any bot power operation.

Desktop settings require explicit desktop IPC/file configuration and persist before activation. Editing is rejected while a countdown is pending or an attempt is lifetime-latched. Accepted changes restart automatic qualification; saved policy is validated equally at startup and by offline CLI validation. The native confirmation binds the exact configured mode/countdown, rejecting changes that occur before submission.

## Implementation

- Entry points: `internal/policy`, `internal/platform/shutdown_windows.go`, `internal/notifications/commands.go` and root `main.go`.
- Automatic evaluation uses only fresh verified on-battery samples. Independently selected triggers: observed outage duration and the verified device low-battery flag. Capacity-based shutdown is not implemented.
- Do not use estimated capacity, stale readings, communication failures or guessed voltage thresholds.
- Separate trigger qualification from a configured cancellation countdown. Low battery never bypasses the countdown. No active timing values are chosen implicitly.
- States: disabled, armed, pending, requesting, requested, failed, blocked. Record trigger, deadline, cancellation and OS error.
- A single serialized controller owns transitions. Immediately before requesting shutdown, recheck latest sample freshness, on-battery state, policy version and cancellation state.
- Verified mains restoration cancels pending shutdown. Communication loss cancels pending evaluation and blocks automation until fresh evidence returns; qualification restarts as described in [F-003](F-003-power-status.md).
- Use an injectable OS shutdown adapter. Production invokes the fixed Windows System32 `shutdown.exe /s /t 0` with a five-second process deadline. The application owns the countdown: a nonzero Windows `/t` would implicitly force applications closed, so neither that nor `/f` is used. This bounded Windows utility replaces the originally proposed in-process API/privilege manipulation. Tests use recording fakes; no privilege is granted automatically.
- One OS request per observed outage/controller lifetime. Failed requests enter a visible failed state without rapid retries; restart must requalify from new samples and the full configured delay.
- Once Windows accepts the request, cancellation is not promised. Explain this boundary in UI and logs.
- PC shutdown does not require or imply UPS output control. Closing the dashboard has no effect on policy execution.

## Acceptance and tests

- OFF-01: fake-clock tests cover threshold crossing, countdown, restoration, communication loss and stale data.
- OFF-02: restoration racing with deadline cannot dispatch a request after the controller has accepted cancellation.
- OFF-03: repeated samples produce at most one OS request; rejection is visible and not retried in a loop.
- OFF-04: disabled policy and estimated/invalid capacity never request shutdown.
- OFF-05: dry-run mode records "would request" without invoking the real OS adapter.
- OFF-06: after explicit approval and saving work, an actual Windows shutdown is observed independently of the initiating process.

## Decisions and safety boundaries

The user selected configurable sustained-mains-loss/low-battery automation and confirmed manual shutdown over Telegram, disabled by default, with mains-restoration cancellation. Timing values remain local configuration choices. Non-forced shutdown may be delayed or blocked by applications; do not claim guaranteed completion. Live tests require explicit approval and no UPS cut-off commands.

## Results and next steps

Earlier planning baseline, 2026-11-12: no shutdown adapter, policy simulation or live shutdown had run. Superseded by the candidate below; real shutdown remains unverified.

### Earlier Telegram-control candidate - 2026-11-12 (manual transport superseded by F-018)

Mặc định không tắt máy. Chỉ khi cấu hình rõ `dry_run` hoặc `live` và bật từng chức năng thì controller mới hoạt động. Chế độ thử chỉ ghi nhận hành động dự kiến; không gọi Windows.

- Optional `power_control` schema and a documentation-only dry-run example are in [F-008](F-008-configuration.md). Existing local/example configurations are unchanged. Automatic controls require USB monitoring; manual controls require the configured Telegram chat and explicit sender allowlist.
- Outage qualification and deadlines use controller-owned monotonic time. Disconnect, stale evidence, changed outage identity and restoration cancel/requalify automatic countdowns. The store also resets outage identity when a successful read follows a stale gap, even if a policy tick missed that gap.
- Low-battery clearing cancels that trigger; if outage duration independently qualifies, a fresh full outage countdown starts. No voltage/capacity estimate or UPS command is used.
- Manual `/shutdown` requires a short-lived, sender-bound `/confirm CODE`; only confirmation starts its configured countdown. Manual shutdown can work while mains is already online or USB unavailable. It is cancelled if the controller observes battery operation followed by verified mains restoration during the countdown, not merely because AC was online initially.
- `/cancel` clears all outstanding bot confirmations and cancels a pending policy. Automatic rearming remains suppressed until verified mains returns. A new explicitly confirmed manual request may start a full countdown in the same outage.
- The controller serializes cancellation and dispatch, rechecks latest automatic evidence immediately before dispatch, and publishes immutable state/events without performing OS/network I/O under the snapshot lock.
- `requested`, `failed` and `dry_run` are latched until process restart, even after mains returns. There is no automatic retry after denial/ambiguous acceptance; a new process must requalify. `requested` means the utility accepted the request, not that Windows completed shutdown.
- The utility runs under the existing account; missing shutdown privilege produces visible failure. No UAC prompt, account-right change, LocalSystem elevation, service installation or active configuration edit is performed.

Fake-clock/controller tests cover OFF-01 through OFF-05, including exact threshold/countdown boundaries, restoration/cancellation ordering, stale/reconnect qualification, low-battery clearing, latest-input recheck, manual online behavior, cancellation suppression, failures and single dispatch. Adapter tests intercept the process runner, verify non-forced zero-delay invocation/cancellation and expose rejection. Telemetry -> real controller -> cached HTTP API integration uses a fake OS callback; Telegram -> confirmation -> real dry-run controller is also tested.

Targeted tests passed five repeated runs; full Go tests/vet and dashboard syntax/DOM tests passed. No real shutdown utility, bot connection, mains-loss test or UPS command was run. OFF-06, LocalService privilege verification and real delivery remain explicitly pending. Race execution is still blocked by [E-003](E-003-race-runtime-startup.md); simulation success is not a race-free/production-safety claim.

Next: review local settings and run only an explicitly approved dry-run deployment first. Real shutdown requires separate approval after saving work.
