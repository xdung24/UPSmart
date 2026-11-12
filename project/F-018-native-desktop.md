# F-018 - Fyne Windows desktop

Updated: 2026-11-12 | Priority: P1 | Status: Desktop/control candidate tested; native window startup verified; not deployed

## Purpose and user journey

Provide a Windows desktop control center similar in workflow to legacy UPSmart, without sharing USB ownership with it. Fyne supplies the native executable/window and its own rendered widgets, not classic Win32 controls. Use original artwork/layout components, not copied vendor graphics.

The user opens the desktop to see readings, power flow, connection/UPS/mains/battery indicators, elapsed outage, active shutdown countdown, hardware and events. Closing hides to the tray; explicit Quit closes only the UI. Monitoring remains in the backend/service.

## Confirmed decisions

- Service-client architecture: the service is the single USB, policy, event and Telegram owner.
- Desktop is the primary control surface. No Administrator/UAC prompt for normal desktop operations; authorize the configured Windows account through a protected local named pipe.
- Web dashboard is optional and remains read-only/loopback-only.
- Telegram is status/checking commands and notifications only. Previous Telegram shutdown/confirmation/cancel commands are superseded, not retained as hidden capabilities.
- Existing build-only/no-real-power-test boundary remains: do not install a service, change active configuration, restart a monitor or request actual shutdown while developing.
- Unsupported battery capacity, bypass topology and sensor semantics must remain unavailable/unverified; screenshots are a workflow reference, not evidence that these measurements exist.

## Technical design and first delivery

- Separate `cmd/upsmart-desktop` and `internal/desktop`; reuse typed backend snapshots, not USB polling from the UI.
- Fyne data/status view, simple honest power-flow diagram, events/hardware views, tray Show/Quit, policy settings and confirmed desktop shutdown/cancel.
- `internal/control`: bounded local named-pipe transport. Restrict its DACL to SYSTEM, the backend account and the selected desktop user; reject remote clients. Do not add writable web routes.
- Explicit optional desktop configuration selects the authorized user and durable policy-settings file. Normal console mode may resolve its own user; service accounts require an explicit human user SID.
- Validate changed policy, reject changes during pending/terminal requests, persist atomically before activation, restart qualification rather than inheriting outage timing. No settings UI may write directly to service executable/configuration directories.
- General configuration/Telegram credential provisioning and automatic installation/startup remain separate from first-delivery policy controls; display limitations explicitly rather than simulated success.
- New dependencies: pinned Fyne and go-winio. Fyne requires CGO/GCC during development, already available on this PC. Backend remains separately buildable.

## Acceptance

- DESK-01: actual Windows amd64 GUI build and safe startup; explicit offline/backend-unavailable state, never fake telemetry.
- DESK-02: fresh/stale/lost connection, nullable readings and flags render honestly; outage and policy countdown follow backend state.
- DESK-03: tray/window close never stops the backend or triggers power actions.
- DESK-04: only the authorized local account can use control IPC; malformed/oversized requests and concurrent controls remain bounded.
- DESK-05: shutdown requires desktop confirmation and configured countdown; cancellation and dry-run use the existing controller. Tests inject fake OS requests only.
- DESK-06: successful policy save persists before activation; failed save leaves active policy unchanged; pending/terminal controls cannot be silently reset.
- DESK-07: optional web-off mode preserves desktop IPC; web mutations remain unavailable. Telegram power commands cannot schedule/cancel anything.

## Results and next steps

Architecture confirmed with the user; no desktop code was present at planning time. GCC/CGO are available. Fyne v2.8.2 and go-winio v0.6.4 are pinned.

Implemented: separate desktop executable, Data View/readings/conceptual power-flow/status lights, outage/countdown, PC hardware and event viewers, system tray Show/Settings/Quit. Settings edit the active shutdown policy with explicit dry-run/live confirmation; other USB/hardware/Telegram settings remain backend-file configuration in this first candidate. Charts, scheduling, battery management and import/export are not implemented or falsely exposed as working buttons.

The first IPC tests passed against an actual uniquely named Windows pipe with fake store/controller, no TCP listener and no hardware. Widget tests use Fyne's headless driver and check actual readings/units, unavailable capacity, backend-derived countdown, pending controls and connection-loss invalidation. The native OpenGL/CGO executable build is a separate check, not implied by widget tests.

### Connection and deployment

Existing local/example files are deliberately unchanged. The new UI cannot control an older backend or one without the optional desktop group. A deliberate future backend configuration/restart is required for live readings; this development task does not perform it.

Example additions for a console backend, **not an active configuration**:

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
go build -o .\bin\upsmart-next.exe .
.\build-desktop.ps1
# Equivalent direct command:
go build -ldflags "-H=windowsgui -linkmode=internal" -o .\bin\upsmart-desktop.exe .\cmd\upsmart-desktop
# UI only; does not start/stop a monitor or query USB:
.\bin\upsmart-desktop.exe
# Safe offline window startup test; no backend requests or controls:
.\bin\upsmart-desktop.exe --smoke-test
```

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
