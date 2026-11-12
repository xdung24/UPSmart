# F-010 - Delayed UPS power-off

Updated: 2026-11-12 | Priority: P2 | Status: Research gate; disabled

## Purpose and user journey

Mục tiêu: chỉ tắt đầu ra UPS sau khi máy tính đã tắt an toàn, không cắt điện sớm gây mất dữ liệu.

This feature remains unavailable until its exact device command, delay, cancellation and restart behavior are verified. Monitoring and PC shutdown must work without it.

## Implementation research gates

- Proposed entry points, only if approved: `internal/ups/control.go`, `internal/policy/ups_shutdown.go`.
- Establish exact AR2000 command semantics from legitimate documentation and controlled testing. Generic VID/PID support is insufficient.
- Verify supported delay range/resolution, acknowledgement meaning, behavior on mains return, pending-command cancellation, restart versus stay-off, and behavior after USB disconnect.
- A normal user-space service cannot confirm its own PC has fully powered off after it exits. A fixed delay alone cannot guarantee safe ordering if Windows stalls.
- Do not implement automatic output cut-off unless a supported lifecycle/handshake or independently verified safe sequencing mechanism satisfies the ordering requirement.
- If that mechanism does not exist, document the limitation and keep automatic UPS cut-off unavailable; do not silently downgrade safety to an assumed delay.
- Capability is disabled by default, separate from PC shutdown, and requires explicit model-specific enablement plus administrator approval.
- A future implementation must serialize control with polling, reject unsupported delays, record acknowledgements and uncertain outcomes, and never retry a potentially accepted destructive command blindly.
- Do not arm a UPS timer before requesting OS shutdown without a proven rollback sequence for OS rejection or cancellation.

## Acceptance and tests

- UPSOFF-01: without verified capability and explicit opt-in, no code path sends an output-changing command.
- UPSOFF-02: simulated acknowledgement loss produces an unknown outcome, not a duplicate send or a success claim.
- UPSOFF-03: OS shutdown rejection cannot leave an uncontrolled pending UPS cut-off.
- UPSOFF-04: a safe test setup verifies delay, cancellation, mains-return and restart behavior, including the stalled-OS case.
- UPSOFF-05: output cannot be removed before safe PC shutdown under the supported sequence; otherwise this feature fails its release gate.

## Decisions and boundaries

Stay-off versus automatic restart is unresolved. A live test affects every load connected to the UPS, not just this PC. Obtain explicit approval, identify all loads and use an isolated test load before any destructive experiment.

## Results and next steps

2026-11-12: no commands or delays have been verified; no power-control test occurred. Assessment: feasibility is unresolved, not promised. Next: protocol documentation research after [F-001](F-001-usb-communication.md), then decide whether safe sequencing is actually possible.
