# F-017 - Telegram notifications

Updated: 2026-11-12 | Priority: P1 | Status: Event sender and authorized checking-only commands tested; no live delivery

## Current scope: checking only

The user selected a primary Fyne desktop, optional read-only web and Telegram checking/notifications. This supersedes the earlier shutdown-command section below: `/shutdown`, `/confirm` and `/cancel` no longer call any policy action and reply that controls are desktop-only.

Optional `telegram.allowed_user_ids` (1-16 positive unique sender IDs) enables outbound command polling independently of `power_control.manual`; numeric `chat_id` plus sender allowlist are required. Omission retains notification-only behavior. `/status`, `/ups` and `/battery` return cached UPS/policy information; `/hardware` returns bounded cached hardware information; `/help` explains this readonly boundary. Exact commands without `@botname` are supported.

Authentication, age/backlog/replay/forward/bot-source protections, fixed HTTPS routing, polling budgets and safe diagnostics remain. Tests passed for checking commands and explicit power-command rejection, unauthorized sources, pre-start backlog, duplicates, cancellation, redirects, malformed/oversized replies and rate limits. No live bot was connected.

## Purpose and user journey

Mục tiêu: nhận tin nhắn Telegram về trạng thái giám sát, mất điện và sự cố mà không cần mở dashboard; tránh gửi lặp hoặc lộ thông tin bí mật.

The user configures their Telegram bot and destination locally, enables selected notifications, and explicitly requests a test message. During monitoring, selected events are sent in the background. The dashboard shows delivery health separately from UPS health.

## Requested scope and proposed notifications

The user requested Telegram messages while monitoring. Exact triggers and cadence remain open; the proposed choices are:

- Monitoring started, with a clear distinction between application startup and the first valid UPS sample.
- Mains lost / UPS on battery, and mains restored.
- Low-battery or fault flag activated/cleared, only where the UPS reports a verified flag.
- UPS communication lost/stale, and communication recovered.
- Optional periodic status summary: UPS readings, freshness and power state; PC hardware readings only when [F-016](F-016-pc-hardware-monitoring.md) is available.
- Opt-in shutdown pending/cancelled/requested/failed events from [F-009](F-009-pc-shutdown.md). A request is not proof of completed shutdown.

Do not send every polling sample. Include event time, a user-chosen device label and relevant readings with units; mark stale/unavailable values honestly. Disk-health alerts require supported readings and separately approved thresholds. PC temperature monitoring/alerts are excluded by user request.

## Proposed implementation and security boundaries

- Proposed entry point: `internal/notifications`, integrated with live events from [F-007](F-007-event-history.md). Use a separate bounded worker/queue; never perform network I/O while holding state locks or polling USB.
- Use Telegram's HTTPS Bot API through a bounded HTTP client. Disable redirects so credential-bearing requests cannot be redirected to another host; preserve TLS certificate validation.
- Add opt-in validated settings through [F-008](F-008-configuration.md): enabled state, destination, selected event types, optional summary interval and a protected token reference. Defaults remain disabled.
- Choose secret storage compatible with both console and LocalService execution before implementation. Do not put bot tokens in source control, examples, browser APIs, diagnostic URLs or logs. Current deployment configuration is readable by local Users and is not suitable for storing the token itself.
- Enabled notifications transmit selected monitoring data to Telegram. The user subsequently selected authenticated shutdown commands over outbound bot polling, detailed below. No public webhook or LAN dashboard exposure is included.
- Deduplicate by live event identity, suppress unchanged repeated failures, and bound message size and send rate. Recovered history must not be automatically resent as new alerts.
- Retry transient network/server failures with bounded backoff; respect Telegram rate-limit guidance. Authentication/destination failures need explicit actionable diagnostics, not endless retries.
- Report queue overflow, exhausted retries and last successful API acceptance separately. API acceptance is not proof a person received/read the message; retries after an ambiguous timeout may produce duplicates.
- Internet access may disappear during a power outage. No guaranteed delivery, durable outbox or exactly-once claim is made. Expire old notifications rather than presenting delayed outage messages as current.
- Delivery failure must not block monitoring or delay shutdown. Do not recursively notify about notification failures; expose them locally. Normal stop may attempt a bounded drain but cannot promise a final message.

## Acceptance and planned tests

- TG-01: disabled notifications make no outbound requests; enabled notifications send only selected live event types.
- TG-02: repeated identical samples do not spam; loss/restoration and failure/recovery are distinct messages with correct timestamps and quality labels.
- TG-03: simulated rate limits, timeouts, authentication failures and queue overflow remain bounded and visible without delaying UPS polling.
- TG-04: token redaction covers logs, errors and APIs; a redirect cannot leak credentials. Service-context secret access is verified before claiming service support.
- TG-05: restart/history recovery does not replay old alerts; delayed messages expire and transient retries disclose duplicate-delivery limitations.
- TG-06: optional summary scheduling is bounded and does not fabricate unavailable UPS or PC sensor values.
- TG-07: after explicit user approval, a test message and selected simulated-event messages are accepted by Telegram and observed in the configured chat.

## Actual results, decisions and next steps

2026-11-12: documentation only. No bot credentials collected, provider requests made, messages sent, runtime configuration changed or notification tests run.

User confirmed the feature request, not the proposed trigger set, summary interval, retry/expiry budgets or credential-storage mechanism. Next: confirm event alerts versus periodic summaries and their cadence, select secure local setup, then implement with mocked delivery tests before an approved real-message test.

### Event sender candidate - 2026-11-12

The user requested continued coding of all workstreams. Implemented `internal/notifications` and optional root console/service wiring. An omitted `telegram` group disables delivery. A present group selects `token_env`, `chat_id` and explicit `events`; unknown/unsupported event names and inline tokens are rejected. Initial selectable events (extended by the shutdown candidate below): `startup`, `connected`, `connection_error`, `power_changed`, `low_battery_changed`, `fault_changed`.

The token is resolved from the process environment at startup, validated, and never returned through browser APIs or diagnostic errors. Environment storage is not encrypted, and a console variable is not automatically provisioned for LocalService. This is the initial explicit setup mechanism, not a verified Windows service secret deployment.

Delivery budgets: 64 queued events, three attempts per event, two-minute expiry, ten-second HTTP timeout, five-second dial/TLS timeout, at least one second between sends, 1s/2s retry backoff, 32 KiB response limit and 3,500-rune text limit. Rate-limit retry delays are honored only within expiry. Permanent authentication/destination failure blocks the sender until restart/reconfiguration, drops queued/new events visibly and makes no further requests.

Current messages contain event time/type/severity and safe event text; user-selected labels, full telemetry summaries, pure stale-age alerts, threshold alerts and a final stop message are not implemented. Periodic summaries remain pending an explicit cadence. Recovering history does not replay messages. Retries after an ambiguous response may duplicate a delivery; API acceptance is not a read receipt.

Tests using local HTTP mocks passed for API acceptance/rejection (including HTTP 200 with `ok:false`), authentication failures, rate limits, retries/expiry, queue overflow/deduplication, redaction, redirects and cancellation. The worker creates no request/retry goroutines and cancels active requests/timers on stop. Integration tests confirm missing tokens fail before application startup and initially asserted UPS low-battery/fault flags are not missed.

Assessment: event delivery is implemented and mock-tested, not Telegram/chat-verified. No real credentials were collected, outbound Telegram messages sent, active configuration edited or monitor restarted. Next: approved local credential setup, event selection and one deliberate live test; optional periodic summaries remain separate.

### Earlier Telegram shutdown commands - 2026-11-12 (superseded by current checking-only scope)

Người dùng chọn lệnh tắt Windows qua bot thay vì nút điều khiển trên web. Cần khớp cả chat ID và user ID đã cho phép; người khác trong cùng nhóm không tự động được cấp quyền.

Optional `power_control.manual` enables `internal/notifications/commands.go`; omission/disabled manual control starts no command poller. Requires an explicit numeric `telegram.chat_id` and 1-16 positive unique `allowed_user_ids`. The existing nonempty `telegram.events` selection is still required; command-only empty event lists are not supported. Bot token stays in the existing environment reference. See the dry-run fragment in [F-008](F-008-configuration.md).

| Command | Behavior |
| --- | --- |
| `/shutdown` | Show dry-run/live mode and configured countdown; issue a cryptographically random 64-bit, same-sender confirmation valid for 60 seconds |
| `/confirm CODE` | Consume a challenge in one attempt; schedule the configured manual countdown only on a correct, unexpired confirmation |
| `/cancel` | Invalidate all outstanding challenges; cancel a pending countdown, not an accepted Windows request |
| `/status` | Safe current policy mode/state/deadline/outcome |

Use exact commands; `/command@botname` syntax is deliberately not supported. Already pending or terminal policy attempts do not create misleading new confirmations. A failed/overflowed confirmation reply invalidates its challenge.

Security/lifecycle boundaries:

- Outbound HTTPS `getUpdates` only; no new listener, webhook registration or browser mutation route. Use a dedicated bot: another poller or an existing webhook causes visible blocked status; the app does not delete webhooks or take over other clients.
- Bootstrap with negative offset drops the pre-start backlog without executing it. Subsequent offsets suppress duplicate/older update IDs. Only fresh original text messages are considered: at most 60 seconds old, not before startup, and no more than five seconds in the future. Clock skew may safely reject commands.
- Match both configured chat and sender IDs. Reject bot, anonymous/sender-chat, forwarded and via-bot sources; ignore edits, channel posts and callback queries. Challenge contents/identities are memory-only and never enter browser status, event history or logs.
- Confirmation is bound to the same sender/chat and is single-use, including wrong-code attempts. Polling failures clear challenges. Restart/stop cannot reuse them.
- Polling uses five-second long polls, ten-second HTTP deadlines, 20 updates/request and a 256 KiB response bound. Redirects are refused, normal TLS verification remains enabled, and diagnostic errors omit raw URLs/bodies/credentials.
- Transient failures back off 1-30 seconds; rate-limit waits up to one hour are honored. Invalid/oversized rate-limit guidance blocks until restart rather than retrying too early. Authentication/setup/conflict errors block visibly. Immediate empty/duplicate replies are throttled to at most four polls/second.
- Replies use the existing bounded sender queue and one-second send spacing, with 30-second reply expiry. Policy never waits for delivery; internet or reply loss cannot promise a visible acknowledgement or stop an independently qualified automatic shutdown.
- `health.telegram_commands` reports `mode`, safe `error` and `rejected_commands` separately from event delivery. No token, configured chat/user IDs or confirmation code is exposed.

Added local-HTTP/fake-controller tests for same-sender authorization, forbidden message sources, confirmation expiry/consumption, global cancellation, reply overflow, startup backlog removal, duplicate updates, redirects, rate limits, oversized/malformed replies, safe errors and prompt stop. A confirmed command through the real dry-run policy never calls its injected OS adapter. Existing event sender regressions also pass; the affected suites passed five repeated runs.

Assessment: implementation is mock-tested only. No real token setup, polling, Telegram message, monitor restart, service secret provisioning or Windows shutdown occurred. Actual bot/chat/LocalService verification remains separate and requires approval.
