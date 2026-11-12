# F-011 - Web access safety

Updated: 2026-11-12 | Priority: P0 | Status: Web tests pass; deployment protection implemented but not installed

## Purpose and user journey

Mục tiêu: xem trạng thái trên máy hiện tại mà không vô tình mở quyền điều khiển nguồn cho mạng LAN hoặc website khác.

The user opens a local, read-only dashboard. Changing service policy requires administrator-controlled local configuration.

## Implementation

- Proposed entry points: `internal/web/security.go`, service/config ACL setup.
- Listen on IPv4 loopback `127.0.0.1` only, explicitly confirmed by the user on 2026-11-12. Reject wildcard/non-loopback configuration; do not add firewall rules or enable LAN access. This also applies to future PC hardware and notification-status APIs.
- Read-only status/event APIs may be unauthenticated on loopback; explicitly disclose that other local processes/users may read them.
- No browser endpoint may modify policy, stop the service, issue raw USB queries, request OS shutdown or alter UPS output in the initial release.
- Validate the Host header against the configured loopback authority and port. Add `localhost` only as an explicit accepted alias.
- Do not enable CORS. Reject cross-origin API requests when an Origin header is supplied; allow same-origin browser use and local CLI requests without Origin.
- Serve no secrets or arbitrary filesystem paths. Sanitize error details and avoid exposing raw captures by default.
- Set a restrictive Content Security Policy, frame protection, MIME-sniffing protection and `Cache-Control: no-store` for live APIs.
- Restrict configuration, executable and writable log directories using Windows ACLs; a privileged service must not load executable code/assets from user-writable paths.
- Use server timeouts, response limits and bounded concurrent request handling.

## Acceptance and tests

- SEC-01: bind configuration accepts only supported loopback addresses; another LAN host cannot connect.
- SEC-02: unexpected Host and cross-origin API requests are rejected; normal local UI remains usable.
- SEC-03: unsupported methods, oversized input and arbitrary file paths cannot reach privileged operations.
- SEC-04: security headers and untrusted-text rendering are checked in HTTP/browser tests.
- SEC-05: ordinary local users cannot replace privileged configuration or executable content.

## Decisions and boundaries

Loopback is not an authentication boundary against local malware. If future browser mutation endpoints are requested, require authenticated authorization, CSRF protection and a separate design decision before implementation. Remote/LAN support and TLS are out of scope.

Planned Telegram delivery is outbound HTTPS only; it must not introduce an inbound listener or public webhook.

## Results and next steps

2026-11-12: implemented in `internal/web/server.go` and configuration validation. Requests with an unexpected Host/cross-origin Origin are rejected; only GET/HEAD are accepted; no power-control route, CORS or LAN bind is supported. Security headers, embedded assets and 32-handler admission limit are present.

HTTP tests passed for Host/Origin checks, same-origin use, unsupported methods, missing shutdown endpoints, headers and body-free HEAD responses including errors. Configuration tests reject wildcard binding. Actual loopback API/browser operation passed; no external LAN-host probe or adversarial load test was performed.

No privileged service or ACL changes exist yet. Other local users/processes may read the unauthenticated dashboard; its footer states this explicitly. Assessment: core web boundaries are tested, not a full security audit. Next: service identity/ACL design before privileged service installation.

Candidate update: protected deployment is implemented in `internal/service/install_windows.go`, with Administrator ownership, read-only executable/configuration access for LocalService/Users, a separate writable logs directory and explicit rejection of reparse points/unexpected existing ACLs. Temporary files are created with their protected security descriptor before writing and renamed only after verification. Installation requires elevation; no protected files or services were created in this session.

A Windows filesystem test confirmed that an existing unprotected executable is rejected without changing its bytes or creating staging output. Actual protected creation, inheritance, non-admin denial and LocalService access still need an approved elevated integration test. These partial tests are not a security audit or proof of SEC-05 on an installed service.
