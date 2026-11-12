# F-015 - Quality and verification

Updated: 2026-11-12 | Priority: P0 | Status: Persistence/service candidate tests/vet/build pass; runtime deployment pending

## Purpose and user journey

Mục tiêu: chỉ công bố khả năng đã kiểm chứng, ưu tiên tránh tắt máy nhầm, mất dữ liệu hoặc cắt nguồn sớm.

The user can see which acceptance criteria passed in simulation, which passed on real hardware and which remain unsupported.

## Implementation and test approach

- Proposed Go test entry points: focused `*_test.go` files beside parser, state, policy, event, config and HTTP code; sanitized protocol fixtures near their consumers.
- Use deterministic fake clocks and adapters; never issue real OS/UPS shutdown from automated tests by default.
- Prioritize tests that fail for actual behavior defects: malformed replies, stale-state eligibility, restoration/countdown races, reconnect bounds, persistence failure and unsafe API reachability.
- Implement one vertical slice at a time: transport discovery -> parser -> state -> cached API -> dashboard -> service -> optional policy.
- Run the smallest relevant test packages first. If Go is chosen, use `go test` for affected packages, `go vet` for the implemented command/packages and a Windows amd64 build.
- Use race detection where the selected Windows toolchain supports it; record unavailable tooling explicitly rather than claiming a race-free build.
- Full-suite checks are appropriate at integrated milestone boundaries, not as a substitute for targeted regression tests.
- No dependency installation until manifests require it or an actual missing-tool/dependency failure warrants it.

## Hardware validation protocol

1. Record OS, UPS identity, selected protocol and test environment; save work and identify every connected load.
2. Obtain approval for handing over polling from UPSmart. Verify readings under stable mains before fault tests.
3. Test USB disconnect/reconnect independently of mains loss; neither event should be misclassified as the other.
4. Only with explicit approval, test mains loss/restoration while monitoring and output controls are disabled.
5. Verify policy dry-run with simulated events, then separately request approval for a real PC shutdown test.
6. UPS cut-off testing is excluded until [F-010](F-010-ups-power-off.md) passes its research and safety gates.

## Acceptance and evidence

- QA-01: each feature records dated checks, actual outcomes, limitations and a post-test assessment.
- QA-02: mock-only behavior is never described as hardware verified.
- QA-03: a confirmed regression has an E-ID record, reproduction, linked feature and verification before closure.
- QA-04: tests do not activate destructive adapters accidentally; real power tests require a separate explicit opt-in.
- QA-05: documentation links and PLAN feature IDs remain consistent.
- Keep feature-specific test evidence in that F file; track errors/observations in [TEST](TEST.md), not a duplicate global result history.
- Pending tests and missing implementation are not bugs. Do not invent E records to represent planned work.

## Results and assessment

2026-11-12: documentation validation passed: 18 Markdown files, 50 resolving local links, exactly one detail file and PLAN row for each ID F-000 through F-015, and no documentation subdirectories. QA-05 passed for this documentation change.

Later on 2026-11-12, the monitoring milestone was implemented and checked with Go 1.26.5, Windows 10 Pro 10.0.19045 amd64:

```powershell
go test .\internal\config .\internal\ups .\internal\state .\internal\web
go vet .\cmd\upsmart .\internal\...
go build -o .\bin\upsmart.exe .\cmd\upsmart
```

All three commands passed. Covered behavior includes strict synthetic/captured Q1 parsing, bad config rejection, isolated-helper termination/output limits, immutable snapshots, exact staleness boundary, observed-outage reset, duplicate-event suppression, 200-event retention/pagination, poll cancellation and HTTP boundaries.

Actual read-only status and advancing live API samples passed after user-approved handover from UPSmart. Browser validation confirmed real readings, a 390 px layout without horizontal overflow, simulated API failure warning/invalidation and subsequent recovery. See feature-local evidence for details.

Final built-binary browser retest passed for connected status, low-battery/fault labels, verified `read_status_q1` capability, unavailable capacity and disabled/null power policy. Updated documentation checks passed: 21 Markdown files, 66 resolving local links and intact F-000 through F-015 coverage. Temporary monitoring was stopped; a subsequent OS check confirmed no project executable/helper or listener on port 18765 remained.

`go test -race .\internal\state .\internal\web` could not execute: both binaries exited at startup with status `0xc0000139`. This is a blocked tooling observation, not an observed Go data race; cause remains unknown in [E-003](E-003-race-runtime-startup.md).

No service installation, physical USB reconnect, mains-loss, real shutdown, UPS cut-off, long-duration resource test or UPSmart reading comparison has been performed. No full production-readiness claim is made.

Assessment: Go and the read-only status transport are established. The console milestone is usable, but measurement parity, unattended lifecycle and any shutdown behavior require additional evidence. A successful build alone cannot establish those properties.

## Progress and next steps

### Persistence/service candidate checks - 2026-11-12

After the user approved the changed root-level layout, these commands passed:

```powershell
go test .\...
go test -count=5 .\internal\events .\internal\service
go vet . .\internal\...
go build -o .\bin\upsmart-next.exe .
```

New checks cover exact byte/file retention thresholds, reduced retention and unrelated-file preservation, malformed/incomplete history recovery, simulated write failure/recovery with retained loss evidence, oversized-history preservation, cross-session pagination without current-state resurrection, history configuration rejection, SCM handler readiness/interrogate/cancellation/failures/time budgets, SCM-only command guard, rejection of unprotected deployment output and recognition of identical portable/installed process images.

Tests use temporary filesystem directories and fake service applications; they do not register a service, shut down the PC/UPS, edit local configuration or replace a monitor. The user explicitly selected build-only. The ordinary full suite passed; the previously blocked race detector was not rerun and remains unverified.

Final candidate rebuild and repeated checks passed after adding rejection of non-regular journal targets. Documentation verification passed for 21 Markdown files, 67 local links and complete F-000 through F-015 coverage. Candidate output is `bin/upsmart-next.exe` (10,249,216 bytes); the original executable was not replaced.

Not executed: real candidate browser/API restart, full-volume exhaustion, actual Administrator deployment/ACL enforcement, LocalService HID access, SCM install/start/stop/uninstall/crash recovery, physical USB/mains tests or shutdown control. Keep feature-local acceptance limitations open.

Next: separately approved persistence/restart and elevated service integration tests, plus USB reconnect/UPSmart comparison. No automatic shutdown protection is implemented.

### Default startup change - 2026-11-12

User chose no-argument monitoring startup. `go test .`, `go vet .`, root candidate build, `upsmart-next.exe --help` and `upsmart-next.exe serve --help` passed. Four focused CLI tests verify valid default dispatch through an injected monitor, missing/invalid local-file errors and preserved explicit configuration selection. Actual helper-free CLI help exits zero without timestamped error logging.

This validation does not start a live monitor or alter local configuration. Candidate runtime/startup integration under Air remains untested; its existing no-argument configuration now selects the documented default behavior. Feature acceptance and usage are recorded in [F-012](F-012-delivery-verification.md).

### Optional monitoring/Telegram candidate - 2026-11-12

- Full `go test .\...`, `go vet .\...` and separate Windows candidate build passed after the final hardware-source changes.
- Event/service tests passed five repeated runs. Notification/state/web tests also passed five repeated runs, including cancellation, bounded queues, snapshot isolation and HTTP access boundaries.
- JavaScript syntax and scripted DOM smoke checks passed for actual nested volume/physical-disk data, nullable measurements, text-only untrusted names and Telegram dropped-event visibility. Project documentation links were checked; live browser verification of the new candidate remains pending.
- Read-only PC diagnostics verified native CPU utilization, supported GPU usage/memory, disk inventory and volume capacities without touching USB. Missing physical-disk counters and temperature access are explicitly partial, not a fully passing hardware integration; [F-016](F-016-pc-hardware-monitoring.md) and [E-004](E-004-disk-performance-provider.md) contain evidence.
- Telegram tests used local mocks only; no real messages or credentials. Existing monitor remains on loopback `127.0.0.1:18765`; no active config changes, restart, service installation or power action occurred.

Assessment: built/tested opt-in candidate, not deployed. LocalService provider/token access, combined service-stop timing, real Telegram delivery, sensor expansion, resource budgets and long-duration stability remain pending. The earlier race-runtime limitation is unchanged.

### User-requested PC temperature removal - 2026-11-12

PC temperature monitoring and its provider expansion were explicitly removed from both app and current plan. Historical investigations remain evidence only; [E-005](E-005-acpi-temperature-unsupported.md) is closed by scope removal, not a claim that the source was fixed.

Full tests/vet/build, JavaScript syntax, live PC-only diagnostic and scripted dashboard checks passed for the removal contract. CPU/GPU/disk metrics remain; PC temperature fields/queries/cards and copied reader/license are gone; UPS temperature remains. The diagnostic still reports the unrelated missing physical-disk performance source honestly. Feature-specific evidence is in [F-016](F-016-pc-hardware-monitoring.md).

### Opt-in PC shutdown / Telegram commands - 2026-11-12

Targeted fake-only config, notification, state, policy, platform and web suites passed, then passed five repeated runs. Full Go tests/vet and Node syntax/DOM tests passed for the integrated candidate. Reusable DOM regression checks use Node's built-in runner (no dependencies installed):

```powershell
go test .\...
go vet .\...
node --check .\internal\web\static\app.js
node --test .\internal\web\app_test.cjs
go build -o .\bin\upsmart-next.exe .
```

Feature-local evidence: [F-009](F-009-pc-shutdown.md) covers fake-clock policy/non-forced adapter; [F-017](F-017-telegram-notifications.md) covers local HTTP command/auth/replay tests; [F-004](F-004-web-dashboard.md) and [F-014](F-014-backend-api.md) cover visible policy and unchanged readonly boundaries. Only injected fakes are used for OS requests, including tests labeled `live`; no real shutdown executable is launched by tests.

No active configuration edit, existing-monitor replacement/restart, service registration, Telegram provider traffic, real Windows shutdown or UPS power command was performed. Actual bot delivery, LocalService privileges and OFF-06 remain pending. The race-runtime tooling limitation remains open; repeated ordinary tests do not establish race freedom.

Final separate candidate build passed. Its offline `--help` and `validate --config .\config.local.json` checks passed; the latter explicitly reports PC shutdown disabled. Final repeated affected-package tests also verify the exact copyable confirmation command without trimming punctuation. All 106 local project documentation links resolved.

### Native Fyne desktop candidate - 2026-11-12

Full Go tests/vet and Node web tests pass for separate desktop/backend integration. Added tests cover native headless widget rendering, real Windows named-pipe round trip using fake data, durable policy save/failed save, stale effective-settings revision, confirmed mode/countdown mismatch, malformed inputs, cancellation, and pending/terminal reconfiguration guards. Telegram tests now verify checking-only behavior and rejection of former power commands.

The actual GUI executable initially exposed Windows loader error 193 despite a successful default build; [E-006](E-006-native-desktop-linker.md) records the verified internal-linker build fix. Native window `UPSmart Monitor` subsequently opened and exited 0 in a safe no-backend-request smoke test. This is native startup evidence, not visual layout/tray or live-service verification.

The backend separately builds with `CGO_ENABLED=0`; unchanged local configuration still validates with PC shutdown disabled. No installed compiler was replaced, no application binary was manually patched, and no active monitor configuration/restart, service install, real bot connection, USB read or power test occurred. Actual cross-account/LocalService integration, visual/tray checks, signing/redistribution and the existing race-runtime limitation remain pending.
