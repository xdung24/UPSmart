# F-004 - Web dashboard

Updated: 2026-11-16 | Priority: P0 | Status: Removed at user request; historical verification retained

## Current disposition - 2026-11-16

The user requested: "Revemo the web support. I only need the desktop app." The embedded dashboard, TCP HTTP server, browser assets and browser-specific behavior have been removed. This feature is no longer planned or supported. The historical implementation and test results below document what existed before removal; they are not current deliverables. Desktop presentation is tracked in [F-018](F-018-native-desktop.md).

Removal validation on 2026-11-16: full Go tests and vet passed, and both Windows applications built successfully without a TCP web package. No browser or listener smoke test was run because the feature no longer exists.

## Purpose and user journey

Mục tiêu: mở trình duyệt để xem UPS ngay, không cần ứng dụng desktop cũ.

The user opens the local URL and sees connection health, mains state, readings, active shutdown policy and recent events.

## Implementation

- Implemented entry points: `internal/web/server.go`, `internal/web/static/index.html`, `internal/web/static/app.js`, `internal/web/static/style.css`.
- Embed assets in the binary; use plain HTML/CSS/JavaScript with no CDN or runtime internet dependency.
- Proposed URL: `http://127.0.0.1:8080`; bind only to loopback as specified in [F-011](F-011-web-access-safety.md).
- Poll the cached status API every two seconds while visible; this must not issue a USB query per browser request.
- Show measurement units, source/estimate labels, device identity, last successful update, stale/disconnected warnings and concise actionable errors.
- Display unavailable values as text, not zero. Do not retain a green "healthy" indicator when API requests fail.
- Show shutdown countdown from server policy state; the server remains authoritative if the browser closes.
- Initial dashboard is read-only. Settings are changed through local CLI configuration, not browser mutation endpoints.
- Use text rendering rather than HTML injection for device strings and errors. Provide keyboard access, readable contrast and non-color status labels.

## Acceptance and tests

- UI-01: all requested fields and unavailable/estimated states render correctly.
- UI-02: API outage, stale readings and USB disconnect are visibly different from healthy operation.
- UI-03: multiple browser tabs do not increase USB polling rate.
- UI-04: countdown matches server state; restored mains removes a cancelled pending countdown.
- UI-05: device/error strings containing HTML are displayed as text.
- UI-06: dashboard works without internet and in a narrow browser window.

## Decisions and boundaries

Depends on [F-014 API contracts](F-014-backend-api.md). UI language and final visual design are unresolved; initial labels may be English. No remote control buttons or assumed browser authority to shut down the PC.

## Results and next steps

2026-11-12: browser showed actual live readings, device warnings, online state and unavailable capacity. At 390 px viewport width, no horizontal overflow was detected. A browser-intercepted API failure changed status to a warning and invalidated old displayed measurements; removing interception recovered connected status. These browser failures were simulated, not USB disconnections.

The visible page explicitly states that power controls are absent and event history is memory-only. Countdown UI is not implemented. Other UI/HTTP tests are recorded in [F-014](F-014-backend-api.md).

Port 8080 was already occupied on this machine; binding failed explicitly without changing the existing listener. The local test configuration therefore uses `http://127.0.0.1:18765`. The example retains 8080; operators may choose another loopback port.

Assessment: live monitoring and API-error recovery work. Estimated measurements, actual USB-disconnect rendering, injection fixtures and countdown behavior remain to be verified/implemented. No network-disable test was performed, although all assets are embedded. Next: add service/persistence without introducing browser control endpoints.

### Persistence candidate - 2026-11-12

The history caption now derives from `health.event_history`: memory-only, persistent or degraded. It reports unpersisted-event counts and does not claim persistence just because logging was requested. The footer no longer describes persistence/service support as wholly absent; it still states that power controls are unavailable.

Existing HTTP tests pass with the embedded updated assets. Browser verification of this candidate is pending: the user selected build-only, so the existing monitor/page was not replaced or reloaded for new UI tests.

### Hardware/notification candidate - 2026-11-12

Added a separate PC hardware section backed by cached `/api/v1/hardware`, including device/sensor reasons, units, sampling time, degraded state and freshness. Its refresh is independent of the UPS API refresh. Telegram delivery mode/error and last API acceptance are read from `health.telegram`; this is not a read receipt.

JavaScript syntax and a scripted DOM smoke test passed for metric rendering, missing sensors, text-only untrusted names and disabled Telegram. HTTP tests cover the added route's loopback Host/Origin checks, read-only methods and body-free HEAD responses. This is not a live-browser check of the candidate; the running monitor was not replaced.

### PC policy and Telegram command visibility - 2026-11-12

The earlier "power controls absent" copy is superseded. The read-only dashboard now distinguishes disabled, DRY RUN and LIVE PC policy; shows enabled automatic triggers, effective automatic/manual countdown settings, pending deadline/remaining seconds, cancellation, outcome and blocking reason. Remaining time is explicitly as of the last server refresh, not a browser-controlled timer.

Telegram command-worker mode/error/rejection count is separate from notification-delivery health. Instructions list `/shutdown`, `/confirm CODE`, `/cancel` and `/status`; no browser buttons or mutation endpoints are added. UPS output control remains unavailable. Lost API access invalidates displayed policy status and warns that a countdown may still be active.

`node --test .\internal\web\app_test.cjs` uses a local scripted DOM and fake API responses. It passed disabled/dry-run/live/pending/cancelled/failed rendering, exact 30-second display, effective settings, text-only untrusted trigger and API-loss behavior. UPS temperature still renders independently. HTTP integration tests use a real policy with a fake OS callback and verify readonly boundaries while enabled. These are synthetic checks, not a live browser/deployment test.
