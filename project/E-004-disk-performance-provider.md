# E-004 - Physical disk performance source unavailable

Updated: 2026-11-12 | Priority: P2 | Status: Open provider observation; cause unconfirmed

## Features and impact

Related: [F-016 hardware monitoring](F-016-pc-hardware-monitoring.md), [F-014 cached API](F-014-backend-api.md).

Mục tiêu: ghi rõ vì sao chưa đọc được tải và tốc độ ổ đĩa, không hiển thị số 0 như thể ổ đĩa đang không hoạt động.

Physical disk utilization and read/write rates are unavailable on this PC. Logical-volume free/used space, physical disk inventory, CPU and supported GPU counters still work. UPS monitoring is unaffected.

## Environment and reproduction

2026-11-12, Windows 10 amd64, existing non-elevated session. Separate candidate binary; running monitor/configuration unchanged.

```powershell
.\bin\upsdaemon.exe read-hardware
```

Expected: supported physical disk counters provide finite utilization/throughput; unsupported sources provide distinct reasons without discarding other measurements.

Actual final retest at 19:29 local time: idle/read/write each reported `0xc0000bb8 (PDH_CSTATUS_NO_OBJECT: performance object not found)`. Both physical disk records had null activity measurements with the corresponding reason. Diagnostic JSON remained available and exit status was 1 to expose partial provider failure.

## Evidence, cause and approach

- Initial CIM performance queries timed out; switching to native English-name PDH sampling recovered GPU engine/memory data but not physical disk activity.
- Confirmed meaning of the returned status: the requested performance object was not found. Why this Windows provider is unavailable remains unknown; disabled counters, provider registration and permissions have not been independently diagnosed.
- Reference: [Microsoft PDH error codes](https://learn.microsoft.com/en-us/windows/win32/perfctrs/pdh-error-codes).
- Fixed a related diagnostic issue: one failed counter no longer overwrites errors/results for the other counters. Successful independent readings remain usable.
- No counter rebuild, registry edit, disk-performance enablement, elevation or driver installation was attempted.

## Regression, assessment and next steps

Tests for a write-only failure and three distinct counter failures passed. Full tests/vet/build passed. Final live retest verified separate missing-object reasons, CPU utilization within 0-100%, supported GPU samples and reconciled volume free/used/total values.

Provider availability is not fixed or closed. Separately investigate Windows performance-provider availability before considering any approved system repair or alternative source. The separate PC temperature feature was removed by user request; it is no longer an outstanding hardware requirement.
