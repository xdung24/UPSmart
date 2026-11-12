# Errors and observations

Last updated: 2026-11-12

Initial monitoring tests and actual read-only USB/browser checks passed. The later persistence/service/shutdown candidate passes Go tests/vet, fake-only policy/Telegram integration and dashboard checks, but was not deployed because the user chose build-only; see [F-015](F-015-quality.md). Real shutdown, bot delivery and service-account privileges remain pending, not classified as bugs.

| ID | Error or observation | Status / priority | Last update | Details |
| --- | --- | --- | --- | --- |
| E-001 | Optional manufacturer descriptor rejected by this UPS | Fixed/verified / P1 | 2026-11-12 | [Descriptor handling](E-001-manufacturer-descriptor.md) |
| E-002 | Replacement filename mistaken for legacy UPSmart | Fixed/verified / P1 | 2026-11-12 | [Process identity](E-002-process-name-collision.md) |
| E-003 | Race-test binaries exit at startup with 0xc0000139 | Tooling observation blocked / P2 | 2026-11-12 | [Race runtime](E-003-race-runtime-startup.md) |
| E-004 | Physical disk performance object unavailable; activity readings null | Provider observation open / P2 | 2026-11-12 | [Disk counters](E-004-disk-performance-provider.md) |
| E-005 | Requested ACPI temperature source returns Not supported | Closed by user-requested feature removal / P2 | 2026-11-12 | [ACPI temperature](E-005-acpi-temperature-unsupported.md) |
| E-006 | Default Fyne external-link binary rejected by Windows loader | Resolved in documented internal-linker build / P1 | 2026-11-12 | [Desktop linker](E-006-native-desktop-linker.md) |

The user's report of legacy UPSmart crashes is background context, not a reproduced defect in this replacement. Add flat `E-xxx-short-name.md` records only for actual observations, with unknown causes labeled explicitly.
