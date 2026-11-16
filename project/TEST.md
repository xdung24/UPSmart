# Errors and observations

Last updated: 2026-11-16

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

The user's report of legacy UPSmart crashes is background context, not a reproduced defect in this replacement. Add flat `E-xxx-short-name.md` records only for actual observations, with unknown causes labeled explicitly.
