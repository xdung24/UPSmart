# E-002 - Process name collision

Updated: 2026-11-12 | Status: Fixed and live verified | Priority: P1

Features: [F-001](F-001-usb-communication.md), [F-006](F-006-windows-service.md).

## Impact and reproduction

Environment: Windows 10 Pro 19045 amd64; replacement named `upsmart.exe`.

With legacy UPSmart closed, run `bin\upsmart.exe read-status`. Initial process-name-only detection matched the replacement's own executable/helper and incorrectly reported that legacy UPSmart was running.

Expected: reject the actual vendor application, not the current replacement and its helper. Actual: status polling was blocked by a false positive.

## Cause and fix

Confirmed cause: case-insensitive executable-name matching cannot distinguish vendor `UPSmart.exe` from replacement `upsmart.exe`.

The guard now resolves each matching process's full executable path using limited process-query access and compares it with the current executable path. Same-image replacement processes are excluded. Other matching images remain blocked. An inaccessible or ambiguous image path is reported explicitly rather than assumed safe.

## Verification and assessment

2026-11-12: rebuilt binary successfully read the actual Q1 status and served repeated fresh samples while its own same-named process/helper existed. A second normal replacement instance was separately rejected by the named owner mutex.

Actual legacy-running rejection after this change and inaccessible-process-path behavior still need controlled regression checks; no legacy application was restarted automatically.

Persistence/service candidate regression: an installed service and a portable `upsmart.exe service start` client can be separate paths containing the same replacement image. The guard now additionally accepts byte-identical images via a bounded SHA-256 identity comparison. Tests passed for identical copies, different images and an inaccessible image. Different or unidentifiable matching processes remain blocked. This extension was built/tested only; no service/client live integration was performed.

Disposition: closed for the reproduced self-collision based on live retest. User approval covered read-only polling, not stopping/restarting arbitrary processes.
