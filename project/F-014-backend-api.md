# F-014 - Backend API and infrastructure

Updated: 2026-11-12 | Priority: P0 | Status: Monitoring API implemented/tested

## Purpose and user journey

Mục tiêu: cung cấp dữ liệu nhất quán cho dashboard mà không cho trình duyệt truy cập trực tiếp USB hoặc quyền tắt nguồn.

The dashboard reads a cached snapshot and recent events. It continues to explain service health when hardware is disconnected.

## Proposed API

Proposed entry points: `internal/web/server.go`, `internal/web/handlers.go`, `internal/web/dto.go`.

| Method/path | Contract |
| --- | --- |
| `GET /` | Embedded dashboard; no external resources |
| `GET /api/v1/status` | Versioned snapshot with device summary, connection, freshness, measurements, power, active policy and health warnings |
| `GET /api/v1/hardware` | Independently cached PC metrics with disabled/waiting/enabled/degraded mode, sampling time, stale flag, safe error and nullable data |
| `GET /api/v1/events?limit=50&before=CURSOR` | Newest-first bounded events and an opaque next cursor; default 50, maximum 200 |
| `GET /healthz` | HTTP 200 when application loop is responsive; not a claim that UPS is connected |
| `GET /readyz` | HTTP 200 only with fresh verified telemetry; otherwise HTTP 503 with a reason |

`GET /api/v1/status` returns HTTP 200 for a readable but degraded/disconnected snapshot, not fabricated healthy values. An internal snapshot read failure returns HTTP 500. Unknown routes return 404; unsupported methods return 405.

### Status shape

Top-level keys: `schema_version`, `service_session_id`, `generated_at`, `device`, `connection`, `telemetry`, `power`, `policy`, `health`.

- `device`: null before identification; otherwise VID/PID, product and verified capabilities, excluding full device path by default.
- `connection`: state, last attempt/success timestamps and safe typed error or null.
- `telemetry`: null before a valid sample; otherwise sample sequence/time, stale flag and named measurement objects with `value`, `unit`, `quality`, `reason`.
- `power`: mode, known/unknown low-battery/fault states and observed outage timestamp or null.
- `policy`: enabled flags, state, trigger, deadline or null, blocking reason and outcome.
- `health`: explicit warnings, policy eligibility and `event_history` storage status; never equate HTTP availability with UPS health.

Use UTC RFC3339 timestamps; nullable fields serialize as null. Numbers must be finite. Do not return credentials, raw diagnostic buffers or stack traces.

### Event and error shape

Events response: `items` array of event objects defined in [F-013](F-013-architecture.md), plus nullable `next_cursor`. Cursors include session/sequence identity; malformed cursors return 400, expired retained history returns 410 with a reload instruction.

Errors: `{"error":{"code":"invalid_parameter","message":"Safe actionable explanation"}}`. No success-shaped fallback on validation failure.

## Runtime boundaries

- Standard-library HTTP server, embedded files and cached snapshots if Go is chosen.
- Proposed limits: read-header 5 seconds, read/write 10 seconds, idle 60 seconds, headers 16 KiB, at most 32 concurrent handlers; excess gets 503.
- Only GET and HEAD for read routes, with correct HEAD semantics. No request-body endpoints, CORS or websocket infrastructure initially.
- Follow [F-011](F-011-web-access-safety.md) for host/origin rules and response headers.
- Listener bind failure is fatal at startup; hardware failure is degraded readiness, not a reason to fabricate data.

## Acceptance and tests

- API-01: response-shape tests cover connected, unavailable, stale and policy-blocked snapshots.
- API-02: invalid limits/cursors, unknown routes, wrong methods and expired cursors return the specified status and error shape.
- API-03: readiness differs correctly from liveness; requests never cause an extra USB poll.
- API-04: concurrency limits/timeouts and graceful close are bounded; API encoding rejects invalid numeric data.

## Decisions, results and next steps

2026-11-12: routes and middleware are implemented together in `internal/web/server.go`; typed snapshots/events live in `internal/state/store.go`. Separate handler/DTO files were unnecessary for this small server. Disabled policy exposes null trigger/deadline/outcome; no mutation endpoints exist.

HTTP tests passed for dashboard/assets, unavailable status, readiness/liveness distinction, method/Host/Origin checks, headers, HEAD, invalid/repeated/unknown query parameters and missing routes. Store tests cover event pagination and expired cursors. JSON is encoded into a buffer before committing a success status.

Live smoke check on port 18765 returned health/readiness 200, connected fresh telemetry and increasing sequence 8 -> 10 over five seconds. The API included actual readings and manufacturer-descriptor warning; event history contained startup/connected/online entries. It rejected a second normal process through the shared owner guard.

Assessment: read-only monitoring contract works. Timeout/admission stress, graceful HTTP stop and all future policy states remain pending. No claim of legacy API parity is made.

Candidate addition: `health.event_history` exposes `mode` (`memory_only`, `persistent`, `degraded`), optional error/recovery warning and `unpersisted_events`. Persistence failure is distinct from intentionally disabled disk history.

Event pagination now resolves a cursor's exact retained session/sequence pair, so pages can cross recovered process sessions. Malformed tokens remain 400 and no-longer-retained identities return 410. State tests verify cross-session non-overlap and initialization-failure health; existing HTTP boundary tests pass unchanged. No new routes or browser mutation privileges were added. Live candidate API/browser validation was not performed because the user chose build-only.

### Hardware/Telegram extension - 2026-11-12

Current scope update: PC hardware temperature monitoring was removed at the user's request. Hardware `data` contains CPU/GPU/disk metrics, availability and warnings, with no `temperatures` field. This intentional removal does not change UPS telemetry, including its separate `ups_temperature` measurement.

The new hardware route is read-only and uses the same Host/Origin, method, header and admission boundaries. Its wrapper exposes `mode`, `generated_at`, `sampled_at`, `stale`, optional `error`, and `data`. Disabled/waiting data is null. Partial provider failures produce degraded data/reasons rather than fabricated readings. Hardware becomes stale at twice its configured interval plus the 10-second collection budget.

`health.telegram` reports `mode`, optional `error`, optional `last_success` and `dropped_events`; bot tokens and destination configuration are never served. UPS `/readyz` semantics are unchanged by optional hardware or notification failures. Startup with an invalid/missing enabled Telegram token is an explicit setup failure, not a silently disabled sender.

### Opt-in shutdown policy API - 2026-11-12

Existing status `policy` now reports actual controller state: `enabled`, `automatic_enabled`, `manual_enabled`, `dry_run`, always-false `ups_power_off_enabled`, `state`, `blocking_reason`, nullable `trigger`/`deadline`/`outcome`. Effective settings are `outage_after`, `low_battery_enabled`, `automatic_countdown`, `manual_countdown` (empty duration strings omitted). No chat ID, sender allowlist, token or confirmation is returned.

`health.policy_eligible` means an armed/pending automatic policy currently has fresh connected on-battery evidence; manual control need not depend on USB. `health.telegram_commands` independently exposes `mode`, safe optional `error` and `rejected_commands`; omitted command functionality reports disabled.

No routes/methods changed. Status remains a cached read, not a policy tick or OS operation. Integration tests drove synthetic Q1 -> actual policy -> status API, verified the pending deadline/effective settings, restored mains at the deadline, then dispatched once to an injected fake on a new qualified outage. Browser POSTs remain 405 with enabled controls; unknown GET control routes remain 404. UPS readiness still depends only on verified UPS freshness.

Desktop addition: [F-018](F-018-native-desktop.md) uses separate account-restricted named-pipe transport for `/snapshot`, `/policy`, confirmed `/shutdown` and `/cancel`. These are **not** registered on the web server; optional web-off mode starts no TCP listener. Telegram power mutation commands are removed. Tests exercise actual named-pipe framing separately from unchanged web Host/Origin/GET-only protections.
