# F-016 - PC hardware monitoring

Updated: 2026-11-16 | Priority: P2 | Status: Optional collector and desktop view retained; hardware support partial

## Purpose and user journey

Mục tiêu: xem tải CPU, GPU và thông tin ổ đĩa trong cùng desktop app với UPS, phân biệt rõ dữ liệu không hỗ trợ và dữ liệu đã cũ.

The user opens the desktop app and sees a separate PC hardware section. Missing metrics do not prevent UPS monitoring or display misleading zero readings.

Current scope decision, 2026-11-12: the user removed all PC hardware temperature monitoring from both the app and plan. CPU/GPU/disk/ACPI temperature collection, placeholders, fields, UI and provider/driver investigations are no longer planned. Existing UPS temperature in [F-002](F-002-telemetry.md) is unchanged.

## Requested scope and proposed measurements

Current CPU, GPU and hard disk scope, after the temperature removal:

| Group | Proposed readings |
| --- | --- |
| CPU | Total utilization (%); model remains a possible inventory enhancement |
| GPU | Adapter name, utilization (%), used/total dedicated memory and shared memory usage, per adapter |
| Hard disk / SSD | Physical device model/capacity, used/free/total volume space, supported utilization and read/write throughput; mapping/health detail remain unimplemented enhancements |

Label physical disks separately from logical volumes. Do not equate free space with disk health or UPS load with CPU utilization. SMART/NVMe health detail is conditional on provider/device support; do not infer a healthy disk from missing data.

## Proposed implementation and boundaries

- Use Windows performance/storage APIs for utilization, capacity and throughput. No PC temperature source or sensor driver is included.
- Keep sampling independent of USB polling, with bounded timeouts and retry/backoff. A failed or slow hardware provider must not delay UPS state updates or shutdown-policy evaluation.
- Proposed entry point: `internal/hardware`; desktop reads cached state rather than accessing hardware directly.
- Use a separately timestamped cached hardware snapshot, with stable device/sensor identifiers, units, nullable values, explicit availability/error reasons and freshness. CPU utilization and disk throughput need valid sampling intervals; initial samples are unavailable until a baseline exists.
- PC data is sampled independently and included in cached desktop snapshots; the earlier HTTP endpoint/dashboard route was removed with web support on 2026-11-16.
- Add validated opt-in configuration and bounded sampling intervals through [F-008](F-008-configuration.md). Defaults and resource budgets must be decided before implementation; do not change current local configuration.
- Reuse the authorized local named-pipe transport described in [F-018](F-018-native-desktop.md). Avoid exposing disk serial numbers or unnecessary host identifiers.
- No writes to hardware settings, disk tests/repairs, fan control, overclocking or automatic hardware-triggered shutdown. Do not silently elevate privileges or install a driver/helper.
- Missing optional PC telemetry must not make UPS readiness fail. Surface its own degraded state and actionable diagnostics.

## Acceptance and planned tests

- HW-01: available CPU/GPU/disk readings show the correct device, units and timestamps; unsupported measurements show an explicit reason, never a fabricated value.
- HW-02: multiple GPUs/disks, integrated GPUs without dedicated memory, and disk-to-volume mapping remain unambiguous.
- HW-03: provider failure, timeout, device removal and stale samples do not block UPS polling or corrupt last-known data; recovery is visible.
- HW-04: utilization/throughput calculations handle initial samples, invalid intervals and counter resets without negative or non-finite readings.
- HW-05: cached desktop reads trigger no additional hardware queries and retain the named-pipe access restrictions.
- HW-06: compare live readings against Windows Task Manager over recorded sampling intervals; document expected differences rather than claiming exact parity.
- HW-07: verify permissions and provider availability in both console and LocalService contexts before claiming background-service support.
- HW-08: measure sampling CPU/memory overhead against an agreed budget before enabling the feature by default.
- HW-09: PC temperature data and queries are absent; removing them cannot degrade otherwise valid hardware availability. UPS temperature remains available through its existing separate telemetry path.

## Removal verification - 2026-11-12

Removed PC temperature types/placeholders, storage/ACPI queries and decoding, availability accounting, dashboard rendering, copied reader/tests and its license route/link. No configuration or Go dependency change was needed.

Full tests, vet, JavaScript syntax and the separate candidate build passed. A focused regression verifies otherwise complete CPU/GPU/disk data is available without temperature-related degradation and exposes no `temperatures` field. The removed license route now returns 404.

The PC-only live diagnostic at 20:11 local time retained CPU utilization, GPU records and two logical volumes, with no PC temperature field or retired temperature-provider warnings. Existing physical-disk counter errors remain separately tracked in [E-004](E-004-disk-performance-provider.md); diagnostic exit 1 correctly reflects those errors, not a temperature failure.

A scripted DOM check verified CPU/GPU/disk cards remain, temperature cards are absent even for an old response containing them, and separate UPS temperature still renders as `29 C`. No actual UPS query, monitor restart, active configuration edit or driver change was performed. Historical temperature evidence is retained below and in closed [E-005](E-005-acpi-temperature-unsupported.md).

## Historical decisions and results

The temperature work and its proposed next steps below are historical, superseded by the user's removal decision above. They are retained as evidence, not an active temperature backlog.

2026-11-12: added to the plan at the user's request. Documentation only; no hardware discovery, provider installation, application/configuration changes or runtime tests performed. No CPU/GPU/disk sensor support has been verified on this PC.

Proposed P2 priority, exact measurements, polling interval, resource budget and provider choice remain open. Next: inventory supported read-only Windows sources, assess sensor-provider options and permissions, then finalize the contract and implementation order. This workstream does not require or authorize any UPS/PC power-control tests.

### Collector candidate - 2026-11-12

The user requested continued coding of all workstreams. Implemented `internal/hardware`, opt-in `hardware.interval` configuration (10 seconds-5 minutes), independent sampling, cached `/api/v1/hardware` and a separate dashboard section. Existing configurations leave this disabled. This route/view integration is historical; the current desktop consumes cached snapshots over named-pipe IPC or local direct monitoring.

Actual sources: native `GetSystemTimes` CPU utilization, DXGI GPU inventory/memory capacity, Windows PDH GPU engine/memory and physical-disk activity counters, logical-volume space, and optional Windows storage-reliability temperature data. Cross-tool measurement accuracy remains unverified. Device/volume mapping is not guessed.

Snapshots contain `cpu`, `gpus`, `disks` (separate `volumes` and `physical` lists), `temperatures`, availability and warnings. Measurements carry nullable `value`, `unit`, `source` and `reason`. Missing sources preserve other available metrics and expose partial/unavailable reasons.

Limitations: CPU model/per-core readings, full SMART/NVMe health detail, reliable disk-to-volume mapping, multi-processor-group CPU totals and motherboard sensors are not implemented. Standard Windows sources do not provide trustworthy universal CPU/GPU temperatures; these remain explicitly unavailable. Disk temperatures depend on device/provider support and permissions; no elevation or sensor driver installation occurs.

Focused source/parser/delta tests, cancellation/bounded-output tests, PowerShell syntax and embedded native-helper compilation passed without installing dependencies. The initial live read found two DXGI adapter records and volume data, but CIM performance queries timed out and storage-temperature access was denied. The performance source was subsequently changed to bounded native PDH sampling rather than hiding these failures.

No UPS query, active configuration edit, service installation or monitoring restart is part of hardware validation. No temperature threshold or hardware-triggered power action is implemented. Resource-budget/long-duration testing, sensor-provider expansion and LocalService verification remain pending.

Final live read at 19:29 local time: CPU utilization 9.03%; supported GPU engine/memory counters returned values; two logical volumes and two physical disk inventory/capacity records were present. Volume free + used exactly matched total. DXGI and WDDM identities that do not match remain separately labeled instead of being guessed into one physical GPU.

Physical-disk activity remained unavailable with explicit per-counter `PDH_CSTATUS_NO_OBJECT` reasons; see [E-004](E-004-disk-performance-provider.md). Disk temperature access remained denied; CPU/GPU temperatures remained unsupported. Diagnostic exit status 1 correctly disclosed partial provider errors while preserving usable JSON. Full tests/vet/build passed after retaining independent counter errors/results.

One-shot `Collect` primes the native CPU baseline with a context-aware 250ms wait, then queries providers once. Normal polling retains an explicitly unavailable first CPU sample until its next baseline delta. Collection has an eight-second total deadline, kills its own subprocess job on cancellation, and bounds pipe cleanup to 200ms; this is not a measured installed-service stop guarantee.

### Requested cputemp adaptation - 2026-11-12

The user requested copying [northwindlight/cputemp](https://github.com/northwindlight/cputemp), testing it first, then investigating a genuine CPU sensor if unavailable or incorrect.

Adapted its Windows ACPI query and Kelvin conversion from revision `f88dd585ba876689c9fd3364b84447669a10a5c4` into `internal/hardware/acpi.go` and the existing bounded PowerShell provider. No dependency or `go.mod` change was needed. Preserved the MIT copyright/permission notice in embedded `internal/web/static/cputemp-license.txt`, accessible from the dashboard's license link.

Unlike upstream, no first-zone-as-CPU assumption is made. Each zone retains its identity with kind `acpi`, a source label and explicit unverified CPU attribution. Verified CPU temperature remains unavailable without an identified CPU sensor. Raw values must be finite integral positive uint32 tenths of Kelvin; converted values outside -40 to 150 C are rejected with a reason. Supported negative Celsius values are not mistaken for invalid electrical readings.

Focused hardware/web tests, vet and candidate build passed. Tests cover conversion, missing/invalid values, multiple zones, CPU attribution and failed-source isolation. Live diagnostic testing returned `Not supported`; see [E-005](E-005-acpi-temperature-unsupported.md). The adapted source is implemented but does not produce a temperature on this PC.

Follow-up investigation found no existing supported sensor application/WMI namespace or PawnIO registration. LibreHardwareMonitor's Intel/Haswell MSR implementation is a genuine-sensor candidate, not yet installed or verified. Any privileged helper/driver setup requires separate approval. No driver, active configuration or running monitor was changed.

Final environment check: the previous dashboard at `127.0.0.1:18765` was unreachable, with no matching monitor/Air process found. Its stop cause/time is unknown; this task did not stop or restart it. Candidate tests/build and the PC-only diagnostic do not require that dashboard process.

### Requested killghost/coretemp assessment - 2026-11-12

Inspected the user-provided [coretemp.c](https://github.com/killghost/coretemp/blob/master/coretemp.c) and repository README. This is a GPL-2.0 Linux kernel hwmon driver, not a Windows library or the Windows Core Temp application's SDK.

It reads genuine Intel digital temperature sensors through kernel `rdmsr_on_cpu`/`rdmsr_safe_on_cpu`, checks the thermal-status valid bit and derives temperature from TjMax minus the reported distance. Its Linux kernel, hwmon/sysfs, CPU-affinity and MSR APIs cannot be compiled/loaded directly into this Windows Go application.

Assessment: relevant hardware approach, incompatible implementation. Copying its calculation alone would not grant Windows access to the sensor registers, and copying GPL code would require the corresponding license compliance. No source was copied, dependency added, driver installed or temperature-reading success claimed. Compatibility was determined from source; no Linux-module execution or Go validation was performed for this documentation-only assessment.

Next remains a Windows-compatible, explicitly approved privileged sensor provider/helper, with actual CPU package/core readings verified before integration. The previously investigated LibreHardwareMonitor/PawnIO route is a candidate; this Linux driver does not remove that requirement.

### Requested jaypipes/ghw assessment - 2026-11-12

Reviewed [ghw's README](https://github.com/jaypipes/ghw#inspecting--monitoring), [Windows CPU implementation](https://github.com/jaypipes/ghw/blob/main/pkg/cpu/cpu_windows.go) and exported CPU types. Unlike the preceding Linux driver, ghw supports Windows, but it is a hardware inventory library rather than a temperature/usage monitor.

Its Windows CPU query reads only manufacturer, model name, logical processor count and core count from `Win32_Processor`; its CPU types expose no temperature reading. The README explicitly distinguishes hardware inspection from monitoring changing metrics. It may help future inventory work, but does not solve this PC's missing CPU-temperature source.

Source-only assessment: no library installation, dependency/application change or runtime test was necessary. Do not add ghw solely for CPU temperature; the genuine-sensor provider requirement remains unchanged.
