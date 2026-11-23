# Errors and observations

Last updated: 2026-11-23

Log rendering: on 2026-11-23, repeated Fyne RichText refreshes caused the Log
tab to become blank or show clipped fragments. The renderer now uses a wrapped
Label in a vertical scroll container. A repeated 100-line refresh test and empty
history test pass, along with `go vet .\internal\desktop`. Native screenshot
verification remains pending. Details:
[E-012](E-012-log-content-disappears.md).

Intermittent `0x05`: on 2026-11-23 a live Telegram connection alert contained
one control byte instead of a Q1 frame. This is not a mains/power event. The
persistent helper now treats `0x05` as transient refresh and retains the last
verified Q1 without emitting a false connection alert; cold startup gets a
bounded six-request refresh allowance. Details:
[E-008](E-008-q1-startup-control-response.md#intermittent-0x05-message---2026-11-23).
Focused UPS worker/parser, state, monitor and notification tests plus targeted
vet passed on 2026-11-23.

Telegram message icons: on 2026-11-22, direct and relay notification formatting
was enhanced with event-specific icons and readable titles while retaining UTC
time, safe detail, relay Markdown escaping and the existing 3,500-rune bound.
Focused notification/control/desktop/monitor/state tests and `go vet .\...`
passed. Live visual receipt remains pending. Details:
[F-017](F-017-telegram-notifications.md#enhanced-telegram-message-format---2026-11-22).

Power-flow chart simplification: on 2026-11-22, the desktop diagram's bypass
path was removed. Fresh online state now shows AC → UPS → output and an honest
charging-unreported battery note; fresh on-battery state shows battery → UPS →
output; unknown/stale greys all paths and stops animation. `go test -count=1
.\internal\desktop` and `go vet .\internal\desktop` passed. Native visual review
remains pending. Details: [F-018](F-018-native-desktop.md).

Desktop icon: on 2026-11-22, `car-battery.ico` was embedded as the Windows
executable resource and bundled as a PNG-backed Fyne app/window/tray resource.
`go test -count=1 .\internal\desktop .\cmd\upsdesktop` and desktop vet passed;
a Windows GUI build succeeded and `Icon.ExtractAssociatedIcon` returned a 32×32
icon. Details: [F-018](F-018-native-desktop.md).

Mains-loss notification live observation: the stale online condition was not a Telegram failure. UPSmart's HID session keeps the device handle open with read/write, overlapped access and cycles Q1/Q1/Q1/F/BL/I; reopening a metadata-only handle left descriptor 3 stale. The replacement now uses a persistent helper and a one-way Windows offline fallback. Outage/restoration events and restoration Telegram delivery are live-verified; installed cold-start/reboot retest remains pending. Details: [E-011](E-011-mains-loss-not-detected.md).

UPS status freshness observation: without legacy UPSmart, repeated index-3 reads can remain cached at online even after an actual mains change. A documented MEC0003 Hunnox trace confirms the vendor cycles read-only descriptor indices Q1/Q1/Q1/F/BL/I; the omitted `BL` (`0xf3`) phase was added to the persistent monitor cycle on 2026-11-21. Focused tests and vet pass; live unplug/replug verification remains mandatory before closing [E-008](E-008-q1-startup-control-response.md).

2026-11-22 clean final-binary verification: legacy UPSmart and the service were stopped; `upsdaemon-re-engineered.exe` started from a fresh journal at 05:30 with an online baseline. Unplug produced `on_battery` at 05:31:42 and Telegram receipt; reconnect produced `online` at 05:33:30 and Telegram receipt. No connection errors occurred. Pre-17:06 diagnostic events from 2026-11-21 are invalid acceptance evidence. Details: [E-008](E-008-q1-startup-control-response.md), [E-011](E-011-mains-loss-not-detected.md).

Telegram desktop test: on 2026-11-21, focused notification/control/desktop/monitor tests and `go vet .\...` passed. The Setting action waits for Telegram API or configured relay acceptance through the existing serialized worker and explicitly does not claim chat receipt. No live message was sent. `go test .\...` remains blocked by an unrelated `internal/ups` capacity assertion at 25.55 V (expects 50.0%, while 24.0-27.0 V linear mapping yields 51.666667%). Details: [F-017](F-017-telegram-notifications.md#desktop-test-notification---2026-11-21), [F-018](F-018-native-desktop.md).

Telegram power/connection alerts: local relay selection configured on 2026-11-17; state/notification/config tests and both local YAML offline validations passed. Restart and live delivery remain pending, not a confirmed bug. Details: [F-017](F-017-telegram-notifications.md#selected-power-and-connection-alerts---2026-11-17).

Desktop navigation and layout: Data View / Setting / Shutdown / event-only Log tabs; screenshot header, PC hardware diagnostics, bottom footer and Data View bottom text removed. On 2026-11-17, `go test .\internal\desktop` passed; its headless test verified the tabs are the full content area, the footer is absent, Data View has no bottom text row, and the window height is 480. Full tests were not rerun after this UI change. Live daemon reload, native visual and live hardware checks remain pending. Details: [F-018](F-018-native-desktop.md).

Power-flow drawing: on 2026-11-17, `go test .\internal\desktop` and `go vet .\internal\desktop` passed; tests cover fresh online/on-battery animation state and unknown/stale labeling. Q1 exposes only mains/on-battery mode, so charging and bypass/inverter topology are labeled not reported; actual native visual appearance and live UPS transitions remain pending. Details: [F-018](F-018-native-desktop.md).

Data View layout: on 2026-11-17, `go test .\internal\desktop` passed with a headless check that the flow schematic occupies the center column spanning the input/frequency and output/load stacks. Native visual confirmation remains pending. Details: [F-018](F-018-native-desktop.md).

Battery capacity estimate: user-provided 24.0-27.0 V AR2000 pack endpoints are implemented as a labeled, clamped linear estimate; policy continues to use only verified UPS flags. A read-only snapshot showed the connected, fresh daemon returned 27.0 V but omitted capacity; the desktop now estimates client-side for this older-daemon payload. With the current 27.0 V endpoint, regression tests verify 100.0% estimated; an earlier 27.1 V endpoint yielded 96.8%. Full `go test .\...` passed on 2026-11-17; live reconnection and battery calibration remain pending. No daemon was restarted. Details: [F-002](F-002-telemetry.md), [F-018](F-018-native-desktop.md) and [F-009](F-009-pc-shutdown.md).

Standard-user desktop: read-only inspection found a Running LocalSystem service without desktop IPC settings/pipe; installation now supplies restricted user authorization automatically. Five repeated focused test runs, vet, daemon rebuild and offline config validation passed. Live reconnection pending; no agent-initiated service action occurred. [E-010](E-010-service-desktop-ipc-configuration.md).

rdpproxy alignment: flat YAML/defaults/CLI and LocalSystem service lifecycle/deployment tests, full tests/vet, both Windows builds and built offline config validation passed on 2026-11-16. No actual SCM or hardware execution. Current contracts: [F-006](F-006-windows-service.md#current-service-model-rdpproxy-alignment---2026-11-16), [F-008](F-008-configuration.md#current-rdpproxy-style-options---2026-11-16). Earlier service-placement evidence below predates this replacement.

Service configuration placement: executable-adjacent destination/legacy command tests, targeted configuration/service/daemon tests and vet, and daemon build passed on 2026-11-16. Actual protected copy/SCM installation remains pending, not a bug. Details: [F-006](F-006-windows-service.md#configuration-beside-the-installed-daemon---2026-11-16).

YAML configuration migration: full tests/vet, both Windows builds, and built-daemon validation of the example and both converted local files passed on 2026-11-16. Live restart/service migration remains pending, not a bug. Details: [F-008](F-008-configuration.md#yaml-configuration---2026-11-16).

Optional rdpwatcher notification relay: configuration, loopback gRPC mocks, direct-delivery regressions, full tests/vet and both Windows builds passed on 2026-11-16. Actual relay/Telegram delivery remains pending, not a bug. Evidence and upstream acknowledgement limitation: [F-017](F-017-telegram-notifications.md#optional-rdpwatcher-relay---2026-11-16).

Initial monitoring and read-only USB/browser checks passed; the browser support was later removed at the user's request on 2026-11-16. The desktop now supports daemon named-pipe or standalone direct-USB operation. Removal and regression checks are recorded in [F-018](F-018-native-desktop.md) and [F-015](F-015-quality.md). Real shutdown, bot delivery and service-account privileges remain pending, not classified as bugs.

Dual-mode desktop selection/lifecycle tests, Windows builds and offline native startup/helper checks passed; final GUI live USB/handover is pending, not a bug. Evidence: [F-018](F-018-native-desktop.md).

The prior pipe/web/USB selection and loopback API checks are historical evidence only. Web support was removed on 2026-11-16; current transport selection is pipe/direct USB. Live attachment to the user's daemon remains pending; no service install is required. See [F-018](F-018-native-desktop.md).

| ID | Error or observation | Status / priority | Last update | Details |
| --- | --- | --- | --- | --- |
| E-001 | Optional manufacturer descriptor rejected by this UPS | Fixed/verified / P1 | 2026-11-12 | [Descriptor handling](E-001-manufacturer-descriptor.md) |
| E-002 | Replacement filename mistaken for legacy UPSmart | Fixed/verified / P1 | 2026-11-12 | [Process identity](E-002-process-name-collision.md) |
| E-003 | Race-test binaries exit at startup with 0xc0000139 | Tooling observation blocked / P2 | 2026-11-12 | [Race runtime](E-003-race-runtime-startup.md) |
| E-004 | Physical disk performance object unavailable; activity readings null | Provider observation open / P2 | 2026-11-12 | [Disk counters](E-004-disk-performance-provider.md) |
| E-005 | Requested ACPI temperature source returns Not supported | Closed by user-requested feature removal / P2 | 2026-11-12 | [ACPI temperature](E-005-acpi-temperature-unsupported.md) |
| E-006 | Default Fyne external-link binary rejected by Windows loader | Resolved in documented internal-linker build / P1 | 2026-11-12 | [Desktop linker](E-006-native-desktop-linker.md) |
| E-007 | Offline native smoke emitted a tray-not-ready warning; window/exit succeeded | Observation open / P2 | 2026-11-12 | [Tray readiness](E-007-native-tray-readiness.md) |
| E-008 | Q1 startup returns 0x05; one status read succeeds after vendor handover | Observation open / P1; independent startup unresolved | 2026-11-16 | [Q1 startup](E-008-q1-startup-control-response.md) |
| E-009 | Installer rejects equivalent Windows-normalized protected ACL | Historical fix verified / P1; installer replaced by user choice | 2026-11-16 | [Service ACL comparison](E-009-service-acl-normalization.md) |
| E-010 | Service running without desktop IPC settings; normal-user desktop cannot connect | Configuration/default-setup observation / P1; defaults fixed, live retest pending | 2026-11-16 | [Desktop/service configuration](E-010-service-desktop-ipc-configuration.md) |
| E-011 | Reopened metadata-only HID handle left Q1 stale, suppressing Telegram transitions | Fixed and live-verified / P1; install/reboot follow-up pending | 2026-11-22 | [Mains-loss detection](E-011-mains-loss-not-detected.md) |
| E-012 | Log content disappears after repeated Fyne RichText refreshes | Fixed and regression-tested / P1; native visual retest pending | 2026-11-23 | [Log rendering](E-012-log-content-disappears.md) |

The user's report of legacy UPSmart crashes is background context, not a reproduced defect in this replacement. Add flat `E-xxx-short-name.md` records only for actual observations, with unknown causes labeled explicitly.
