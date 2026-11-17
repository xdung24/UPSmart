# Errors and observations

Last updated: 2026-11-17

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

The user's report of legacy UPSmart crashes is background context, not a reproduced defect in this replacement. Add flat `E-xxx-short-name.md` records only for actual observations, with unknown causes labeled explicitly.
