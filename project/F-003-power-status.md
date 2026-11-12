# F-003 - Power status

Updated: 2026-11-12 | Priority: P0 | Status: State reducer implemented; live mains baseline only

## Purpose and user journey

Mục tiêu: phân biệt mất điện thật, UPS chạy pin và mất kết nối để tránh báo sai hoặc tắt máy nhầm.

The dashboard reports the latest verified mains/battery state and emits one event when that state changes.

## Implementation

- Current entry point: `internal/state/store.go`; keep state reduction together until policy complexity justifies separation.
- Model power mode separately from communication: `unknown`, `online`, `on_battery`; retain independently verified low-battery and fault flags.
- Decode only documented or hardware-verified bits. Unsupported flags remain unknown rather than false.
- Use a monotonic clock for outage duration and countdown calculations; wall-clock UTC timestamps are for display and event records.
- First valid sample establishes current state. If already on battery at startup, start the observed outage duration then; do not invent its historical start time.
- A failed poll never transitions to `on_battery` or `online`. Last known state may remain visible with stale labeling, but cannot masquerade as fresh.
- Consecutive valid same-state samples do not duplicate events. Mains restoration ends an observed outage and informs the shutdown controller.
- After a communication gap, mark continuity unknown and restart outage-delay qualification on the next verified on-battery sample; no shutdown based on assumed continuity.
- Low-battery/fault interpretation must be verified before it affects policy.

## Acceptance and tests

- PWR-01: fixtures produce correct online, on-battery, low-battery and fault states.
- PWR-02: online -> battery -> online emits ordered transitions without duplicates.
- PWR-03: disconnect and malformed samples produce unknown/stale communication, not a power-loss event.
- PWR-04: startup on battery and reconnection on battery use observed timing, not fabricated outage duration.
- PWR-05: wall-clock jumps do not shorten or extend monotonic outage timing.

## Decisions and boundaries

Depends on [telemetry](F-002-telemetry.md) and [freshness rules](F-005-connection-resilience.md). Any debounce added after hardware testing must document latency and policy impact. Do not infer low battery from voltage alone.

## Results and next steps

2026-11-12: reducer tests passed for startup unknown state, observed battery timing, communication-failure continuity reset, recovery, restoration and duplicate-transition suppression. The Q1 parser tests synthetic low-battery/fault bits. Actual baseline is online with both flags false, exposed in API/UI.

Assessment: observed timestamps and last-known-state handling are implemented. No mains interruption or real low-battery/fault condition was induced. Monotonic freshness is implemented, but no outage-duration/countdown policy exists yet; PWR-05's policy timing proof remains pending. Next: separately approved mains-loss/restoration validation after non-destructive USB reconnect tests.
