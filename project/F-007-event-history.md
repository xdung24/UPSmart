# F-007 - Event history

Updated: 2026-11-16 | Priority: P1 | Status: Persistence implemented/tested; not enabled in local configuration

## Purpose and user journey

Mục tiêu: xem lại thời điểm mất điện, kết nối lại và lý do quyết định tắt máy mà không để nhật ký chiếm đầy ổ đĩa.

The user views recent events in the native desktop and can inspect local rotated logs for diagnosis.

## Implementation

- Current entry points: `internal/events/journal.go` and `internal/state/store.go`.
- Append structured JSON Lines records with schema version, process-session ID, sequence, UTC time, severity, event type, safe message and structured context.
- Record startup/stop, power transitions, first connection failure/recovery, relevant device faults, policy changes, shutdown pending/cancelled/requested/failed and log-storage failures.
- No high-frequency measurement history in initial scope. Keep a bounded in-memory ring for recent events and load a bounded tail on startup.
- Proposed limits: 5 MiB per file, 5 total files, latest 200 events in memory; no age-based retention initially.
- Store under the application's protected ProgramData directory. Do not log secrets, authentication material, arbitrary memory or unrelated device identities.
- Serialize writes and rotation. Handle partial last lines after interruption; report corruption explicitly without discarding entire valid history.
- Disk-full/access errors go to stderr in console mode or an available Windows event reporting path in service mode, and to an in-memory health warning.
- History failure does not fabricate successful audit persistence. Monitoring continues; expose which events could not be persisted.

## Acceptance and tests

- EVT-01: each state transition creates one ordered event, including its reason.
- EVT-02: rotation and restart respect exact configured size/count limits; bounded tail recovery handles a partial last line.
- EVT-03: full disk and access denied produce visible warnings without crashing polling.
- EVT-04: duplicate reconnect failures are rate-limited without hiding recovery.
- EVT-05: event API limits and pagination cannot read arbitrary files.

## Decisions and boundaries

Retention limits are proposals. Event history is diagnostic, not tamper-proof auditing. Related: [API](F-014-backend-api.md), [access boundaries](F-011-web-access-safety.md).

## Results and next steps

2026-11-12: `internal/state/store.go` implements the latest 200 events in memory with schema/session/sequence identity, UTC time, severity, type and message. Events cover startup, first/different connection failures, recovery, power transitions and changed low-battery/fault flags. No disk history or shutdown-policy events are implemented.

Tests passed for exactly 200 retained events, descending non-overlapping pages, invalid/expired cursors and duplicate-state suppression. The live API/UI showed startup, connected and online events. A fresh process starts a new session and history; the UI/API warning explicitly discloses memory-only operation.

The above describes the first memory-only milestone. It remains the behavior of an unchanged configuration that omits `event_history`.

### Persistence candidate - 2026-11-12

Implemented `internal/events/journal.go`. Event records now live in that package; state keeps the latest 200 records and non-blockingly submits new events to a 256-entry writer queue. A single worker appends JSONL, syncs each successful record and rotates `events.jsonl`, `events.1.jsonl`, etc. No disk I/O occurs while holding the snapshot lock.

- Optional configuration specifies directory, logical byte cap per file and total file count including the active file. Files never intentionally exceed the cap; an oversized single record fails explicitly.
- Default proposed service retention is 5 MiB x 5 files. Configured limits are 1 KiB-50 MiB per file and 1-10 files. Lowering file count prunes only recognized numbered archives beyond the limit; unrelated files are preserved.
- Startup scans bounded files chronologically, restores at most 200 valid records, reports malformed lines and truncates an incomplete active-file tail before appending. Oversized existing files fail initialization without deleting their evidence.
- Failed/partial writes, queue overflow and closed-journal submissions increment an unpersisted count and expose degraded storage health. Recovered writes clear the active error but retain the loss count. A subsequent write repairs a damaged tail after prior failure.
- Initialization failure is explicitly logged and exposed in status while monitoring continues memory-only in degraded mode until restart. This is not reported as successful persistence.
- Normal stop queues a stop record and drains the writer within the shared cleanup deadline. Timeout or lost records make flush return an error.
- Cursor identities use each record's session and sequence, allowing pages across recovered sessions. Events never restore current telemetry, outage timing or policy state.

Filesystem tests passed for exact 1024-byte rotation boundaries, total file/count limits, count reduction, unrelated-file preservation, rejection of archive-named directories without deleting them, corrupt/partial-tail recovery, explicit simulated file-open failure/recovery, retained loss counts and oversized-history preservation. Non-regular file/link targets are rejected. State tests passed for cross-session pagination and no telemetry resurrection. Event tests passed five consecutive runs.

Assessment: disk behavior is verified in temporary test directories, not an enabled live monitor or a full-volume test. The user explicitly declined configuration changes/restart, so no workspace/ProgramData event history was enabled. Next: approved real-process restart/persistence test and service-account storage verification.

### Live notification integration - 2026-11-12

An optional nonblocking sink now receives newly generated events, including startup; recovered records remain history-only and are not resent. Initial asserted low-battery/fault flags emit critical-state events even before a previous flag value exists. Initial false flags remain quiet, and unchanged samples do not generate duplicate alerts. Focused state tests and existing persistence/service regressions pass. See [F-017](F-017-telegram-notifications.md); no live messages or configuration changes were performed.
