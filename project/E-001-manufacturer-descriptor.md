# E-001 - Optional manufacturer descriptor

Updated: 2026-11-12 | Status: Fixed and live verified | Priority: P1

Features: [F-001](F-001-usb-communication.md), [F-002](F-002-telemetry.md).

## Impact and reproduction

Environment: Windows 10 Pro 19045 amd64, Go 1.26.5, ARES AR2000 HID VID 0001/PID 0000, product MEC0003.

After the user closed legacy UPSmart, run `bin\upsmart.exe discover`. Product lookup succeeded, but discovery aborted with `HidD_GetManufacturerString: The parameter is incorrect.` (Windows error 87).

Expected: an optional unavailable manufacturer should not prevent a positively identified UPS from being monitored; its absence must be explicit. Actual initial behavior: all descriptor failures aborted discovery.

## Cause and fix

Confirmed application cause: mandatory handling of an optional manufacturer field. The device/driver reason for error 87 is unknown; do not claim firmware corruption or a missing descriptor as proven.

For manufacturer lookup only, error 87 now produces an empty manufacturer plus a structured descriptor warning. Other descriptor errors remain errors; product identity and attributes are still mandatory. The warning appears in discovery JSON, status health and UI.

## Verification and assessment

2026-11-12: rebuilt executable; live discovery passed with an explicit warning, read-status returned a valid Q1 reply, repeated live polling and browser rendering passed. UPSmart reading parity remains pending, unrelated to this fix.

Disposition: closed based on observed live verification. No user decision authorized ignoring arbitrary USB failures.
