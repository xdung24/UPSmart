# F-005 - Connection resilience

Updated: 2026-11-16 | Priority: P0 | Status: Bounded helper and polling implemented; live reconnect pending

Current startup observation: [E-008](E-008-q1-startup-control-response.md) records repeated `0x05` responses despite existing backoff and unsuccessful bounded transport experiments. One read recovered after deliberate vendor handover, not by a verified automatic recovery mechanism. No backoff/transport/policy behavior was changed; diagnostic-only regressions passed. Physical reconnect and independent initialization remain pending.

## Purpose and user journey

Mục tiêu: tự kết nối lại khi rút/cắm USB, đồng thời báo lỗi rõ ràng thay vì treo hoặc hiển thị dữ liệu cũ như mới.

The service remains responsive when the UPS disappears, shows the last known sample as stale and resumes after the verified device returns.

## Implementation

- Proposed entry points: `internal/ups/poller.go`, `internal/ups/transport_windows.go`.
- One serialized device owner and at most one exchange in flight. Never overlap polls, retry storms or control operations.
- Proposed initial values: poll every 2 seconds, transaction deadline 1 second, stale after 10 seconds, reconnect backoff 1/2/4/8/16/30 seconds capped at 30.
- Start the next attempt only after the previous operation is terminated or its handle is safely retired. Cancellation of a Go context alone does not cancel a blocking Windows syscall.
- Prove bounded cancellation for the chosen Windows I/O path. If indexed-string/control calls cannot be cancelled reliably, use a supervised transport helper process with a bounded lifetime rather than leaking blocked goroutines. Decide only after transport investigation.
- Explicit states: discovering, connected, retrying, disconnected and stopping; separately derive sample freshness.
- No successful sample means stale/unavailable immediately. Any failed exchange makes telemetry ineligible for shutdown decisions until fresh verified communication resumes, even before the display stale threshold.
- On recovery, validate device identity and a full response before publishing connected state. Never reuse partial response bytes.
- Coalesce repeated identical errors into periodic summaries; preserve first failure and recovery events.

## Acceptance and tests

- RES-01: unplug/replug recovers without process restart and without unbounded handle/goroutine growth.
- RES-02: timeout and permanently blocked transport do not freeze the desktop UI or service stop.
- RES-03: bounded fake-clock tests verify backoff, single-flight operation and reset after success.
- RES-04: stale data remains visibly stale and cannot trigger shutdown.
- RES-05: service stop interrupts retry waiting and terminates device work within the documented stop budget.

## Decisions and boundaries

Timing values are proposed defaults requiring hardware validation. Lost communication deliberately suspends automation, trading battery exhaustion risk for protection against false shutdown; disclose this limitation. Related: [F-003](F-003-power-status.md), [F-009](F-009-pc-shutdown.md).

## Results and next steps

2026-11-12: implemented `internal/ups/worker.go` and `internal/state/poller.go`. Each exchange runs in the same executable's isolated HID-helper mode. The parent applies a deadline, terminates a blocked child and waits for it before any retry, avoiding unbounded blocked native-call goroutines. Helper output is capped at 32 KiB.

Tests passed: a deliberately blocked helper was terminated after a 100 ms deadline within the test's 3-second upper bound; helper errors/oversized output were rejected; poller cancellation released its in-flight fake reader; freshness changed exactly at 10 seconds. Failed exchanges preserve last-known readings but mark communication retrying and keep policy eligibility false.

The poller uses one-second exponential reconnect backoff capped at 30 seconds; successful polls use the configured pause after the previous completed exchange. Duplicate errors are logged initially and at most every 30 seconds unless the message changes.

Live polling advanced sample sequence 8 -> 10 over a five-second API smoke check without overlapping device work. Assessment: process-level timeout containment is tested, but real stuck-driver termination, physical reconnect and long-duration resource stability are not established. Next: approved USB unplug/replug and bounded resource measurements. Race-detector execution is blocked by [E-003](E-003-race-runtime-startup.md).
