# F-013 - Architecture and domain models

Updated: 2026-11-12 | Priority: P0 | Status: Go monitoring core implemented

## Purpose and user journey

Mục tiêu: tách giao tiếp phần cứng, dữ liệu, giao diện và quyết định tắt máy để dễ kiểm thử và không để lỗi USB làm treo toàn bộ ứng dụng.

The application continues serving honest health information even when the device is unavailable; the browser is never the automation engine.

## Proposed architecture

Go was confirmed by the user. `cmd/ups-daemon/main.go` and direct mode in `cmd/upsmart-desktop/main.go` call the shared `internal/monitor.Run` lifecycle, composing configuration, USB, state/events, web and optional hardware/Telegram workers. The opt-in shutdown candidate also composes `internal/policy` and the injectable `internal/platform` shutdown adapter. Earlier root-entry references below record the prior layout.

Data flow: serialized transport -> protocol parser -> validated sample -> state reducer -> immutable snapshot. The reducer emits bounded events and policy inputs. HTTP reads snapshots only. Only the policy controller can invoke an injected OS shutdown adapter.

## Domain contracts

- `DeviceIdentity`: VID/PID, product, manufacturer, optional serial, selected device path and verified capabilities. Device paths stay internal unless explicitly requested for diagnostics.
- `Measurement`: nullable numeric value, unit, quality and reason; no sentinel zero or NaN.
- `Sample`: process-local increasing sequence, UTC acquisition time, monotonic acquisition reference, measurements and independently supported status flags.
- `Snapshot`: schema version, service session ID, connection state, last attempt/success, stale/eligibility state, latest valid sample, active policy and health warnings.
- `PowerState`: mode, observed outage start and flags with known/unknown semantics.
- `PolicyState`: enablement, trigger, lifecycle state, deadline, blocking reason and OS request outcome.
- `Event`: schema version, session/sequence identity, UTC timestamp, type, severity and safe context.
- Typed errors distinguish device absence, ambiguity, access denial, timeout, malformed reply, unsupported capability, invalid configuration and persistence failure.

## Ownership and lifecycle

- One device owner; one state/policy controller; one serialized event writer; bounded HTTP workers/queues.
- Publish immutable copies under a small synchronization boundary. Never hold a snapshot lock while performing USB, disk or OS shutdown calls.
- Inject transport, clock, event sink and shutdown adapter for deterministic tests.
- Process restart invalidates all monotonic timing and starts policy qualification from fresh samples. Persisted events are not evidence of present power state.
- A bounded queue must declare overflow behavior: coalesce diagnostics, expose dropped-event counts, and never drop authoritative power/policy state transitions silently.
- Context cancellation coordinates lifecycle, but native I/O cancellation must be verified separately.

## Acceptance and tests

- ARCH-01: HTTP requests cannot invoke USB or shutdown adapters.
- ARCH-02: fake transport/clock/OS adapters exercise the complete monitoring-to-policy path without hardware.
- ARCH-03: concurrent reads, updates and stop do not race or deadlock; memory and queues remain bounded.
- ARCH-04: restart does not reuse stale telemetry or a historical countdown.

## Decisions, results and next steps

2026-11-12: the monitoring vertical slice is implemented without policy/OS-control adapters. Native descriptor/status access runs in a short-lived bounded child; the parent owns serial polling and a cross-process mutex. The HTTP server reads synchronized immutable snapshot copies and a 200-event memory history.

Tests passed for cached-data isolation, staleness boundaries, observed outage reset, event retention and reader cancellation. Actual helper polling and concurrent HTTP/browser reads worked. Race-detector startup is blocked by [E-003](E-003-race-runtime-startup.md), so no race-free claim is made.

Assessment: ARCH-01 holds for current HTTP routes; no shutdown adapter exists. ARCH-02's monitoring portion is tested, but policy execution is absent. ARCH-03 long-duration resource/race validation and full ARCH-04 restart tests are pending. Next: persist history and introduce service lifecycle only as needed.

Candidate update: the journal owns a bounded asynchronous writer, while the state store owns bounded recent history. Enqueue/status operations perform no disk I/O under the snapshot lock. A recovered event's original session/sequence identity is preserved; the new process starts with unknown power and no telemetry regardless of historical events. Restart-isolation and cross-session cursor tests passed.

Console signal handling and SCM control now share a context-driven application function. Readiness is signaled only after binding and starting the application, not merely on entry into the dispatcher. SCM handler tests verify readiness/failure and bounded startup/stop behavior. Actual SCM and concurrent long-duration/race checks remain pending; this candidate was not deployed.

### Optional workers - 2026-11-12

Root composition now also supports `internal/hardware` and `internal/notifications`. PC sampling and Telegram delivery run independently of USB polling and share console/SCM cancellation. Neither browser requests nor notification submission initiate a hardware/network operation under the state mutex.

The hardware cache serializes a typed sample at publication, then returns independent copies of its JSON payload and timestamp. Hardware availability/freshness are separate from UPS readiness. Live events enter a nonblocking notification sink; recovered history is not submitted. Constructor injection includes the new startup event without a replay window.

State tests cover live-only event submission, initially asserted critical UPS flags, unchanged-state suppression, independent hardware cache copies and its exact stale boundary. No shutdown adapters were added in that earlier optional-worker milestone.

### Shutdown controller ownership - 2026-11-12

The current candidate adds one serialized policy controller, an optional 250 ms evaluation loop and optional outbound Telegram command polling. Only the controller invokes its injected OS request; sender submission, browser reads and bot confirmation creation never invoke it. A correct confirmation schedules a countdown rather than directly requesting Windows shutdown.

Lock order: controller -> short state input/publication operations; state never calls back into controller. `Store.PolicyInput` reads only verified freshness/power continuity, and `Store.UpdatePolicy` publishes copied status/events. Command health locks are not held while calling policy methods. USB, bot requests and Windows utility execution occur outside the snapshot mutex.

Workers share console/SCM cancellation. The OS subprocess has a five-second timeout; polling/replies are bounded and cancel with the application. No hot reload, persisted countdown/challenge, background process outside the application lifecycle or extra HTTP listener is added.

Telemetry-to-policy-to-API and confirmed-command-to-dry-run tests pass with fake OS adapters. Stale-gap identity reset prevents inferred continuity after process suspension/missed ticks. Terminal outcomes stay latched for the controller lifetime; restart requires fresh qualification. Full race/LocalService/resource-duration verification remains pending, not inferred from passing unit tests.

### Native desktop separation - 2026-11-12

[F-018](F-018-native-desktop.md) adds `cmd/upsmart-desktop`, `internal/desktop` and `internal/control`. Widgets read cached typed snapshots/events/hardware and submit confirmed policy operations through a protected Windows named pipe. With a daemon, desktop Quit leaves monitoring alone. The later dual-mode extension below supersedes service-client-only operation.

The backend owns one optional pipe HTTP-framing server with account ACLs and no TCP control routes. Optional web and independent Telegram workers are read-only. Bridge requests serialize policy edits/confirmation checks; controller state/dispatch remains independently serialized. Disk persistence occurs before activation, outside the snapshot lock. Cancelled queued requests do not execute; interrupted mutating requests disclose unknown acceptance rather than claiming no side effect.

The disabled-policy evaluation worker still runs when desktop IPC exists so an explicitly saved policy can be enabled without restarting monitoring. Effective persisted policy loading is shared with offline configuration validation. Backend remains buildable without CGO; only the separate Fyne GUI requires the compiler/graphics stack.

### Dual-mode ownership - 2026-11-12

The user selected daemon-first startup with direct USB fallback only when no daemon owns monitoring. `internal/monitor.Connect` probes the pipe, then starts the same `Run` engine inside the desktop if needed. The existing global ownership guard precedes all polling/listener startup; failed/inaccessible daemon IPC is never permission to bypass it. A second probe handles a daemon winning the startup race. A compatible daemon requires no direct configuration load or USB access.

`Session` owns cancellation/join only for a direct runtime. A direct runtime failure latches connection failure; existing desktop sessions never silently transfer ownership after disconnect. Native helper dispatch is shared by both executables before GUI construction, preserving process-isolated bounded HID calls. Window close hides; explicit Quit joins desktop-owned workers but never stops the service. No background child daemon is left behind.

Simulation tests, full tests/vet, both Windows builds, offline native-window startup and invalid-helper rejection passed. Live final-GUI USB/SCM handover remains pending; detailed evidence and defaults are in [F-018](F-018-native-desktop.md).
