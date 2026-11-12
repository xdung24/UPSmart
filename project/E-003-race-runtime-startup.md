# E-003 - Race runtime startup observation

Updated: 2026-11-12 | Status: Blocked tooling observation | Priority: P2

Features: [F-005](F-005-connection-resilience.md), [F-013](F-013-architecture.md), [F-015](F-015-quality.md).

## Impact, environment and reproduction

Windows 10 Pro 10.0.19045 amd64; installed Go reports `go1.26.5 windows/amd64`.

Run:

```powershell
go test -race .\internal\state .\internal\web
```

Expected: both test executables start and execute under the race detector. Actual: both exit at startup with `exit status 0xc0000139`; no race report or test result is produced.

## Cause and approach

Exact cause is unknown. The exit status suggests a Windows loader/entry-point problem, but no failing DLL or symbol has been identified. This is not a confirmed application bug or an observed data race.

Ordinary targeted tests, vet and Windows amd64 build pass. Do not remove concurrency tests, change the machine runtime or install tooling speculatively to make this observation disappear.

Next: capture loader diagnostics on an approved test environment and rerun with a supported race-capable Windows toolchain. Until then, explicitly report race coverage unavailable.

## Verification and decisions

2026-11-12: initial race command failed as above. Non-race tests and actual monitoring/API/browser checks subsequently passed; they do not close this observation or prove absence of races.

No user-approved waiver, environment modification or fix has been recorded. Keep open until the race tests execute successfully or the user explicitly accepts a documented alternative.
