# F-018 - Fyne Windows desktop

Updated: 2026-11-16 | Priority: P1 | Status: Daemon named pipe/direct USB tested and built; service installation not required

## Current scope - 2026-11-16

The user requested removal of web support and confirmed that only the desktop app is needed. The supported modes are:

1. Connect to a daemon through its account-restricted local named pipe.
2. If no compatible daemon is available, run the shared monitor directly in the desktop, provided exclusive USB ownership is available.

The desktop does not discover or connect to a web API. The daemon has no TCP listener, browser dashboard or HTTP API. HTTP framing inside the named pipe is local IPC, not web support. Configure the daemon's `desktop` group to accept a desktop connection; a daemon without this group is not remotely attachable. A Windows service remains optional.

## Purpose and user journey

Provide a Windows desktop control center similar in workflow to legacy UPSmart, without sharing USB ownership with it. Fyne supplies the native executable/window and its own rendered widgets, not classic Win32 controls. Use original artwork/layout components, not copied vendor graphics.

The user opens the desktop to see readings, power flow, connection/UPS/mains/battery indicators, elapsed outage, active shutdown countdown, hardware and events. Closing hides to the tray. Explicit Quit stops desktop-owned monitoring in direct mode, but never stops a daemon.

## Confirmed decisions

- Available transports: prefer named pipe, otherwise host the same USB engine directly when no other owner exists. Exactly one process owns monitoring. The daemon may run as a normal executable; Windows service installation is optional.
- Desktop is the primary control surface. No Administrator/UAC prompt for normal desktop operations; authorize the configured Windows account through a protected local named pipe.
- Web dashboard and API were removed at the user's request on 2026-11-16; see [F-004](F-004-web-dashboard.md), [F-011](F-011-web-access-safety.md) and [F-014](F-014-backend-api.md).
- Telegram is status/checking commands and notifications only. Previous Telegram shutdown/confirmation/cancel commands are superseded, not retained as hidden capabilities.
- Existing build-only/no-real-power-test boundary remains: do not install a service, change active configuration, restart a monitor or request actual shutdown while developing.
- Unsupported battery capacity, bypass topology and sensor semantics must remain unavailable/unverified; screenshots are a workflow reference, not evidence that these measurements exist.

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

- Separate `cmd/upsmart-desktop` and `internal/desktop`; widgets reuse typed snapshots. In direct mode, `internal/monitor` polls USB in background workers inside the desktop process; widgets still never access hardware.
- Fyne data/status view, simple honest power-flow diagram, events/hardware views, tray Show/Quit, policy settings and confirmed desktop shutdown/cancel.
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

## Results and next steps

Architecture confirmed with the user; no desktop code was present at planning time. GCC/CGO are available. Fyne v2.8.2 and go-winio v0.6.4 are pinned.

Implemented: separate desktop executable, Data View/readings/conceptual power-flow/status lights, outage/countdown, PC hardware and event viewers, system tray Show/Settings/Quit. Settings edit the active shutdown policy with explicit dry-run/live confirmation; other USB/hardware/Telegram settings remain backend-file configuration in this first candidate. Charts, scheduling, battery management and import/export are not implemented or falsely exposed as working buttons.

The first IPC tests passed against an actual uniquely named Windows pipe with fake store/controller, no TCP listener and no hardware. Widget tests use Fyne's headless driver and check actual readings/units, unavailable capacity, backend-derived countdown, pending controls and connection-loss invalidation. The native OpenGL/CGO executable build is a separate check, not implied by widget tests.

### Connection and deployment

The daemon must include the `desktop` configuration group to expose its protected named pipe. If IPC is absent or unavailable, the desktop attempts direct USB; the global ownership guard prevents it from competing with a running daemon or legacy monitor. No Windows service install or automatic takeover is required.

Without a daemon, direct mode needs no separate service/executable. No-argument startup uses 2-second USB polling, 1-second helper timeout and 10-second staleness. Hardware sampling and Telegram are off. Initial power controls are off; settings saved through the desktop are restored from `%LOCALAPPDATA%\UPSmartMonitor\desktop-policy.json`. The directory is created for the current user without elevation. Direct mode uses the same protected pipe, so another desktop may attach as a client of that owner.

After a failed pipe probe, `--config PATH` selects the direct-mode configuration. Without it, the desktop uses its documented USB-only defaults and per-user policy storage. Configuration is never discovered implicitly or rewritten, and `monitoring_enabled:false` is respected. Invalid chosen configs and corrupt saved policies fail visibly. Source choice is fixed for the session; restart the desktop to select again.

To allow a same-account desktop to connect to a daemon, add `desktop` to its configuration. This is an example only; choose a writable settings path and preserve other configured settings:

```json
"desktop": {
  "settings_file": "C:\\Users\\YOUR_USER\\AppData\\Local\\UPSmartMonitor\\desktop-policy.json"
}
```

The parent settings directory must already exist and be writable to the backend account. Omitted user SID authorizes the console backend's own user only. For a service, specify `user_sid` explicitly and use the exact protected `%ProgramData%\UPSmartMonitor\logs\desktop-policy.json` path. The existing installer creates/protects that logs directory; no new writable executable/configuration directory is introduced.

Obtain your own SID locally, without elevation:

```powershell
[System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
```

Windows still requires Administrator rights for **service installation**; the application does not bypass this OS requirement or prompt for UAC during ordinary desktop control. The configured service account also needs OS shutdown permission for live shutdown. No account right is granted automatically; denial is a visible failed policy, not a successful shutdown.

Build and run:

```powershell
go build -o .\bin\upsmart-next.exe .\cmd\ups-daemon
go build -ldflags "-H=windowsgui -linkmode=internal" -o .\bin\upsmart-desktop.exe .\cmd\upsmart-desktop
# Connect through the daemon pipe, or start direct USB if no monitor owns it:
.\bin\upsmart-desktop.exe
# Explicit configuration for direct USB fallback:
.\bin\upsmart-desktop.exe --config .\config.local.json
# Safe offline window startup test; no backend requests or controls:
.\bin\upsmart-desktop.exe --smoke-test
```

For the configuration beside the built executables, run a foreground daemon in
one Command Prompt and the desktop in another; no service installation is needed:

```bat
bin\upsmart-next.exe serve --config bin\config.local.json
```

```bat
bin\upsmart-desktop.exe
```

The daemon configuration must include the `desktop` group for the named pipe to
accept this client. Otherwise the desktop falls back to direct USB, which will
be rejected if the daemon already owns monitoring.

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

The backend entry was moved concurrently to `cmd/ups-daemon`; its desktop/IPC integration was preserved. Current backend build commands above target that package; earlier root-entry milestone commands elsewhere are historical.

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
