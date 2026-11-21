# E-011 - Mains loss not detected by Q1 status

Updated: 2026-11-22 | Linked features: [F-017](F-017-telegram-notifications.md), [F-018](F-018-native-desktop.md) | Status: Fixed and live-verified / P1

## Impact and environment

On the user's Windows desktop, no automatic Telegram alert was delivered when
the mains feed from the wall socket into the UPS was removed. The desktop
screenshot still showed 228.0 V AC input, 49.8 Hz and `Mains online`; the Log
tab contained no `power_changed` event.

The manual **Test Telegram notification** button succeeded through the active
relay, so relay connectivity and the destination were working for that action.

## Reproduction

1. Start the active UPS monitor and desktop with Telegram relay alerts enabled.
2. Confirm a manual test notification is received.
3. Disconnect the wall-socket cable feeding the UPS AC input.
4. Observe Data View, Log and Telegram delivery.

Expected:

- UPS state changes to on-battery.
- A live `power_changed` event is recorded and selected for Telegram.

Actual on 2026-11-21:

- The screenshot continued to report 228.0 V AC, 49.8 Hz and `Mains online`.
- No power-change log event or Telegram outage alert appeared.

## Cause

Confirmed cause: the monitor did not recognize a mains transition, so the
notification worker received no event to deliver.

Confirmed underlying cause: the replacement repeatedly opened a metadata-only
HID handle, so index 3 remained a cached mains-like descriptor until legacy
UPSmart maintained the required active session. Windows AC-online was not a
reliable alternate signal because the PC remained powered from the UPS
inverter. Reverse engineering verified UPSmart's persistent read/write,
overlapped HID session and Q1/Q1/Q1/F/BL/I read cycle.

## Fix approach

The UPS parser now also recognizes 0 V input with positive output as battery
operation when the Q1 utility-fail bit is not asserted. Native status capture
also records `GetSystemPowerStatus`, but Windows AC state is diagnostic-only
when it is online: Windows continues to report online while the PC is supplied
from the UPS inverter. It can only promote an otherwise-online Q1 result to
on-battery when Windows explicitly reports offline; it never overrides a
verified Q1 on-battery result. This prevents the independent PC signal from
masking the UPS's correct transition.

When this fallback changes the mode, the existing store emits the normal
`power_changed` event and the existing Telegram sender sends it if selected.

The monitor now also preserves the HID helper handle and read sequence across
polls, which refreshes index 3 independently of vendor software.

## Verification

2026-11-21 automated checks:

- `go test -count=1 .\internal\state .\internal\notifications .\internal\desktop .\internal\monitor .\internal\control` passed.
- Focused Q1 parser tests passed.
- `go vet .\...` passed.
- The existing unrelated 25.55 V capacity assertion still prevents an
  unfiltered `go test .\...` pass.

Earlier diagnostic captures before 17:06 on 2026-11-21 are invalid evidence and
are not used for closure.

Clean final-binary retest on 2026-11-22:

- Legacy UPSmart and the installed service were stopped.
- The final `upsdaemon-re-engineered.exe` started at 05:30 local time
  with a new journal and connected UPS; its first state was `online`.
- Wall input was removed. The fresh journal recorded `on_battery` at 05:31:42
  local time (`2026-11-21T22:31:42Z`), and the user confirmed Telegram receipt.
- Wall input was reconnected. The fresh journal recorded `online` at 05:33:30
  local time (`2026-11-21T22:33:30Z`), and the user confirmed Telegram receipt.
- No connection errors occurred during this clean outage/restoration test.

Disposition: fixed and verified. Install/reboot/long-duration soak remain
deployment follow-up, not blockers for this specific error closure.
