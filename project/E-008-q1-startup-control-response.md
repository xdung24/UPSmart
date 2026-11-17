# E-008 - Q1 startup returns control byte 0x05

Updated: 2026-11-16 | Status: Open observation / P1; one post-vendor recovery read verified, independent startup unresolved

## Features and impact

Linked features: [F-001 USB](F-001-usb-communication.md), [F-002 telemetry](F-002-telemetry.md), [F-005 resilience](F-005-connection-resilience.md).

Khi khởi động daemon, UPS trả về một ký tự điều khiển thay vì bản tin trạng thái. Ứng dụng không có số đo hợp lệ và phải tiếp tục báo lỗi; không được diễn giải ký tự này thành điện áp hoặc trạng thái nguồn. Một lần đọc đã thành công sau khi người dùng mở rồi thoát hẳn phần mềm UPSmart gốc. Chưa xác minh được cơ chế khởi tạo độc lập, nên lỗi khởi động chưa được đóng.

## Environment and reproduction

- 2026-11-16, Windows amd64, ARES AR2000, HID VID `0001`, PID `0000`, product `MEC0003`.
- User started `bin\upsdaemon.exe` with monitoring enabled, desktop IPC absent and PC shutdown disabled.
- Observed log at 15:28:06: `UPS read failed: unsupported UPS status response: invalid Q1 frame`. User confirmed continuous repetition.
- Expected: a complete validated Q1 status sample without relying on vendor software.
- Actual: approved diagnostic status read returned exactly `"\u0005"` (one control character, `0x05`). Product discovery succeeded; the existing optional manufacturer-descriptor warning [E-001](E-001-manufacturer-descriptor.md) remained.

## Evidence and bounded diagnostics

The user explicitly stopped the daemon and approved each hardware diagnostic phase. Captures used exclusive `Global\UPSmart-Monitor` ownership, legacy-process checks and bounded helper lifetimes. No concurrent poller or power-changing command was started.

1. One fixed status-index-3 read confirmed `"\u0005"`.
2. Four serialized A/B reads compared the existing 512-byte USB string buffer with a temporary 102-byte candidate. All four returned `0x05`. The candidate change was reverted; production transport buffer/commands are unchanged.
3. User approved a bounded read-only startup experiment informed by the public [NUT Fabula/Hunnox implementation](https://github.com/networkupstools/nut/blob/master/drivers/nutdrv_qx.c). After normal discovery, explicit indexed reads were `0, 0, 1, 13, 3, 12, 13, 3, 12` (standard descriptors, ratings, status, vendor information). The two status reads still returned `0x05`; ratings/vendor descriptors responded, including firmware text `V3.8`. This limited experiment does not establish that every required device initialization step was performed. No other control indexes, output reports or power operations were used.
4. User reported live readings in vendor UPSmart and closed its window. The next attempted capture was correctly blocked before USB access because `UPSmart.exe` still ran in the tray.
5. After the user explicitly exited the vendor process, one approved status read succeeded:

   ```text
   (231.0 000.0 233.0 015 50.2 27.0 29.0 00001000\r
   ```

   The helper validated the frame and advertised `read_status_q1`. Decoded values: input 231 V AC, output 233 V AC, load 15%, frequency 50.2 Hz, battery 27.0 V DC and UPS temperature 29 C; online, no low-battery/fault flags. This is one successful sample, not sustained-polling or cold-start verification.

## Cause and disposition

Confirmed: the parser rejected non-Q1 device data correctly. YAML parsing is not the source of the invalid USB response. Buffer reduction alone did not recover status. A valid status response was available after the vendor-software handover.

Unknown: the meaning of this device's `0x05`, the exact initialization/communication sequence used by the vendor, whether the same behavior recurs after USB reconnect/reboot, and whether status continues refreshing independently. Missing startup initialization is a supported hypothesis, not a fully confirmed root cause. Do not label `0x05` as an ACK/NAK or map it to a UPS power state without evidence.

Temporary user-operated workaround: while this app is stopped, open vendor UPSmart and verify live readings, then **Exit/Quit it completely from the tray** before starting this daemon. Never run both pollers together. The workaround produced the single successful capture above; repeatability remains unverified. It is not an implemented automatic fix or a production-readiness claim.

## Delivered change and regression checks

- [ParseQ1](../internal/ups/protocol.go) now reports the actual response byte count and an escaped prefix of at most 32 bytes when the Q1 marker is missing; oversized replies report their length without echoing their body. This makes `0x05` distinguishable from ordinary malformed text while preventing log-control injection or unbounded output.
- No validation was relaxed, retries added, initialization enabled, telemetry fabricated or configuration changed.
- 2026-11-16: `go test .\internal\ups .\internal\state .\internal\monitor .\cmd\upsdaemon` and corresponding `go vet` passed. Tests cover empty/control-character/long/oversized invalid frames, bounded escaped diagnostics, existing captured Q1 decoding and monitoring regressions.
- `build.cmd` rebuilt `bin\upsdaemon.exe` and `bin\upsdesktop.exe` successfully with the diagnostic improvement. These rebuilt apps were not started for live monitoring. The successful hardware capture used the existing transport before this diagnostic-only rebuild.
- Temporary diagnostic source/executables were removed. The daemon and vendor app were not restarted automatically. No service action, Telegram delivery, real shutdown, mains change or USB replug was performed.

## Next steps and user decisions

The user approved only the bounded read-only diagnostic phases above and manually controlled vendor startup/exit. No unverified startup command or proprietary implementation was copied.

Keep E-008 open. Next requires a separately approved read-only investigation of the missing initialization/refresh behavior, with a reproducible cold-start baseline and sustained fresh-status verification. Do not add a guessed handshake, disable the legacy-process guard or accept `0x05` as a sample.
