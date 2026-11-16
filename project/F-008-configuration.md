# F-008 - Configuration

Updated: 2026-11-16 | Priority: P1 | Status: Desktop configuration tested; local file unchanged

Current desktop configuration: [F-018](F-018-native-desktop.md) uses the explicit `desktop` group for local IPC/settings. Web support was removed on 2026-11-16; `listen` and `web_enabled` are rejected as unknown legacy fields and must be deleted from existing JSON. Manual shutdown requires desktop configuration, not Telegram. `telegram.allowed_user_ids` independently enables checking-only commands. Earlier Telegram-power examples below are historical and must not be used to activate current desktop controls.

## Purpose and user journey

Mục tiêu: người dùng biết chính xác chính sách đang áp dụng và không vô tình bật tắt nguồn vì cấu hình lỗi.

An administrator edits a local configuration, validates it with the CLI, then restarts the daemon. The desktop shows monitoring and effective policy without exposing secrets.

## Implementation

- Proposed entry points: `internal/config/config.go`, `internal/config/validate.go`.
- Versioned JSON. No-argument startup uses `.\config.local.json` from the current directory, as explicitly requested by the user. Explicit `serve`/`validate` without `--config` use `%ProgramData%\UPSmartMonitor\config.json`, matching protected service deployment. Explicit `--config` selects another file. No invalid/missing-file fallback is performed. The earlier proposed `UPSmart` directory was never installed by this project.
- Groups: device selector, polling/deadline/staleness, event limits, desktop IPC/settings, PC shutdown policy and separately gated UPS output policy.
- All automation defaults to disabled. A missing production configuration is a startup error with instructions to generate a monitoring-only example; never silently replace a broken file with defaults.
- Reject unknown fields, unsupported schema versions, invalid durations, invalid percentages and unsupported enabled capabilities.
- Require positive bounded intervals, transaction deadline below poll interval and stale threshold greater than poll interval plus deadline.
- CLI validation reports field-specific errors and exits nonzero; generation never overwrites an existing file without explicit approval.
- Initial version has no hot reload or browser editing. Activation requires restart; display the effective configuration version and policy.
- If policy is enabled but the required verified battery flag/measurement is unsupported, report a policy-blocked state and do not execute that trigger. Transient missing USB must not prevent monitoring service startup.
- Protect configuration writes with administrator/service-account ACLs. If a later CLI writes configuration, use same-directory temporary output and atomic replacement.

## Acceptance and tests

- CFG-01: valid monitoring-only configuration round-trips with automation off.
- CFG-02: corrupt JSON, typo fields, invalid timing relationships and non-loopback addresses fail explicitly.
- CFG-03: enabled output control without a verified capability is rejected.
- CFG-04: an unprivileged user cannot modify the service policy.
- CFG-05: editing the file alone does not change a running policy; restart and the desktop reflect the new active configuration.

## Decisions and boundaries

Go is confirmed. Retention and shutdown timing are explicitly configurable; no active shutdown thresholds were enabled. User-selected controls and activation boundaries are in [F-009](F-009-pc-shutdown.md). The optional [Telegram sender](F-017-telegram-notifications.md) references a token environment variable rather than storing the bot token in configuration.

## Results and next steps

2026-11-12: implemented `internal/config/config.go`. The original schema included `listen`; the current schema no longer has web/listener fields. Unknown keys, including the removed `listen` and `web_enabled` fields, are rejected.

Bounds: schema version 1; poll 1 second-1 minute; transaction timeout 100 ms-30 seconds and less than poll interval; stale 1 second-5 minutes and greater than poll + timeout. Files over 64 KiB, trailing JSON and invalid timing relationships fail explicitly. No hot reload.

Historical tests covered the original loopback binding. Current tests cover valid monitoring-disabled configuration, unknown/removed web fields, wrong version, invalid timing, trailing data and malformed JSON. The example configuration is validated; local configuration is left unchanged.

The repository example disables monitoring. The ignored local file enables monitoring only after the user's read-only handover approval and uses port 18765 because 8080 is occupied. No ProgramData configuration or ACLs were installed.

```powershell
Copy-Item .\config.example.json .\config.local.json
# Edit the copy: choose a free loopback port; enable monitoring only after closing UPSmart.
.\bin\upsmart.exe validate --config .\config.local.json
.\bin\upsmart.exe serve --config .\config.local.json
```

Do not overwrite an existing local file without preserving its settings. Assessment: monitoring-schema validation is working; device selectors, persistent event options, administrator ACLs and future opt-in policies remain pending.

### Persistence/service candidate - 2026-11-12

Optional `event_history` is now supported; omission retains memory-only behavior with no automatic filesystem logging. Example fragment for a future explicitly approved console configuration:

```json
"event_history": {
  "directory": "C:\\Users\\dung\\UPSmart\\data\\events",
  "max_file_bytes": 5242880,
  "max_files": 5
}
```

Require an absolute local non-root directory, size 1024-52428800 bytes and count 1-10. UNC paths, relative paths, filesystem roots and invalid limits are rejected. Reducing the count intentionally removes numbered archives beyond the new retention limit. Unknown fields and original timing checks remain enforced.

Service installation always uses the protected `%ProgramData%\UPSmartMonitor\logs` directory; it preserves explicit retention limits, or uses 5 MiB x 5 files if omitted. Existing service configuration is never silently overwritten with differing values.

Tests for valid/invalid history options and existing configuration regressions passed. The user selected build-only: neither local/example configuration was changed and logging was not enabled. The fragment above is documentation, not active configuration. Next: obtain deployment approval before adding it; power-policy settings remain absent.

### Optional PC metrics and Telegram candidate - 2026-11-12

Two optional groups are now accepted. Omission disables the respective feature; existing configurations retain their behavior. Example additions, not changes to any active file:

```json
"hardware": {"interval": "15s"},
"telegram": {
  "token_env": "UPSMART_TELEGRAM_TOKEN",
  "chat_id": "<destination-chat-id>",
  "events": ["startup", "connected", "connection_error", "power_changed", "low_battery_changed", "fault_changed"]
}
```

Hardware interval must be 10 seconds-5 minutes. Telegram settings require a supported nonempty event selection and valid token-variable/destination syntax. Validation does not send a message or require the environment variable to be populated. Application startup resolves the token and fails explicitly before binding if it is missing/invalid.

The token must be supplied locally in the executing account's environment; do not commit it, place it in JSON, or expose it in browser/status/log output. Console environment setup does not automatically configure LocalService; account-specific secret provisioning and actual service delivery remain unverified. Environment variables are not an encrypted credential vault.

Tests passed for both optional groups, combined selection, invalid intervals, missing event selection and attempted inline-secret/unknown fields. The active local/example files remain unchanged.

### Earlier Windows shutdown and Telegram controls - 2026-11-12 (manual transport superseded by F-018)

The previous monitoring-only rejection of shutdown fields is superseded by optional `power_control`. Omission leaves all shutdown controls disabled. Enabled controls require explicit `mode: "dry_run"` or `"live"`; there is no implicit live mode. Unknown control fields, including UPS output-off options, remain invalid.

Documentation-only dry-run example; these timings and illustrative IDs were **not** written to either configuration or activated:

```json
"power_control": {
  "mode": "dry_run",
  "automatic": {
    "enabled": true,
    "outage_after": "5m",
    "low_battery": true,
    "countdown": "30s"
  },
  "manual": {
    "enabled": true,
    "countdown": "30s",
    "allowed_user_ids": [123456789]
  }
},
"telegram": {
  "token_env": "UPSMART_TELEGRAM_TOKEN",
  "chat_id": "123456789",
  "events": ["power_changed", "low_battery_changed", "shutdown_pending", "shutdown_cancelled", "shutdown_requested", "shutdown_dry_run", "shutdown_failed"]
}
```

- Enable either automatic or manual independently. Enabled automation requires `monitoring_enabled: true` and at least one of `outage_after` (1 second-24 hours) or `low_battery: true`. No capacity/voltage threshold exists.
- Each enabled section requires its own explicit countdown, 5 seconds-10 minutes. Low battery does not bypass this countdown.
- Enabled manual controls require the Telegram group, a nonzero signed numeric chat ID (no channel username), and 1-16 positive unique sender IDs. Replace the illustrative chat and sender IDs independently: they need not be the same.
- Telegram still requires a nonempty explicit event list. Additional supported events: `shutdown_pending`, `shutdown_cancelled`, `shutdown_requesting`, `shutdown_requested`, `shutdown_dry_run`, `shutdown_failed`, `shutdown_state_changed`.
- Validation is offline and does not resolve the token. Startup validates/resolves enabled credentials before binding. No configuration edits take effect without restart; restarting discards prior confirmations and countdowns and requires fresh automatic qualification.
- The status API shows effective enablement, mode, trigger/countdown settings and lifecycle without chat IDs, sender allowlists or secrets.

Config tests passed for explicit dry-run/live mode, monitoring dependency, mandatory triggers/countdowns, Telegram/allowlist dependency, numeric destination and unsupported output controls. Runtime/local/example files were not edited. Start with an approved dry-run deployment; review [F-009](F-009-pc-shutdown.md) and [F-017](F-017-telegram-notifications.md) before any live activation.

### Desktop configuration candidate - 2026-11-12

`desktop.settings_file` is an explicit absolute local JSON path; its parent must exist and be writable to the backend account. Console mode can authorize its own Windows SID; services require explicit `desktop.user_sid` and the protected service settings path described in F-018. When this group is configured, the daemon exposes the named pipe to the authorized desktop account.

Saved desktop policy overrides base `power_control` only when the desktop group is explicitly present. `Config.Effective` is shared by file load/CLI validation and runtime startup, so a saved LIVE policy cannot be reported as disabled by validating only the base file. Invalid/corrupted/oversized saved policy fails explicitly. Automatic monitoring dependencies are checked again after loading/saving.

Legacy `power_control.manual.allowed_user_ids` is parsed for compatibility but no longer grants any authority and is not required for desktop manual controls. Use the independent Telegram allowlist only for checking commands. Existing local/example files are unchanged.
