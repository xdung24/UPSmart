# UPSmart Replacement - Feature Plan

Last updated: 2026-11-12

Goal: replace legacy UPSmart for the ARES AR2000 with a lightweight 64-bit Windows background service, primary dual-mode Fyne desktop (daemon client or direct USB) and optional read-only web dashboard.

Planned extension: optional CPU/GPU usage and memory, plus hard disk/SSD capacity and activity in the same dashboard, separate from UPS readings.

Go was confirmed by the user on 2026-11-12. The first console milestone reads real AR2000 status. A separate persistence/service/shutdown candidate is built and tested, but not deployed: the user selected build-only, with no monitor restart or configuration changes. Actual SCM/LocalService, Telegram delivery and Windows shutdown verification remain pending.

## Features

| ID | Feature | Priority | Status | Details |
| --- | --- | --- | --- | --- |
| F-000 | Project scope, shared decisions and delivery milestones | P0 | Monitoring milestone delivered | [Scope](F-000-project-scope.md) |
| F-001 | USB communication: verify identity, protocol and read-only status queries | P0 | Live status verified | [USB](F-001-usb-communication.md) |
| F-002 | Telemetry: voltage, frequency, load, temperature and capacity with honest quality labels | P0 | Six readings live; capacity unavailable | [Telemetry](F-002-telemetry.md) |
| F-003 | Power status: mains loss/restoration, battery operation and verified fault flags | P0 | Simulated transitions; live mains baseline | [Power status](F-003-power-status.md) |
| F-004 | Local dashboard: live readings, connection health, freshness and active policy | P0 | UPS live verified; policy/countdown UI DOM-tested | [Dashboard](F-004-web-dashboard.md) |
| F-005 | Connection resilience: bounded I/O, automatic reconnection and explicit errors | P0 | Helper timeout tested; physical reconnect pending | [Resilience](F-005-connection-resilience.md) |
| F-006 | Native 64-bit Windows service with automatic startup | P1 | Candidate implemented; handler tests pass; SCM pending | [Service](F-006-windows-service.md) |
| F-007 | Bounded timestamped event history | P1 | Rotation/recovery tested; not enabled locally | [Events](F-007-event-history.md) |
| F-008 | Validated persistent configuration and visible active policy | P1 | Shutdown/history/hardware/Telegram schema tested; local unchanged | [Configuration](F-008-configuration.md) |
| F-009 | Opt-in graceful PC shutdown with countdown and restoration cancellation | P1 | Policy/adapter fake-tested; disabled locally; no real shutdown | [PC shutdown](F-009-pc-shutdown.md) |
| F-010 | Delayed UPS output cut-off only with verified safe sequencing | P2 | Research gate; disabled | [UPS power-off](F-010-ups-power-off.md) |
| F-011 | Loopback-only read-only web access and protected local configuration | P0 | Protected-deployment code tested partially; admin install pending | [Access safety](F-011-web-access-safety.md) |
| F-012 | Standalone delivery, lifecycle instructions and hardware comparison | P1 | CMD build for both apps verified; candidates not deployed | [Delivery](F-012-delivery-verification.md) |
| F-013 | Architecture, domain contracts and bounded resource ownership | P0 | Shared console/SCM/direct desktop engine; exclusive USB ownership | [Architecture](F-013-architecture.md) |
| F-014 | Versioned cached APIs, health endpoints and server limits | P0 | Cached policy and command health tested; still read-only | [Backend/API](F-014-backend-api.md) |
| F-015 | Acceptance tests, hardware safety gates and evidence requirements | P0 | Full tests/vet/build pass; race tool still blocked | [Quality](F-015-quality.md) |
| F-016 | PC hardware monitoring: CPU/GPU usage, GPU memory and disk metrics | P2 | Collector/API candidate; PC temperatures removed by request | [Hardware monitoring](F-016-pc-hardware-monitoring.md) |
| F-017 | Telegram alerts, authorized status/checking commands and optional summaries | P1 | Checking-only commands mock-tested; power commands removed by request | [Telegram](F-017-telegram-notifications.md) |
| F-018 | Dual-mode Fyne desktop: daemon preferred, direct USB fallback | P1 | Tests/build/native startup pass; live handover and visual checks pending | [Desktop](F-018-native-desktop.md) |

P0 = monitoring foundation and safety; P1 = background operation and automation; P2 = optional hardware monitoring/control.

## Delivery order

1. Verify monitoring-only USB communication and compare readings with UPSmart. Avoid simultaneous polling by both applications.
2. Deliver the dashboard, reconnect handling, event history and Windows service.
3. Implement and verify opt-in PC shutdown with simulated events before controlled live testing.
4. Add UPS power-off only after its command, delay and power-restoration behavior are verified.

Optional workstream: add PC hardware monitoring after provider feasibility checks. It does not depend on, or enable, shutdown controls.

Optional workstream: add opt-in Telegram alerts from monitoring events; confirm alert selection and summary cadence before enabling outbound delivery.

## Open decisions and limits

- Go confirmed. Current Q1 query does not report capacity; no estimate is fabricated. Temperature sensor location remains unverified.
- Localhost-only listening explicitly confirmed; applies to all dashboard/API features. Telegram delivery is outbound only.
- PC hardware temperatures are excluded by user request. Existing UPS-reported temperature remains in scope.
- New desktop direction supersedes Telegram power commands: desktop controls require no UAC, using account-restricted local IPC; Telegram becomes checking/status plus notifications.
- Automatic outage/verified low-battery shutdown and confirmed desktop manual controls selected; explicit mode/timing/authorized Windows account must be configured before activation. Mains restoration cancels pending requests; no real shutdown approved.
- Confirm whether UPS power-off should remain off or restart automatically after mains restoration.
- Scheduling, remote/LAN access, email/SMS alerts, battery self-tests and beeper control are outside the initial scope.
- Feature details are linked above; errors/observations belong in [TEST](TEST.md). All rows share the last-update date above. "Specified" does not mean implemented or verified.
- Monitoring tests and a live read-only USB/browser check passed. No mains-loss, real PC shutdown or UPS power-control tests have been performed.
