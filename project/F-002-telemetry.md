# F-002 - Telemetry

Updated: 2026-11-17 | Priority: P0 | Status: Six reported readings; battery capacity estimated from user-provided pack endpoints; startup response observation open

Current startup limitation: [E-008](E-008-q1-startup-control-response.md) records non-Q1 control responses and one valid sample after vendor handover. Invalid responses remain excluded from telemetry/policy; no measurement or power state is fabricated. Parser failure messages now include bounded escaped response diagnostics, with targeted regression tests/vet and both builds passing. Independent cold-start and sustained freshness remain unverified.

## Purpose and user journey

Mục tiêu: hiển thị số đo rõ ràng, đúng đơn vị và không nhầm dữ liệu ước tính hoặc cũ với giá trị thực tế.

The user views readings with units, freshness and source. Unsupported measurements display "Unavailable", not zero.

## Implementation

- Entry points: `internal/ups/protocol.go`, `internal/state/store.go`, `internal/desktop/desktop.go`.
- Model input voltage and output voltage in V AC; input frequency in Hz; load in percent; battery voltage in V DC; temperature in degrees Celsius; capacity in percent.
- Each measurement has a nullable value, unit, quality (`reported`, `estimated`, `unavailable`, `invalid`) and optional reason. Snapshot freshness is separate from measurement quality.
- Preserve device-reported precision. Reject non-finite numbers, invalid framing, invalid status bits and invalid reported percentages. Do not clamp malformed raw measurements; only clamp the derived capacity estimate to its specified 0-100% endpoints.
- Establish model-specific ranges and field meanings from evidence. The user states the AR2000 pack uses two 12 V batteries; this pack interpretation has not been independently measured.
- Label temperature as UPS temperature until evidence establishes a battery sensor; do not silently relabel it.
- Q1 does not report capacity. For display only, linearly estimate percent from the user-provided whole-pack endpoints: 24.0 V = 0%, 27.0 V = 100%, clamped to 0-100%. Label the result estimated; do not present it as UPS-reported state of charge or use it for shutdown policy.
- If an older daemon omits capacity or marks it unavailable but reports battery voltage, the desktop applies the same shared estimator; invalid capacity data is not overridden, and missing/unreported voltage leaves capacity unavailable.
- Publish one coherent snapshot per parsed response. Do not combine old and new fields without their individual acquisition timestamps.
- Invalid replies are excluded from policy evaluation and logged with bounded diagnostic context.

## Acceptance and tests

- TEL-01: all seven requested measurements have explicit units and availability.
- TEL-02: valid captured fixtures decode to independently recorded expected values.
- TEL-03: malformed framing, missing fields, invalid numbers and out-of-range percentages cannot become valid samples.
- TEL-04: voltage-derived capacity is explicitly labeled estimated, uses the documented pack endpoints and never drives shutdown policy; unknown sensor location remains labeled honestly.
- TEL-05: comparison with UPSmart records timestamps, display rounding and differences without assuming UPSmart is ground truth.

## Decisions and boundaries

Depends on [F-001](F-001-usb-communication.md). Q1 still has no capacity field, so the displayed estimate is a user-calibrated voltage mapping, not a reported capacity measurement. Battery estimates must not trigger shutdown; see [F-009](F-009-pc-shutdown.md).

## Results and next steps

2026-11-12 baseline: implemented in `internal/ups/protocol.go` and `internal/state/store.go`. Live capture decoded to input 232 V AC, output 234 V AC, load 13%, frequency 49.6 Hz, battery 27 V DC and temperature 29 C. At that time capacity was null/unavailable because Q1 does not report it; no approximation had been added.

`TestCapturedAR2000Status` preserves the actual baseline reply. Parser tests passed for field mapping, synthetic flags, missing/truncated frames, invalid decimal syntax, NaN/infinity, negative electrical values, load above 100%, bad bits and trailing junk. State tests verify that modifying a returned snapshot does not mutate cached readings.

Historical assessment (2026-11-12): TEL-01/TEL-03/TEL-04 passed for the then-implemented Q1 scope; captured-field decoding was tested. TEL-02's independently recorded expected values and TEL-05's UPSmart comparison remained pending. Sensor location and battery-pack interpretation remained unverified at that time.

### User-provided battery-pack calibration - 2026-11-17

The user stated the AR2000 uses two 12 V batteries and specified 24.0 V as 0% and 27.0 V as 100%. `battery_capacity` is now calculated by linear interpolation between those endpoints and clamped to 0-100%; Q1's reported battery voltage is unchanged. The UI labels the card/value as estimated. This mapping is an explicit user-provided display assumption, not an independently validated state-of-charge curve; estimated capacity is not used by policy or shutdown decisions.

On 2026-11-17, full `go test .\...` passed. Parser regression cases cover both endpoints, a midpoint and clamping beyond the range. Desktop tests simulate an older-daemon payload with reported 27.0 V and unavailable capacity. An earlier 27.1 V full-pack endpoint displayed this as 96.8%; the current user-provided 27.0 V endpoint displays 100.0%. A read-only check found the connected, fresh daemon returned that payload; its running process predates the parser change. No daemon was restarted. Live battery discharge/charge calibration and comparison against UPSmart remain pending.
