# F-018 - Fyne Windows desktop

Updated: 2026-11-12 | Priority: P1 | Status: Pipe/web/direct candidate tested/built; service installation not required

## Purpose and user journey

Provide a Windows desktop control center similar in workflow to legacy UPSmart, without sharing USB ownership with it. Fyne supplies the native executable/window and its own rendered widgets, not classic Win32 controls. Use original artwork/layout components, not copied vendor graphics.

The user opens the desktop to see readings, power flow, connection/UPS/mains/battery indicators, elapsed outage, active shutdown countdown, hardware and events. Closing hides to the tray. Explicit Quit stops desktop-owned monitoring in direct mode, but never stops a daemon.

## Confirmed decisions

- Available transports: prefer named pipe, then localhost web API, otherwise host the same USB engine directly. Exactly one process owns monitoring. The daemon may run as a normal executable; Windows service installation is optional.
- Desktop is the primary control surface. No Administrator/UAC prompt for normal desktop operations; authorize the configured Windows account through a protected local named pipe.
- Web dashboard is optional and remains read-only/loopback-only.
- Telegram is status/checking commands and notifications only. Previous Telegram shutdown/confirmation/cancel commands are superseded, not retained as hidden capabilities.
- Existing build-only/no-real-power-test boundary remains: do not install a service, change active configuration, restart a monitor or request actual shutdown while developing.
- Unsupported battery capacity, bypass topology and sensor semantics must remain unavailable/unverified; screenshots are a workflow reference, not evidence that these measurements exist.

## Technical design and first delivery

### Confirmed transport selection - 2026-11-12

The desktop must use whatever is available: named pipe, localhost web API, or direct USB, in that order. A Windows service is not required: a foreground daemon can provide either transport. HTTP mode uses existing cached GET APIs only and disables policy editing/shutdown/cancel; full controls still require the protected pipe. No writable web routes are introduced.

After a failed pipe probe, discover the web address from `--web-address`, explicit `--config`, adjacent `config.local.json`, current-directory `config.local.json`, or the default `127.0.0.1:8080`, in that order. Configuration errors must be visible. Discovery never rewrites configuration or implicitly enables its direct-mode settings. Selection stays fixed for the session; direct USB remains protected by the global owner guard.

- DESK-12: pipe wins without web/config access; web wins without starting USB; only unavailable remote transports allow a direct attempt.
- DESK-13: web-only readings/hardware/events retain honest quality and freshness. Power controls remain disabled even if the remote policy is enabled/pending; client-side mutation attempts produce an explicit error without network I/O.
- DESK-14: web access is bounded, numeric IPv4 loopback only, with no proxy/redirects. Failed/invalid/oversized responses do not publish partial snapshots. No service/configuration change is needed for an existing compatible web daemon.

### Earlier dual-mode extension - 2026-11-12

The user now requests both direct USB and daemon connections, superseding service-client-only operation. Startup tries the daemon first, then starts the same monitoring engine inside the desktop only if exclusive ownership is available. A running daemon without usable IPC must block direct mode, not permit competing USB access. No automatic service stop, takeover or elevation.

Closing still hides to the tray. Quit stops desktop-owned monitoring in direct mode only; it never stops a daemon. Selection occurs at startup, not by silently changing owners after a connection loss. Display the selected source explicitly.

Extract the existing lifecycle into `internal/monitor` for console, SCM and desktop reuse. Keep isolated/bounded HID helpers in both executables. Direct startup without `--config` initially uses read-only defaults (USB enabled; web, hardware, Telegram and shutdown disabled), plus per-user durable policy storage. Previously saved direct-mode policy is restored, including explicitly enabled power controls. An explicitly supplied configuration must load successfully and is never silently replaced.

- DESK-08: a reachable compatible named pipe wins without loading web/direct settings or touching USB.
- DESK-09: absent daemon starts direct monitoring once; existing ownership, legacy conflicts and startup failures remain visible. A daemon winning a startup race is retried rather than replaced.
- DESK-10: direct Quit cancels/joins owned workers; daemon Quit does not stop the daemon. A failed direct runtime cannot silently attach to another owner.
- DESK-11: both executables dispatch bounded HID helper operations without creating a GUI; smoke mode still performs no backend/configuration/USB operations.

- Separate `cmd/upsmart-desktop` and `internal/desktop`; widgets reuse typed snapshots. In direct mode, `internal/monitor` polls USB in background workers inside the desktop process; widgets still never access hardware.
- Fyne data/status view, simple honest power-flow diagram, events/hardware views, tray Show/Quit, policy settings and confirmed desktop shutdown/cancel.
- `internal/control`: bounded local named-pipe transport. Restrict its DACL to SYSTEM, the backend account and the selected desktop user; reject remote clients. Do not add writable web routes.
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
- DESK-07: optional web-off mode preserves desktop IPC; web mutations remain unavailable. Telegram power commands cannot schedule/cancel anything.

## Results and next steps

Architecture confirmed with the user; no desktop code was present at planning time. GCC/CGO are available. Fyne v2.8.2 and go-winio v0.6.4 are pinned.

Implemented: separate desktop executable, Data View/readings/conceptual power-flow/status lights, outage/countdown, PC hardware and event viewers, system tray Show/Settings/Quit. Settings edit the active shutdown policy with explicit dry-run/live confirmation; other USB/hardware/Telegram settings remain backend-file configuration in this first candidate. Charts, scheduling, battery management and import/export are not implemented or falsely exposed as working buttons.

The first IPC tests passed against an actual uniquely named Windows pipe with fake store/controller, no TCP listener and no hardware. Widget tests use Fyne's headless driver and check actual readings/units, unavailable capacity, backend-derived countdown, pending controls and connection-loss invalidation. The native OpenGL/CGO executable build is a separate check, not implied by widget tests.

### Connection and deployment

Existing local/example files are deliberately unchanged. A running foreground daemon/web server is sufficient: the desktop first tries its named pipe, then its existing web API. Only full power controls require the optional desktop IPC group. If neither transport is usable, a running owner's guard still blocks competing USB access. No Windows service install, monitor restart or configuration change is performed by this development task.

Without a daemon, direct mode needs no separate service/executable. No-argument startup uses 2-second USB polling, 1-second helper timeout and 10-second staleness. Web, hardware sampling and Telegram are off. Initial power controls are off; settings saved through the desktop are restored from `%LOCALAPPDATA%\UPSmartMonitor\desktop-policy.json`. The directory is created for the current user without elevation. Direct mode uses the same protected pipe, so another desktop may attach as a client of that owner.

After a failed pipe probe, `--web-address 127.0.0.1:PORT` overrides web discovery. Otherwise `--config PATH` supplies the web address, or discovery looks for `config.local.json` beside the executable and then in the working directory. With neither file, it tries `127.0.0.1:8080`. `web_enabled:false` skips web discovery unless an address was explicitly supplied. Discovery does not activate that file's power/Telegram/hardware settings. A connected pipe skips discovery entirely.

If both remote transports are unavailable, `--config PATH` also selects the direct-mode configuration. Omitted direct config still uses USB-only defaults rather than silently enabling settings from a discovered file. It never rewrites a file or overrides `monitoring_enabled:false`. Existing desktop policy storage is respected; otherwise the per-user path above is used. Missing/invalid chosen configs, corrupt saved policies and incompatible web snapshots fail visibly. Source choice is fixed for the session; restart the desktop to select again.

Optional additions for full controls on a console backend, **not an active configuration**:

```json
"web_enabled": false,
"desktop": {
  "settings_file": "C:\\Users\\YOUR_USER\\AppData\\Local\\UPSmartMonitor\\desktop-policy.json"
}
```

The parent settings directory must already exist and be writable to the backend account. Omitted user SID authorizes the console backend's own user only. For a service, specify `user_sid` explicitly and use the exact protected `%ProgramData%\UPSmartMonitor\logs\desktop-policy.json` path (expand environment variables in the JSON string). The existing installer creates/protects that logs directory; no new writable executable/configuration directory is introduced.

Obtain your own SID locally, without elevation:

```powershell
[System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
```

Windows still requires Administrator rights for **service installation**; the application does not bypass this OS requirement or prompt for UAC during ordinary desktop control. The configured service account also needs OS shutdown permission for live shutdown. No account right is granted automatically; denial is a visible failed policy, not a successful shutdown.

Build and run:

```powershell
go build -o .\bin\upsmart-next.exe .\cmd\ups-daemon
go build -ldflags "-H=windowsgui -linkmode=internal" -o .\bin\upsmart-desktop.exe .\cmd\upsmart-desktop
# Prefer named pipe, then web API, otherwise direct USB:
.\bin\upsmart-desktop.exe
# Explicit configuration for web address discovery and direct fallback:
.\bin\upsmart-desktop.exe --config .\config.local.json
# Explicit localhost web endpoint; pipe is still preferred:
.\bin\upsmart-desktop.exe --web-address 127.0.0.1:18765
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

The desktop discovers `bin\config.local.json` and uses its web `listen` address
when no pipe is available. Web mode displays readings, hardware, events and active
policy/countdown, but never edits policy or sends shutdown/cancel.

### Settings and authorization boundaries

- Pipe name is fixed, local-only; normal desktop clients use anonymous impersonation level so the server cannot impersonate their token.
- The DACL grants SYSTEM/backend full access and the selected desktop account read/write without pipe-instance creation rights. Other processes under that account share the same trust boundary; this is account authorization, not per-application authentication.
- Service-account mode with no human SID fails explicitly; no grant to Everyone/Users or automatic installation occurs. Cross-account/LocalService integration is still pending.
- Bounded HTTP framing is carried over the pipe only, not a TCP mutation server: eight admitted requests, 16 KiB settings, 4 KiB confirmation payload, bounded timeouts and 1 MiB client response.
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

### Three-transport results - 2026-11-12

- Implemented lazy pipe -> web -> USB selection. Web mapping uses existing `/api/v1/status`, `/api/v1/hardware` and `/api/v1/events?limit=100` GET routes without changing the server or configuration. The three cached reads are separate, not an atomic server transaction; partial fetches never publish a successful snapshot.
- Web client accepts numeric loopback only, bypasses proxies, refuses redirects, bounds each response to 1 MiB and the whole refresh to 5 seconds. Invalid/schema-mismatched responses are explicit errors, not permission to fall through to USB.
- UI identifies read-only web mode, retains remote policy/countdown visibility and disables shutdown/cancel regardless of remote flags. Settings explains the pipe requirement without sending a policy request. Web adapter rejects all control operations before network I/O.
- Focused checks, three repeated control/monitor/desktop runs, full `go test .\...`, `go vet .\...` and `build.cmd` passed. Tests include actual loopback web-server/client/session round trips with fake cached state and no USB/service; selection order, race recovery, config discovery, bad responses, exact 1 MiB boundary, redirect/mutation refusal and read-only native widgets.
- No active config edit, daemon/service restart, service installation, live USB poll, real Telegram or power operation was performed. Live desktop attachment to the user's running daemon and final direct hardware handover remain unverified. E-007 stays open.
- Final `build.cmd`, full tests/vet and validation of the unchanged `bin/config.local.json` passed (PC shutdown disabled). Native smoke with a nonexistent config and invalid web address still opened the window and exited 0, proving smoke bypasses connection discovery. The known E-007 tray warning recurred. All 135 local README/project links resolved.
