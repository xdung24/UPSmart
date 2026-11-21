# F-017 - Telegram notifications

Updated: 2026-11-22 | Priority: P1 | Status: Icon-enhanced notifications implemented; focused tests/vet passed; live message format retest pending

## Enhanced Telegram message format - 2026-11-22

Direct Telegram and rdpwatcher relay messages now share a compact visual format
without changing event selection, queueing, retries or safety bounds:

- `🔋 Mains power lost - running on battery`
- `⚡ Mains power restored`
- `✅ UPS connected`
- `⚠️ UPS connection problem`
- `🚨 UPS fault status changed`
- `🪫 UPS low-battery status changed`
- shutdown/policy messages use status-appropriate icons
- the desktop test uses `🧪 UPSmart test notification`

Each message includes a `🕒` UTC timestamp and the existing redacted event detail.
Relay mode still applies legacy Markdown escaping after formatting, so icons are
preserved while `_`, `*`, brackets and backticks remain safely escaped. Direct
mode remains plain text. The 3,500-rune bound is unchanged.

Checks on 2026-11-22:
`go test -count=1 .\internal\notifications .\internal\control .\internal\desktop .\internal\monitor .\internal\state`
and `go vet .\...` passed. The tests cover distinct power icons, generic fallback,
relay escaping, explicit test formatting and the existing delivery bounds. No
live Telegram message was sent in this change.

## Desktop test notification - 2026-11-21

The Setting tab now shows the active monitor's Telegram delivery status and a
**Test Telegram notification** button. The button is enabled only while the
backend reports an enabled or degraded sender; disabled, blocked and disconnected
states remain visibly unavailable.

The desktop sends one bounded `POST /telegram/test` request through its existing
protected local IPC. The monitor enqueues the test on the same serialized
Telegram/relay worker as live events, so it keeps the existing spacing, retry,
expiry and cancellation limits. The response reports that Telegram's API or the
configured relay accepted the notification; it does not claim chat receipt or a
human read. A failed/unknown request is shown explicitly and not converted into
successful-looking health.

Implementation checks on 2026-11-21: focused
`go test .\internal\notifications .\internal\control .\internal\desktop .\internal\monitor`
and `go vet .\...` passed. Coverage verifies exact desktop test-message content,
worker acceptance, local endpoint validation and disabled/failure/accepted
responses. `go test .\...` reached an unrelated existing failure in
`internal/ups`: the 25.55 V test expects 50.0% although the documented 24.0-27.0 V
linear mapping produces 51.666667%. No real relay, Telegram message, active
configuration change, monitor restart or live chat receipt was performed.

## Mains-loss notification live observation - 2026-11-21

The user received the manual test notification but received no alert after
disconnecting the wall socket from the UPS input. The desktop remained
`Mains online` with 228.0 V input and no `power_changed` log entry. This proved
the selected relay/destination worked for the manual action, but the monitor
never submitted an outage event for Telegram; it is tracked in
[E-011](E-011-mains-loss-not-detected.md).

To handle this UPS/driver behavior, Q1 remains authoritative. A frame with 0 V
input and positive output is treated as battery operation when the utility-fail
bit is not asserted. `GetSystemPowerStatus` is only a one-way fallback: Windows
AC-online never overrides Q1 because Windows can remain online while receiving
power from the UPS inverter; Windows AC-offline may only promote a stale Q1
online result. Live capture confirmed Q1 itself changes from
`10001000` at 0 V input to `00001000` at restored 227 V input.

Automated state, notification, desktop, monitor and control tests passed, as did
focused parser checks and `go vet .\...`. A live unplug test confirmed the outage
path. A live plug-in test remains pending for the new symmetric restoration
handling.

## Selected power and connection alerts - 2026-11-17

Người dùng chọn gửi thông báo khi mất điện, điện lưới trở lại và kết nối UPS phục hồi; xác nhận dùng relay rdpwatcher tại `127.0.0.1:50061`. Hai cấu hình cục bộ `config.yaml` và `bin/config.yaml` đã được bổ sung:

```yaml
notify-addr: 127.0.0.1:50061
telegram-events: [power_changed, connection_error, connected]
```

- `power_changed` reports `Power mode: on_battery` on mains loss and `Power mode: online` on restoration. The first valid sample also reports the initial power mode; it is not proof of a restoration.
- `connection_error` reports UPS read/communication failure; `connected` reports the first verified Q1 response and recovery after a failed read. A communication failure does not fabricate a mains outage. Identical repeated samples/failures are suppressed; changed error messages may produce additional failure events.
- No periodic summary, startup event or bot command polling is selected. Relay credentials and destination remain managed by rdpwatcher. Internet/relay loss during an outage can prevent delivery; the existing two-minute expiry applies.
- Both local files preserve monitoring/timing settings and leave shutdown disabled. These local files are ignored by Git; deployments must configure their own active YAML. A running daemon retains its existing configuration until restarted; an installed service may use a separate executable-adjacent copy. Desktop attached to a daemon uses that daemon's notifications, not the desktop's YAML.

Actual checks on 2026-11-17: `go test .\internal\state .\internal\notifications .\internal\config` passed (notifications/config cached). `go run .\cmd\upsdaemon --validate -c .\config.yaml` and the same command for `.\bin\config.yaml` passed with PC shutdown disabled.

Assessment: existing event handling required no code change; local selection is configured and offline-valid. No relay connection, live Telegram message, UPS interruption, service configuration change or process restart was performed. Next: restart the owning monitor with the selected configuration, then verify actual chat receipt under separately approved live checks. Do not equate configuration validation with active delivery.

## Optional rdpwatcher relay - 2026-11-16

Người dùng chọn thêm cách gửi thông báo qua API của rdpwatcher đang quản lý nhóm Telegram, thay vì để UPSmart tự giữ bot token và gửi trực tiếp. Cấu hình này dùng được cho daemon và desktop standalone vì cùng chạy shared monitoring engine.

Current flat configuration (selected later on 2026-11-16 to match rdpproxy):

```yaml
notify-addr: 127.0.0.1:50061
telegram-events: [startup, connected, connection_error, power_changed, low_battery_changed, fault_changed]
```

These also work as daemon CLI flags; repeat `--telegram-events` for multiple events. Current parameter/default/legacy compatibility contracts are in [F-008](F-008-configuration.md#current-rdpproxy-style-options---2026-11-16). Older nested examples/results below are historical or remain readable through legacy compatibility. The service now uses LocalSystem by explicit choice; earlier LocalService secret-provisioning notes are historical, not evidence that the new account was tested.

- Start/configure rdpwatcher separately with `--notify-listen 127.0.0.1:50061` (or its matching YAML option). Use its actual listener address if different. The relay owns the bot credentials and destination group; UPSmart sends only message text.
- `notify-addr` requires explicit `host:port` (IPv6 uses brackets), not a URL. It is mutually exclusive with `telegram-token-env`, `telegram-chat-id` and `telegram-allowed-user-ids`. Empty/omitted relay address retains direct-mode validation; omitting all notification parameters disables delivery.
- Notifications only: no UPSmart bot command poller is started in relay mode. This avoids competing with rdpwatcher's existing bot polling. Direct mode retains its optional checking commands.
- Existing rdpwatcher serves **plaintext, unauthenticated gRPC**. Prefer loopback; do not expose its listener to untrusted networks. Remote use requires a separately secured trusted network/tunnel; this client adds no TLS/authentication feature.
- `internal/notifications/relay.go` calls `/notify.Notifier/SendMessage` using the locally generated typed contract in `internal/notifications/relaypb/notify.proto`: request `message` field 1; response `sent` field 1 and `error` field 2. No dependency on a sibling checkout or changes to rdp-ex are required.
- rdpwatcher uses legacy Telegram Markdown, so UPSmart escapes formatting characters and bounds the escaped message to 3,500 runes. Direct mode remains plain text.
- Both transports share event selection, live-only deduplication, 64-item queue, expiry, send spacing and three-attempt maximum. Each relay call has a ten-second timeout, cancellation and a 32 KiB response bound. The connection is lazy and closes with the worker; an unavailable relay does not block UPS monitoring or cause fallback to direct Telegram.
- Selected transient gRPC errors retry within existing budgets. A `sent:false` or nonempty response error drops the event with visible degraded status, without retrying an unclassified upstream failure; later events may recover. Invalid service/authentication/setup errors block the sender until restart. Raw RPC/relay errors are never published because they may contain Telegram credentials.
- `last_success` means the relay reported `sent:true` without an error, **not independent Telegram acceptance or a read receipt**. The inspected rdpwatcher implementation checks Telegram HTTP status but does not inspect its JSON `ok` field; this upstream limitation is not fixed or hidden by UPSmart. Ambiguous retries can duplicate messages.

Acceptance TG-08: relay mode needs no bot credentials, sends the selected live message through the configured gRPC endpoint, rejects mixed direct/relay settings, handles negative acknowledgements explicitly, bounds retries/payloads and cancels promptly without affecting direct delivery.

2026-11-16 actual checks: full `go test .\...` and `go vet .\...` passed. `go test -count=5 .\internal\notifications .\internal\config` passed. Windows amd64 daemon (CGO disabled) and desktop (internal-linker Windows GUI build) compiled to temporary session outputs without replacing existing binaries. Loopback fake-gRPC tests cover exact payload/Markdown, event selection/deduplication, missing command support, negative/contradictory/empty responses, credential-safe errors, unavailable-service retry/recovery, wrong service, oversized replies, cancellation, expiry and the exact escaped text bound. Existing direct HTTP tests pass.

Assessment: configuration and relay behavior are mock-verified. No active local/example configuration was changed, monitor restarted, rdpwatcher contacted or real Telegram message sent. Next: configure the actual listener and event selection locally, restart the owning monitor, then obtain approval for one deliberate live-message test. Service-account network access and real Telegram receipt remain pending.

## Current scope: checking only

The user selected a primary Fyne desktop and Telegram checking/notifications, then removed web support on 2026-11-16. In direct mode, this supersedes the earlier shutdown-command section below: `/shutdown`, `/confirm` and `/cancel` no longer call any policy action and reply that controls are desktop-only. Relay mode above supports notifications only.

Optional `telegram-allowed-user-ids` (1-16 positive unique sender IDs) enables outbound command polling independently of `manual-shutdown`; numeric `telegram-chat-id` plus sender allowlist are required. Omission retains notification-only behavior. `/status`, `/ups` and `/battery` return cached UPS/policy information; `/hardware` returns bounded cached hardware information; `/help` explains this readonly boundary. Exact commands without `@botname` are supported.

Authentication, age/backlog/replay/forward/bot-source protections, fixed HTTPS routing, polling budgets and safe diagnostics remain. Tests passed for checking commands and explicit power-command rejection, unauthorized sources, pre-start backlog, duplicates, cancellation, redirects, malformed/oversized replies and rate limits. No live bot was connected.

## Purpose and user journey

Mục tiêu: nhận tin nhắn Telegram về trạng thái giám sát, mất điện và sự cố mà không cần mở desktop; tránh gửi lặp hoặc lộ thông tin bí mật.

The user configures their Telegram bot and destination locally, enables selected notifications, and explicitly requests a test message. During monitoring, selected events are sent in the background. The desktop shows delivery health separately from UPS health.

## Requested scope and proposed notifications

The user requested Telegram messages while monitoring. Power and connection alerts were selected on 2026-11-17 above; other triggers and summary cadence remain proposals:

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
- Enabled notifications transmit selected monitoring data to Telegram. The user subsequently selected authenticated shutdown commands over outbound bot polling, detailed below. No public webhook or LAN listener is included.
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
