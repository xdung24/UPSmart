# E-005 - ACPI temperature source unsupported

Updated: 2026-11-12 | Priority: P2 | Status: Closed by user-approved feature removal; source not repaired

## Features and impact

Related: [F-016 hardware monitoring](F-016-pc-hardware-monitoring.md).

Mục tiêu: kiểm tra thư viện người dùng đề xuất và giải thích rõ vì sao chưa có nhiệt độ CPU thực trên máy này.

The requested `northwindlight/cputemp` Windows method cannot supply a reading on this PC. ACPI thermal zones also do not inherently identify CPU package/core sensors, even when they return values.

## Environment and reproduction

2026-11-12, Windows amd64, non-elevated session; reported processor: Intel Xeon E5-2699 v3.

The upstream method queries `MSAcpi_ThermalZoneTemperature` in `root\WMI`, reads `CurrentTemperature` and converts tenths of Kelvin to Celsius. A direct read-only query and the adapted candidate diagnostic were both tested:

```powershell
Get-CimInstance -Namespace 'root\WMI' -ClassName MSAcpi_ThermalZoneTemperature
.\bin\upsdaemon.exe read-hardware
```

Actual result at 19:48 local time: `acpi_temperatures: Not supported`. Diagnostic JSON exposed an unavailable ACPI record with that reason; verified CPU temperature remained null. Independent CPU/GPU/disk readings were preserved. Existing unrelated provider errors also kept the diagnostic exit status nonzero.

Expected: preserve any supported zones with source identity, or explicitly report unavailability; never assume the first returned zone is the CPU.

## Cause, adaptation and verification

- Confirmed: this Windows ACPI query returns `Not supported`. Firmware/provider reasons have not been independently established.
- Copied/adapted the upstream MIT-licensed query/conversion with full attribution and license notice. Kept bounded subprocess collection rather than adding unbounded in-process WMI calls/dependencies.
- Deliberate correction: preserve every zone and label CPU attribution unverified; do not copy the upstream first-zone-as-CPU assumption.
- Focused tests passed for conversion, signed Celsius, missing/invalid readings, multiple zones, attribution, partial failure isolation and embedded-license HTTP access.
- Live retest confirmed explicit unsupported status, not a recovered CPU reading. No BIOS changes, driver installation, elevation, monitor restart or active configuration edit occurred.

## Genuine CPU provider investigation

The user explicitly requested copying/testing first, then investigating a genuine provider if unsuccessful. No running LibreHardwareMonitor, OpenHardwareMonitor, HWiNFO or Core Temp process was found; neither Libre/OpenHardwareMonitor WMI namespace was registered.

LibreHardwareMonitor is a candidate: its Intel CPU implementation recognizes Haswell Xeon E5-2600/1600 v3 and accesses CPU model-specific registers through its current PawnIO helper. The checked machine had no PawnIO uninstall registration or driver entry. This is source-level feasibility, not verified support or a live temperature reading.

Sources:

- [Intel CPU implementation](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/blob/master/LibreHardwareMonitorLib/Hardware/Cpu/IntelCpu.cs)
- [Intel MSR adapter](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/blob/master/LibreHardwareMonitorLib/PawnIo/IntelMsr.cs)
- [PawnIO device access](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/blob/master/LibreHardwareMonitorLib/PawnIo/PawnIo.cs)
- [Project setup, privileges and licensing](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor)

Historical proposed next step was a separately approved privileged provider. It is withdrawn: on 2026-11-12 the user requested removal of PC hardware temperature monitoring from both the app and plan. No further temperature-provider work is planned. The user also independently confirmed that the elevated ACPI query returned `HRESULT 0x8004100c` / `Not supported`.

The ACPI/storage temperature queries, API data, UI rendering and copied reader/license were removed. The observation is closed by explicit scope removal, not successful temperature recovery; retain the evidence above. UPS-reported temperature remains supported.
