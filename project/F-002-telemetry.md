# F-002 - Telemetry

Updated: 2026-11-12 | Priority: P0 | Status: Six readings live; capacity unavailable

## Purpose and user journey

Mục tiêu: hiển thị số đo rõ ràng, đúng đơn vị và không nhầm dữ liệu ước tính hoặc cũ với giá trị thực tế.

The user views readings with units, freshness and source. Unsupported measurements display "Unavailable", not zero.

## Implementation

- Proposed entry points: `internal/ups/parser.go`, `internal/state/snapshot.go`.
- Model input voltage and output voltage in V AC; input frequency in Hz; load in percent; battery voltage in V DC; temperature in degrees Celsius; capacity in percent.
- Each measurement has a nullable value, unit, quality (`reported`, `estimated`, `unavailable`, `invalid`) and optional reason. Snapshot freshness is separate from measurement quality.
- Preserve device-reported precision. Reject non-finite numbers, invalid framing, invalid status bits and percentages outside 0-100. Do not clamp bad data into apparently valid readings.
- Establish model-specific ranges and field meanings from evidence. Do not assume battery voltage refers to one battery rather than the whole pack.
- Label temperature as UPS temperature until evidence establishes a battery sensor; do not silently relabel it.
- Initially leave capacity unavailable unless the protocol reports it. Any later estimate needs a documented model, assumptions and validation; no voltage-to-percent shortcut.
- Publish one coherent snapshot per parsed response. Do not combine old and new fields without their individual acquisition timestamps.
- Invalid replies are excluded from policy evaluation and logged with bounded diagnostic context.

## Acceptance and tests

- TEL-01: all seven requested measurements have explicit units and availability.
- TEL-02: valid captured fixtures decode to independently recorded expected values.
- TEL-03: malformed framing, missing fields, invalid numbers and out-of-range percentages cannot become valid samples.
- TEL-04: unsupported capacity and unknown sensor location are labeled honestly.
- TEL-05: comparison with UPSmart records timestamps, display rounding and differences without assuming UPSmart is ground truth.

## Decisions and boundaries

Depends on [F-001](F-001-usb-communication.md). Protocol fields, sensor location and capacity source are unresolved. Battery estimates must not trigger shutdown by default; see [F-009](F-009-pc-shutdown.md).

## Results and next steps

2026-11-12: implemented in `internal/ups/protocol.go` and `internal/state/store.go`. Live capture decoded to input 232 V AC, output 234 V AC, load 13%, frequency 49.6 Hz, battery 27 V DC and temperature 29 C. Battery capacity is null/unavailable because Q1 does not report it. No battery-percentage approximation was added.

`TestCapturedAR2000Status` preserves the actual baseline reply. Parser tests passed for field mapping, synthetic flags, missing/truncated frames, invalid decimal syntax, NaN/infinity, negative electrical values, load above 100%, bad bits and trailing junk. State tests verify that modifying a returned snapshot does not mutate cached readings.

Assessment: TEL-01/TEL-03/TEL-04 pass for the implemented Q1 scope; captured-field decoding is tested. TEL-02's independently recorded expected values and TEL-05's UPSmart comparison remain pending. Sensor location and battery-pack interpretation remain unverified. Next: compare stable measurements in separate sessions before claiming measurement accuracy.
