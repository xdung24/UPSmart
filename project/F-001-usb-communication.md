# F-001 - USB communication

Updated: 2026-11-16 | Priority: P0 | Status: Q1 live verified; independent-startup observation open

## Current startup observation - 2026-11-16

[E-008](E-008-q1-startup-control-response.md) records repeated `0x05` status responses and one successful protected read after vendor UPSmart was opened and fully exited. The exact initialization requirement remains unknown; this does not establish independent cold-start operation. Buffer/startup experiments were not retained as unverified transport changes. Only bounded escaped failure diagnostics were delivered; targeted tests/vet and both Windows builds passed. Full evidence, user approvals and remaining verification are authoritative in E-008.

## Purpose and user journey

Mục tiêu: đọc trực tiếp UPS mà không phụ thuộc UPSmart và không vô tình thay đổi trạng thái nguồn.

The user stops UPSmart only when ready, runs a diagnostic connection check, selects the intended device if identity is ambiguous, and sees either validated status or an actionable connection error.

## Implementation

- Proposed entry points: `internal/ups/transport_windows.go`, `internal/ups/protocol.go`, `cmd/upsmart/main.go`.
- Enumerate through Windows SetupAPI/HID APIs. Start with VID `0001`, PID `0000`, product `MEC0003`; obtain manufacturer, serial if present, device path and report capabilities.
- VID/PID alone is insufficient identification. Reject ambiguous matches; never select the first arbitrary HID device.
- Determine whether this device uses indexed USB strings, feature reports or input/output reports. Compare the candidate transport with public protocol documentation before any query.
- Define a small transport contract: open the selected device, exchange an allowlisted status query with a deadline, close resources, and return typed transport/protocol errors.
- Keep protocol encoding separate from Windows handle management. No general-purpose raw-command API or HTTP command passthrough.
- Begin with confirmed read-only status commands only. Do not sweep indexes, enumerate unknown commands, toggle beepers or trigger tests to discover capabilities.
- Capture bounded diagnostic responses and timing locally. Do not record unrelated device identifiers or upload captures automatically.
- Prefer the existing HID driver; installing WinUSB/libusb drivers is outside the initial plan.
- Failed or timed-out I/O must release or safely terminate the operation before retrying; see [F-005](F-005-connection-resilience.md).

## Acceptance and tests

- USB-01: intended device is identified; missing and ambiguous devices produce distinct errors.
- USB-02: a verified status query returns repeatable, bounded responses without changing UPS state.
- USB-03: malformed, oversized and truncated replies are rejected without publishing a successful sample.
- USB-04: disconnect, access denied and contention with UPSmart are visible and do not crash the process.
- USB-05: monitoring transport cannot dispatch a power-changing operation.
- Unit tests use fake transport responses; hardware comparison runs only after a deliberate handover from UPSmart.

## Decisions and security boundaries

The exact transport and query set are unresolved. Do not treat generic NUT VID/PID support as a verified protocol. Concurrent polling is unsupported until proven safe. Power control is separately specified in [F-010](F-010-ups-power-off.md).

## Results and next steps

2026-11-12: current entry points are `internal/ups/hid_windows.go`, `internal/ups/worker.go`, `internal/ups/protocol.go` and root `main.go`. Discovery uses Configuration Manager HID interfaces rather than SetupAPI; both expose the same registered HID interfaces, and this avoids manual variable-size SetupAPI structures. Selection requires exactly one candidate and product `MEC0003`.

With explicit read-only approval, Windows HID indexed string 3 returned:

```text
(232.0 000.0 234.0 013 49.6 27.0 29.0 00001000\r
```

`\r` above denotes the actual terminal carriage return. No other indexed command is implemented. Standard product/manufacturer descriptor reads occur during discovery. Serial/report capabilities and broader device selection remain pending; advertised capability is only `read_status_q1`.

The device rejects its manufacturer descriptor with Windows error 87; this optional field is empty with an explicit warning. Other descriptor failures remain fatal. See fixed [E-001](E-001-manufacturer-descriptor.md). The legacy-process guard also distinguishes the replacement's own executable path; see fixed [E-002](E-002-process-name-collision.md).

Tests passed for malformed replies, bounded subprocess timeout, oversized helper output and explicit helper errors. Actual discovery/status and repeated serialized polling passed. Physical removal, ambiguous-device hardware and UPSmart comparison are pending.

Assessment: the read-only status path works on this AR2000; no power controls or generic transport fallback are implemented. Next: approved unplug/replug test and separate-session UPSmart comparison.

Persistence/service candidate: the legacy-process guard also recognizes byte-identical copies of the replacement, allowing a portable management command and an installed service image to coexist without misclassifying the management client as vendor UPSmart. Unknown/inaccessible or different images remain blocked. Regression tests passed for identical copies, different images and inaccessible images; no new live USB session was started in this build-only continuation.
