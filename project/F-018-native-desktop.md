# F-018 - Fyne Windows desktop

Updated: 2026-11-22 | Priority: P1 | Status: Power-flow chart simplified; Telegram test action added; live visual confirmation pending

## Current scope - 2026-11-16

The user requested removal of web support and confirmed that only the desktop app is needed. The supported modes are:

1. Connect to a daemon through its account-restricted local named pipe.
2. If no compatible daemon is available, run the shared monitor directly in the desktop, provided exclusive USB ownership is available.

The desktop does not discover or connect to a web API. The daemon has no TCP listener, browser dashboard or HTTP API. HTTP framing inside the named pipe is local IPC, not web support. New service installations automatically configure the installing Windows account's desktop access; a console daemon needs explicit desktop settings. A Windows service remains optional. **Normal desktop use never requires Administrator rights.**

Current configuration/setup observation [E-010](E-010-service-desktop-ipc-configuration.md): the installed service was running but its copied YAML omitted both desktop fields, so no pipe existed. Installer defaults now supply restricted IPC authorization automatically without enabling power controls. Existing installations still need a one-time administrator-side config update/restart or corrected reinstall, then relaunch the desktop normally. This is not permission elevation in the desktop. Non-elevated explicit/implicit pipe regression tests pass; actual desktop/service reconnection remains pending.

## Purpose and user journey

Provide a Windows desktop control center similar in workflow to legacy UPSmart, without sharing USB ownership with it. Fyne supplies the native executable/window and its own rendered widgets, not classic Win32 controls. Use original artwork/layout components, not copied vendor graphics.

The user opens the desktop to see readings, power flow, connection/UPS/mains/battery indicators, shutdown status/countdown and events. Closing hides to the tray. Explicit Quit stops desktop-owned monitoring in direct mode, but never stops a daemon.

## Confirmed decisions

- Available transports: prefer named pipe, otherwise host the same USB engine directly when no other owner exists. Exactly one process owns monitoring. The daemon may run as a normal executable; Windows service installation is optional.
- Desktop is the primary control surface. No Administrator/UAC prompt for normal desktop operations; authorize the configured Windows account through a protected local named pipe.
- In daemon mode, Shutdown-tab policy changes use the protected IPC policy endpoint, are saved to the daemon's dedicated desktop-policy file and apply immediately. The desktop does not edit the daemon's main YAML configuration.
- Web dashboard and API were removed at the user's request on 2026-11-16; see [F-004](F-004-web-dashboard.md), [F-011](F-011-web-access-safety.md) and [F-014](F-014-backend-api.md).
- Telegram is status/checking commands and notifications only. Previous Telegram shutdown/confirmation/cancel commands are superseded, not retained as hidden capabilities.
- Existing build-only/no-real-power-test boundary remains: do not install a service, change active configuration, restart a monitor or request actual shutdown while developing.
- Battery capacity is not reported by Q1; its user-calibrated display estimate is explicitly labeled and never used for shutdown. If an older daemon omits capacity or marks it unavailable but reports battery voltage, the desktop applies the shared estimator client-side; it does not override invalid capacity data or use missing/unreported voltage. Bypass topology and sensor semantics remain unavailable/unverified; screenshots are a workflow reference, not evidence that these measurements exist.
- On 2026-11-17, the user requested removal of the bottom footer and a shorter initial window. The desktop now requests an 800x480 window with the tabs as the full content area; the tab contents retain their existing scrolling where needed.
- On 2026-11-17, the user requested removal of the text at the bottom of Data View. Backend health-warning text is no longer displayed in that tab.
- On 2026-11-17, the user requested an animated power-flow drawing in the Data View center block. The animation follows fresh mains-online or on-battery status; charge, bypass and inverter topology remain explicitly unreported because Q1 exposes no such fields.
- On 2026-11-17, the user requested that the center animation also use the second row's space, with input/frequency and output/load stacked in narrower side columns.
- On 2026-11-21, the user requested a Telegram test button in Setting. The
  desktop requests a single test through the active monitor and displays API or
  relay acceptance without claiming message receipt or a read.
- The 2026-11-21 live outage test exposed [E-011](E-011-mains-loss-not-detected.md):
  Q1 remained mains-like after wall-input disconnection, so no power-change
  event existed for Telegram. Native Windows AC-line state is now a fallback.

## Technical design and first delivery

### Historical three-transport selection - superseded 2026-11-16

The prior implementation added a localhost web API fallback. The user removed this option on 2026-11-16; DESK-12 through DESK-14 and web-address discovery are retired acceptance criteria. The named-pipe-first/direct-USB fallback remains as described above.

### Earlier dual-mode extension - 2026-11-12

The user now requests both direct USB and daemon connections, superseding service-client-only operation. Startup tries the daemon first, then starts the same monitoring engine inside the desktop only if exclusive ownership is available. A running daemon without usable IPC must block direct mode, not permit competing USB access. No automatic service stop, takeover or elevation.

Closing still hides to the tray. Quit stops desktop-owned monitoring in direct mode only; it never stops a daemon. Selection occurs at startup, not by silently changing owners after a connection loss. Display the selected source explicitly.

Extract the existing lifecycle into `internal/monitor` for console, SCM and desktop reuse. Keep isolated/bounded HID helpers in both executables. Direct startup without `--config` uses defaults (USB enabled; hardware, Telegram and shutdown disabled), plus per-user durable policy storage. Previously saved direct-mode policy is restored, including explicitly enabled power controls. An explicitly supplied configuration must load successfully and is never silently replaced.

- DESK-08: a reachable compatible named pipe wins without loading web/direct settings or touching USB.
- DESK-09: absent daemon starts direct monitoring once; existing ownership, legacy conflicts and startup failures remain visible. A daemon winning a startup race is retried rather than replaced.
- DESK-10: direct Quit cancels/joins owned workers; daemon Quit does not stop the daemon. A failed direct runtime cannot silently attach to another owner.
- DESK-11: both executables dispatch bounded HID helper operations without creating a GUI; smoke mode still performs no backend/configuration/USB operations.

- Separate `cmd/upsdesktop` and `internal/desktop`; widgets reuse typed snapshots. In direct mode, `internal/monitor` polls USB in background workers inside the desktop process; widgets still never access hardware.
- Fyne data/status view, simple honest power-flow diagram, event log, tray Show/Quit, policy settings and confirmed desktop shutdown/cancel.
- `internal/control`: bounded local named-pipe transport. Restrict its DACL to SYSTEM, the backend account and the selected desktop user; reject remote clients. Do not add a TCP listener.
- Explicit optional desktop configuration selects the authorized user and durable policy-settings file. Normal console mode may resolve its own user; service accounts require an explicit human user SID.
- Validate changed policy, reject changes during pending/terminal requests, persist atomically before activation, restart qualification rather than inheriting outage timing. No settings UI may write directly to service executable/configuration directories.
- General configuration/Telegram credential provisioning and automatic installation/startup remain separate from first-delivery policy controls; display limitations explicitly rather than simulated success.
- New dependencies: pinned Fyne and go-winio. Fyne requires CGO/GCC during development, already available on this PC. Backend remains separately buildable.

## Acceptance

- DESK-01: actual Windows amd64 GUI build and safe startup; explicit offline/backend-unavailable state, never fake telemetry.
- DESK-02: fresh/stale/lost connection, nullable readings and flags render honestly; outage and policy countdown follow backend state.
- DESK-03: window close hides to tray without stopping monitoring or triggering power actions. Explicit Quit stops only a desktop-owned direct runtime.
- DESK-04: only the authorized local account can use control IPC; malformed/oversized requests and concurrent controls remain bounded.
- DESK-05: shutdown requires desktop confirmation and configured countdown; cancellation and dry-run use the existing controller. Tests inject fake OS requests only.
- DESK-06: successful policy save persists before activation; failed save leaves active policy unchanged; pending/terminal controls cannot be silently reset.
- DESK-07: no TCP/web listener is present; Telegram power commands cannot schedule/cancel anything.
- DESK-15: the Shutdown tab exposes existing policy status, manual confirm/cancel and policy editing; updates use protected IPC and hot-apply without rewriting the main daemon YAML or restarting it.
- DESK-16: the simplified center power-flow schematic animates only for fresh verified mains/on-battery status and stops at unknown/stale status. It shows AC → UPS → output or battery → UPS → output, with no bypass path. When mains is online, the battery branch is shown as connected but charging remains explicitly unreported because Q1 has no verified charging flag.
- DESK-17: the center schematic spans the combined height of the input/frequency and output/load rows; the four readings remain in two side columns.
- DESK-18: Setting shows Telegram health and can request one bounded test
  notification through the active monitor; disabled, blocked and failed states
  are explicit, and no test result is presented as chat receipt.

## Results and next steps

Architecture confirmed with the user; no desktop code was present at planning time. GCC/CGO are available. Fyne v2.8.2 and go-winio v0.6.4 are pinned.

Implemented: separate desktop executable, Data View/readings/conceptual power-flow/status lights, outage/countdown, event viewer, system tray Show/Settings/Quit and a Shutdown tab. The Shutdown tab edits the active policy with explicit dry-run/live confirmation; other USB/hardware/Telegram settings remain backend-file configuration. Charts, scheduling, battery management and import/export are not implemented or falsely exposed as working buttons.

On 2026-11-17, added an animated center schematic: fresh online status animates the mains-to-output path; fresh on-battery status animates from the battery to output; stale or unknown status stops the animation. The schematic labels charge and bypass/inverter state as not reported, matching the current Q1 contract. Focused `go test .\internal\desktop` and `go vet .\internal\desktop` passed. Native visual comparison and live UPS behavior remain unverified.

On 2026-11-17, combined the first two Data View rows into three columns: input/frequency and output/load stack at the sides while the schematic fills the center column. The schematic minimum height was increased to use that combined area. `go test .\internal\desktop` passed, including a layout-structure regression check; native visual comparison remains pending.

The first IPC tests passed against an actual uniquely named Windows pipe with fake store/controller, no TCP listener and no hardware. Widget tests use Fyne's headless driver and check actual readings/units, unavailable capacity, backend-derived countdown, pending controls and connection-loss invalidation. On 2026-11-17, regression tests simulated an older daemon's reported 27.0 V with unavailable capacity and verified the current 27.0 V pack endpoint displays 100.0% estimated (an earlier 27.1 V endpoint displayed 96.8%); another test confirms the bottom of Data View has no text row, the footer is absent, and the window height is 480. `go test .\internal\desktop` passed after the latest UI adjustment. The full Go test suite passed before it and was not rerun. The native OpenGL/CGO executable build is a separate check, not implied by widget tests.

On 2026-11-21, the Telegram card was added to Setting with delivery status, a
test button and an explicit acceptance/read-receipt limitation. Focused desktop,
control, monitor and notification tests plus `go vet .\...` passed. The full suite
is currently blocked by the unrelated 25.55 V capacity assertion documented in
[F-017](F-017-telegram-notifications.md); native GUI rendering and live Telegram
receipt remain pending.

On 2026-11-22, the bypass-style diagram was simplified to the requested
AC-to-output and battery-to-output views. Online status highlights the direct
mains route and shows the battery branch with the explicit note that charging is
not reported; on-battery status highlights the battery route to output. The
unknown/stale state stops animation and greys all paths. Focused desktop tests
and vet pass; native screenshot comparison remains pending.

### Connection and deployment

The daemon needs desktop IPC settings to expose its protected named pipe; new service installations generate these automatically for the installing account. If IPC is absent or unavailable, the desktop attempts direct USB; the global ownership guard prevents it from competing with a running daemon or legacy monitor. No Windows service install or automatic takeover is required.

Without a daemon, direct mode needs no separate service/executable. No-argument startup uses 2-second USB polling, 1-second helper timeout and 10-second staleness. Hardware sampling and Telegram are off. Initial power controls are off; settings saved through the desktop are restored from `%LOCALAPPDATA%\UPSmart\desktop-policy.json`. The directory is created for the current user without elevation. Direct mode uses the same protected pipe, so another desktop may attach as a client of that owner.

After a failed pipe probe, `--config PATH` selects the direct-mode YAML configuration; see [F-008](F-008-configuration.md#yaml-configuration---2026-11-16). Without it, the desktop uses its documented USB-only defaults and per-user policy storage. Configuration is never discovered implicitly or rewritten, and `monitoring_enabled:false` is respected. Invalid chosen configs and corrupt saved policies fail visibly. Source choice is fixed for the session; restart the desktop to select again.

For a console daemon, enable IPC with these flat YAML settings. This is an example only; choose a writable settings path and preserve other configured settings:

```yaml
desktop-settings-file: 'C:\Users\YOUR_USER\AppData\Local\UPSmart\desktop-policy.json'
```

The parent settings directory must already exist and be writable to the backend account. An omitted SID authorizes the console backend's own user. New service installation fills `desktop-user-sid` with the installing account and `desktop-settings-file` with `%ProgramData%\UPSmart\logs\desktop-policy.json`. For another target desktop account, configure that human SID explicitly and retain the service policy path. An existing saved service policy requires deliberate review before automatic IPC setup; no hidden activation of shutdown controls occurs.

Obtain your own SID locally, without elevation:

```powershell
[System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
```

Windows still requires Administrator rights for **service installation**; the application does not bypass this OS requirement or prompt for UAC during ordinary desktop control. The configured service account also needs OS shutdown permission for live shutdown. No account right is granted automatically; denial is a visible failed policy, not a successful shutdown.

Build and run:

```powershell
go build -o .\bin\upsdaemon.exe .\cmd\upsdaemon
go build -ldflags "-H=windowsgui -linkmode=internal" -o .\bin\upsdesktop.exe .\cmd\upsdesktop
# Connect through the daemon pipe, or start direct USB if no monitor owns it:
.\bin\upsdesktop.exe
# Explicit configuration for direct USB fallback:
.\bin\upsdesktop.exe --config .\config.yaml
# Safe offline window startup test; no backend requests or controls:
.\bin\upsdesktop.exe --smoke-test
```

For the configuration beside the built executables, run a foreground daemon in
one Command Prompt and the desktop in another; no service installation is needed:

```bat
bin\upsdaemon.exe serve --config bin\config.yaml
```

```bat
bin\upsdesktop.exe
```

The console daemon configuration must include desktop settings for the named
pipe to accept this client. Otherwise the desktop falls back to direct USB,
which is rejected if the daemon already owns monitoring. Service installation
now supplies these settings automatically; merely running a service with an
older IPC-disabled configuration does not create a pipe.

### Settings and authorization boundaries

- Pipe name is fixed, local-only; normal desktop clients use anonymous impersonation level so the server cannot impersonate their token.
- The DACL grants SYSTEM/backend full access and the selected desktop account read/write without pipe-instance creation rights. Other processes under that account share the same trust boundary; this is account authorization, not per-application authentication.
- Service-account mode with no human SID fails explicitly; no grant to Everyone/Users or automatic installation occurs. Cross-account/LocalService integration is still pending.
- Bounded HTTP framing is carried over the pipe only, not a TCP server: eight admitted requests, 16 KiB settings, 4 KiB confirmation payload, bounded timeouts and 1 MiB client response.
- Saving uses validated policy plus effective-policy revision, preventing stale active settings overwrite. A shutdown request includes the exact mode/countdown the user confirmed; a concurrent settings change requires a new confirmation. Cancellation remains serialized by the existing controller.
- Persistence is temp-write/sync/rename before activation. Failed persistence leaves the active controller unchanged; corrupt/oversized saved settings fail startup and offline CLI validation, never silently fall back.
- Desktop reconfiguration resets automatic timing; it cannot clear pending or lifetime-latched requested/failed/dry-run attempts. Saved policy overrides are read only when desktop configuration explicitly enables their file.

Service installation, real shutdown and live bot testing still require separate approval.

### Actual validation - 2026-11-12

- Full `go test .\...` and `go vet .\...` passed, including real local named-pipe round trip, fake-only policy save/confirmation/cancel, stale edit rejection, corrupted/oversized settings and native widget rendering. Policy reconfiguration tests cover fresh timing and pending/terminal reset rejection.
- Node syntax/DOM tests passed after changing web instructions to desktop controls/checking-only Telegram.
- Native GUI built, then an actual Windows loader test exposed [E-006](E-006-native-desktop-linker.md) with the old external MinGW linker. The documented internal-linker build resolves it without changing installed compilers or patching binaries.
- The internally linked Windows amd64 GUI opened a native window titled `UPSmart Monitor` and exited cleanly with status 0 in `--smoke-test` mode. This mode does not start snapshot polling or send control requests. This verifies actual startup, not visual layout/tray interaction or live service connectivity.
- Backend also built independently with `CGO_ENABLED=0`; offline validation of the unchanged local configuration reports PC shutdown disabled.
- No monitor/service restart, service installation, active configuration edit, real Telegram traffic, USB read or power action was performed in this desktop milestone.

Assessment: first native/service-client candidate, not full legacy-feature parity or a deployed service. Further work: deliberately approved IPC-enabled backend deployment, cross-account/LocalService rights checks, visual/tray interaction, general configuration editors and optional charts. The PC policy editor/controls are implemented; other settings are clearly labelled backend-file-only.

The backend entry was moved concurrently to `cmd/upsdaemon`; its desktop/IPC integration was preserved. Current backend build commands above target that package; earlier root-entry milestone commands elsewhere are historical.

### Dual-mode results - 2026-11-12

- Implemented `internal/monitor.Run`, reused unchanged core lifecycle by console, SCM and direct desktop; both executables share `ups.HandleHelper`. No extra daemon executable is started by direct mode.
- `monitor.Connect` selects a compatible daemon first, otherwise waits for direct readiness. Ownership failure is explicit; a second daemon probe recovers startup races. Selection stays fixed for the session. A stopped direct runtime refuses further requests rather than silently reaching a new owner.
- Header/footer identify direct versus daemon ownership and the consequences of Quit. Window close still hides to tray. Connection startup and monitor stop run off the UI thread; startup can be cancelled by closing its window. Direct Quit joins the shared runtime/workers before exiting; real cleanup failures are shown before closing, not hidden by cancellation.
- Focused tests passed, then full `go test .\...` and `go vet .\...` passed. Monitor/desktop/UPS regressions also passed five repeated runs. Coverage includes daemon preference, blocked ownership before any USB/readiness, startup-race recovery, cancellation/join, runtime failure, unsupported schemas, explicit config preservation and helper dispatch.
- Both Windows candidates built; daemon remains buildable with `CGO_ENABLED=0`. Actual GUI `--smoke-test` opened the native window and exited 0 without connection/configuration/USB actions. Actual GUI `__hid invalid` exited 1 with the expected explicit error, before creating a GUI or touching USB.
- Final offline window launch also emitted a nonfatal tray-readiness warning; actual tray behavior remains unverified. This new observation is open in [E-007](E-007-native-tray-readiness.md), not represented as a fully passing tray test.
- Offline validation of the unchanged local config reports PC shutdown disabled. No normal-mode launch, live USB handover, daemon restart, service installation, live Telegram or power operation was performed.

Assessment: dual-mode selection/lifecycle is implemented and simulation-tested; native loading and helper dispatch are verified. Actual direct USB polling and service/direct ownership handover with the final GUI still need a separately approved live check. No automatic handover after disconnect, service takeover or new power capability is introduced. Existing visual/tray, service-account and full legacy-parity limits remain open.

### Historical three-transport results - 2026-11-12 (removed 2026-11-16)

- Implemented lazy pipe -> web -> USB selection. Web mapping uses existing `/api/v1/status`, `/api/v1/hardware` and `/api/v1/events?limit=100` GET routes without changing the server or configuration. The three cached reads are separate, not an atomic server transaction; partial fetches never publish a successful snapshot.
- Web client accepts numeric loopback only, bypasses proxies, refuses redirects, bounds each response to 1 MiB and the whole refresh to 5 seconds. Invalid/schema-mismatched responses are explicit errors, not permission to fall through to USB.
- UI identifies read-only web mode, retains remote policy/countdown visibility and disables shutdown/cancel regardless of remote flags. Settings explains the pipe requirement without sending a policy request. Web adapter rejects all control operations before network I/O.
- Focused checks, three repeated control/monitor/desktop runs, full `go test .\...`, `go vet .\...` and `build.cmd` passed. Tests include actual loopback web-server/client/session round trips with fake cached state and no USB/service; selection order, race recovery, config discovery, bad responses, exact 1 MiB boundary, redirect/mutation refusal and read-only native widgets.
- No active config edit, daemon/service restart, service installation, live USB poll, real Telegram or power operation was performed. Live desktop attachment to the user's running daemon and final direct hardware handover remain unverified. E-007 stays open.
- Final `build.cmd`, full tests/vet and validation of the unchanged `bin/config.local.json` passed (PC shutdown disabled). Native smoke with a nonexistent config and invalid web address still opened the window and exited 0, proving smoke bypasses connection discovery. The known E-007 tray warning recurred. All 135 local README/project links resolved.

### Web support removal - 2026-11-16

- Removed the TCP dashboard/API, embedded browser assets, web client and desktop web-discovery options. The desktop now connects by named pipe or runs direct USB subject to the existing exclusive-owner guard.
- Removed `listen` and `web_enabled` from configuration; tests verify these old keys are rejected instead of silently ignored. The example configuration no longer contains a listener address.
- Focused config/monitor/desktop/control/daemon tests and full `go test .\...` passed. `go vet .\...` passed. Windows amd64 daemon build with `CGO_ENABLED=0` and internally linked Fyne desktop build both passed; outputs were written to temporary paths, not over the existing `bin` executables.
- No local configuration, running monitor, service, hardware connection or power state was changed or tested. Live daemon attachment and direct USB handover remain pending. A legacy configuration containing `listen` or `web_enabled` must have those keys removed before the current daemon will load it.

### GUI navigation refresh - 2026-11-16

The user initially confirmed the tab order as “Data view | Setting | Log.” Data View retains UPS readings (including UPS-reported temperature) and status; Setting is informational; Log contains event history only. PC hardware diagnostics are not shown in the desktop. This UI change does not alter the optional backend hardware collector. PC-temperature support remains excluded; no PC-temperature provider or reading was added.

The user then requested removal of the items shown in the desktop screenshot. Removed the in-content app heading, toolbar icons, connection-source line, backend status line and power-summary line; the original tabs remained. The native window title and ownership/quit footer are unchanged.

The user then requested removal of PC hardware diagnostics. The Log tab now shows only event history; hardware snapshot rendering was removed from the desktop.

The user reported that the Log text was difficult to read in dark mode. Replaced the disabled entry with read-only rich text using Fyne's theme foreground color and vertical scrolling, so text uses the active theme's high-contrast foreground. On 2026-11-16, focused desktop tests and full `go test .\...` passed. Headless checks cover tab order, initial selection, event-only Log, foreground theme token and updated log text. Native visual comparison in dark mode, tray behavior and live daemon/USB connectivity remain unverified.

The user then requested a Shutdown tab and confirmed policy changes should target daemon-owned settings. The fourth tab groups policy status/countdown, confirmed manual shutdown/cancel and access to the existing policy editor. In daemon mode, save continues through protected `/policy` IPC, persists the dedicated desktop-policy file and applies immediately; it does not edit daemon YAML or require a restart. Separate per-trigger delays are not added; the existing shared countdown model remains. On 2026-11-16, `go test .\internal\desktop` and full `go test .\...` passed. The headless test checks all four tab names/order and daemon-policy messaging. Live daemon reload/reconnection and actual shutdown were not tested.
