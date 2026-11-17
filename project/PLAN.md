# UPSmart Replacement - Feature Plan

Last updated: 2026-11-17

Goal: replace legacy UPSmart for the ARES AR2000 with a lightweight 64-bit Windows daemon and primary Fyne desktop. The desktop runs standalone with direct USB or connects to a daemon through protected local IPC; there is no web UI or TCP API.

Planned extension: optional CPU/GPU usage and memory, plus hard disk/SSD capacity and activity in the desktop app, separate from UPS readings.

Go was confirmed by the user on 2026-11-12. The first console milestone reads real AR2000 status. The latest service/desktop IPC candidate is built and tested, but not deployed by the agent; no active configuration or service was changed. Live desktop reconnection, Telegram delivery and Windows shutdown verification remain pending.

## Features

| ID | Feature | Priority | Status | Details |
| --- | --- | --- | --- | --- |
| F-000 | Project scope, shared decisions and delivery milestones | P0 | Monitoring milestone delivered | [Scope](F-000-project-scope.md) |
| F-001 | USB communication: verify identity, protocol and read-only status queries | P0 | Live status verified; independent-startup observation E-008 open | [USB](F-001-usb-communication.md) |
| F-002 | Telemetry: voltage, frequency, load, temperature and capacity with honest quality labels | P0 | Capacity estimated from user-provided pack endpoints; legacy-daemon display fallback tested | [Telemetry](F-002-telemetry.md) |
| F-003 | Power status: mains loss/restoration, battery operation and verified fault flags | P0 | Simulated transitions; live mains baseline | [Power status](F-003-power-status.md) |
| F-004 | Web dashboard | P0 | Removed at user request (2026-11-16) | [Dashboard history](F-004-web-dashboard.md) |
| F-005 | Connection resilience: bounded I/O, automatic reconnection and explicit errors | P0 | Helper timeout tested; startup E-008/reconnect verification pending | [Resilience](F-005-connection-resilience.md) |
| F-006 | Native 64-bit Windows service with automatic startup | P1 | Default desktop authorization tested; E-010 live reconnection pending | [Service](F-006-windows-service.md) |
| F-007 | Bounded timestamped event history | P1 | Rotation/recovery tested; not enabled locally | [Events](F-007-event-history.md) |
| F-008 | Validated persistent configuration and visible active policy | P1 | Flat YAML/defaults/CLI precedence verified; local values preserved; legacy reader retained | [Configuration](F-008-configuration.md) |
| F-009 | Opt-in graceful PC shutdown with countdown and restoration cancellation | P1 | Policy/adapter fake-tested; disabled locally; no real shutdown | [PC shutdown](F-009-pc-shutdown.md) |
| F-010 | Delayed UPS output cut-off only with verified safe sequencing | P2 | Research gate; disabled | [UPS power-off](F-010-ups-power-off.md) |
| F-011 | Web access safety | P0 | Removed with web support at user request (2026-11-16); pipe ACLs remain in F-018 | [Web safety history](F-011-web-access-safety.md) |
| F-012 | Standalone delivery, lifecycle instructions and hardware comparison | P1 | CMD build for both apps verified; candidates not deployed | [Delivery](F-012-delivery-verification.md) |
| F-013 | Architecture, domain contracts and bounded resource ownership | P0 | Shared console/SCM/direct desktop engine; exclusive USB ownership | [Architecture](F-013-architecture.md) |
| F-014 | Daemon-to-desktop IPC contracts and limits | P0 | Named-pipe IPC retained; TCP/web APIs removed (2026-11-16) | [Backend/IPC](F-014-backend-api.md) |
| F-015 | Acceptance tests, hardware safety gates and evidence requirements | P0 | Full tests/vet/build pass; race tool still blocked | [Quality](F-015-quality.md) |
| F-016 | PC hardware monitoring: CPU/GPU usage, GPU memory and disk metrics | P2 | Collector/API candidate; PC temperatures removed by request | [Hardware monitoring](F-016-pc-hardware-monitoring.md) |
| F-017 | Telegram alerts, authorized status/checking commands and optional summaries | P1 | Local relay power/connection alerts configured 2026-11-17; offline checks passed; restart/live receipt pending | [Telegram](F-017-telegram-notifications.md) |
| F-018 | Fyne desktop: daemon named pipe or direct USB | P1 | Animated schematic spans two rows; narrower side readings and animation state tested | [Desktop](F-018-native-desktop.md) |

P0 = monitoring foundation and safety; P1 = background operation and automation; P2 = optional hardware monitoring/control.

## Delivery order

1. Verify monitoring-only USB communication and compare readings with UPSmart. Avoid simultaneous polling by both applications.
2. Deliver the native desktop, reconnect handling, event history and optional Windows service.
3. Implement and verify opt-in PC shutdown with simulated events before controlled live testing.
4. Add UPS power-off only after its command, delay and power-restoration behavior are verified.

Optional workstream: add PC hardware monitoring after provider feasibility checks. It does not depend on, or enable, shutdown controls.

Optional workstream: add opt-in Telegram alerts from monitoring events; confirm alert selection and summary cadence before enabling outbound delivery.

## Open decisions and limits

- Go confirmed. Q1 does not report capacity; the desktop now labels the user's 24-27 V pack mapping as an estimate, excluded from shutdown policy. Temperature sensor location remains unverified.
- Historical localhost-only web restriction is superseded by the user's 2026-11-16 request to remove web support. There is no TCP listener/dashboard; Telegram delivery remains outbound only.
- PC hardware temperatures are excluded by user request. Existing UPS-reported temperature remains in scope.
- New desktop direction supersedes Telegram power commands: desktop controls require no UAC, using account-restricted local IPC; Telegram becomes checking/status plus notifications.
- Automatic outage/verified low-battery shutdown and confirmed desktop manual controls selected; explicit mode/timing/authorized Windows account must be configured before activation. Mains restoration cancels pending requests; no real shutdown approved.
- Confirm whether UPS power-off should remain off or restart automatically after mains restoration.
- Scheduling, remote/LAN access, email/SMS alerts, battery self-tests and beeper control are outside the initial scope.
- Feature details are linked above; errors/observations belong in [TEST](TEST.md). All rows share the last-update date above. "Specified" does not mean implemented or verified.
- Historical monitoring tests and a live read-only USB/browser check passed before web removal. No mains-loss, real PC shutdown or UPS power-control tests have been performed.
