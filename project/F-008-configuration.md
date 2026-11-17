# F-008 - Configuration

Updated: 2026-11-16 | Priority: P1 | Status: Flat YAML/defaults/CLI precedence tested; local values preserved; both apps rebuilt

Current desktop configuration: [F-018](F-018-native-desktop.md) uses `desktop-settings-file` and optional `desktop-user-sid` for local IPC/settings. Web support was removed on 2026-11-16; `listen` and `web_enabled` are rejected as unknown legacy fields and must be deleted from existing configuration. Manual shutdown requires desktop configuration, not Telegram. `telegram-allowed-user-ids` independently enables checking-only commands. Earlier Telegram-power examples below are historical and must not be used to activate current desktop controls.

Current optional notification relay: set `notify-addr` to rdpwatcher's explicit `host:port` and provide nonempty `telegram-events`. Omit `telegram-token-env`, `telegram-chat-id` and `telegram-allowed-user-ids`; mixed relay/direct settings are rejected. UPSmart then needs no bot credentials and does not poll bot commands. Offline validation does not contact the endpoint. Restart is required. See [F-017](F-017-telegram-notifications.md#optional-rdpwatcher-relay---2026-11-16).

## Current rdpproxy-style options - 2026-11-16

Người dùng xác nhận áp dụng cách cấu hình của rdpproxy: **defaults → YAML → CLI**, YAML phẳng có tên khóa trùng tham số dài của daemon. Thiếu file thì thông báo rõ và dùng defaults an toàn; file tồn tại nhưng sai vẫn báo lỗi. Mục tiêu là một cách chạy nhất quán ở console và Windows service, không tự bật thông báo hoặc chính sách nguồn.

### User journey and parameter mapping

- Default daemon file: `config.yaml` in the working directory for both no-argument and explicit console aliases. Select with `--config PATH`, `--conf PATH`, `-c PATH` or equals syntax. Explicit empty paths fail. ProgramData is no longer a console-config fallback.
- Core YAML/CLI parameters: `schema-version` (1), `poll-interval` (2s), `transaction-timeout` (1s), `stale-after` (10s), `monitoring-enabled` (false).
- Optional persistence: `event-history-directory`, `event-history-max-file-bytes`, `event-history-max-files`. Directory enables persistence; omitted limits then use 5 MiB/5 files. Explicit zero/invalid limits fail. Installation retains its fixed service logs directory.
- Optional PC metrics: `hardware-interval`; omission disables the collector.
- Notifications: `notify-addr` for relay; or `telegram-token-env`/`telegram-chat-id` for direct delivery; plus `telegram-events` and optional checking-only `telegram-allowed-user-ids`. Inline tokens remain unsupported.
- Power controls: `power-mode`, `automatic-shutdown`, `outage-after`, `low-battery-shutdown`, `automatic-countdown`, `manual-shutdown`, `manual-countdown`. There is no implicit live mode or countdown. Optional `manual-allowed-user-ids` retains legacy metadata only, never authorization.
- Desktop IPC: `desktop-user-sid`, `desktop-settings-file`. Service installation now supplies the installing user's SID and fixed service path when omitted; an explicit SID is preserved. Console IPC is still optional. Existing account/path validation and saved-policy JSON are unchanged. Saved desktop power policy still overrides base power settings after CLI merging; automatic service IPC setup requires explicit review of any pre-existing saved policy so it cannot silently enable old controls. See [E-010](E-010-service-desktop-ipc-configuration.md).
- Boolean parameters support a bare positive flag and explicit `=false`, e.g. `--monitoring-enabled=false` overrides YAML true. YAML lists become repeatable flags; additional CLI list items append, as in rdpproxy. Scalars use the last CLI value.
- Actions are CLI-only: `--install`, `--remove`, `--start`, `--stop`, `--status`, `--validate`. Conflicting actions fail. YAML cannot trigger service changes. Management remains usable even with a malformed application config.
- `--help`/`help` and `--version`/`version`/`-v` do not start monitoring. `serve` and `validate` remain console aliases; diagnostic commands are retained. Windows service subcommands are replaced by action flags; see [F-006](F-006-windows-service.md#current-service-model-rdpproxy-alignment---2026-11-16).

```yaml
schema-version: 1
poll-interval: 2s
transaction-timeout: 1s
stale-after: 10s
monitoring-enabled: false
# Optional relay; no direct credentials:
# notify-addr: 127.0.0.1:50061
# telegram-events: [startup, power_changed, connection_error]
```

### Design, compatibility and validation

- Entry points: `internal/config/options.go` (`Parameters`, `Options`, `Parse`, `Marshal`), `config.go` (`Load`, `Effective`, validation), daemon main and the service installer. Uses the reference's `go-flags v1.7.0` and existing `yaml.v3`.
- Validate known fields/types, duplicates, mapping root, exactly one YAML document and the actual 64 KiB read limit before applying flags. Then validate merged durations, optional groups and power/account contracts before worker startup or installation. Present malformed files never fall back to defaults.
- Valid legacy nested/underscore files with `schema_version` remain explicitly readable and validated for compatibility; serialization/install writes only flat names. Legacy files must already pass base validation before CLI overrides, avoiding silent loss of malformed nested groups. JSON-compatible legacy documents still decode through YAML.
- Both ignored local copies were converted to flat keys, preserving `monitoring-enabled: true`, 2s polling, 1s timeout and 10s freshness. No notification or shutdown settings were added. Existing running processes are not restarted or hot-reloaded.
- Desktop `--config PATH` consumes either valid format through `Load`, without rewriting it. Missing explicit desktop files still fail; no-config desktop-owned USB defaults remain unchanged. No general daemon action flags were added to the GUI.
- Installed service receives validated effective flat YAML including CLI overrides, with an explicit absolute path; source comments are not copied. Current account/deployment details and the explicitly selected LocalSystem boundary are authoritative in F-006, replacing older LocalService references below.

2026-11-16 actual checks: full tests/vet and both Windows builds passed. Built help and `--validate -c` passed for the example/root/bin YAML files, with PC shutdown disabled. Tests cover defaults/file/CLI precedence, `-c`/`--conf`/long aliases, explicit false override, appended list flags, flat encode/load round-trip, unknown/duplicate/nested/oversized/multiple-document/null inputs, CLI-only actions, conflicts and management despite malformed YAML; existing duration, saved-policy, notification and desktop-direct regressions pass.

Assessment: offline configuration/CLI behavior is verified. Missing-file defaults disable hardware polling, notifications, persistence and power automation; a notice is emitted rather than silently selecting another file. Actual service startup, network delivery, hardware reads and live shutdown were not tested. See [F-006](F-006-windows-service.md) for elevated deployment limits.

## YAML configuration - 2026-11-16

Historical migration record: the underscore/nested schema and mandatory-file/default-path contracts below are superseded by the current flat-option section above; dated checks remain historical evidence.

Người dùng chọn YAML cho cấu hình ứng dụng để dễ đọc, thêm ghi chú và chỉnh các nhóm cấu hình. Tên trường và các giá trị đang áp dụng được giữ nguyên; đổi định dạng không tự bật Telegram hoặc chính sách tắt máy.

- Template: [config.example.yaml](../config.example.yaml), monitoring disabled. Daemon no-argument startup reads `config.yaml` in the working directory; explicit `serve`/`validate` retain their existing defaults. New service deployments instead use `%ProgramFiles%\UPSmart\config.yaml` beside the installed binary, via an explicit absolute `--config` path. Desktop `--config PATH` reads the same YAML only for direct mode; without it, existing USB-only defaults remain unchanged.
- `internal/config.Load` uses `yaml.v3` with known-field checking at every level, a single document and the existing 64 KiB limit. Comments/block lists work. Duplicate keys, unknown fields, extra/empty second documents and invalid settings fail explicitly. All existing duration, policy, account and notification validation remains.
- Each configuration option type has explicit snake-case YAML tags. Service installation writes YAML and compares decoded settings before allowing reuse; protected write/ACL behavior is unchanged.
- Subsequent user-selected service placement: validate the chosen source, deploy its effective configuration beside the installed executable, then register that copy's absolute path. Logs remain in ProgramData. Scope, path/compatibility test evidence and pending elevated-copy/SCM verification are in [F-006](F-006-windows-service.md#configuration-beside-the-installed-daemon---2026-11-16).
- JSON syntax remains readable through YAML's built-in JSON compatibility when an old path is explicitly selected. There is no automatic search/fallback to `config.local.json`. New service commands use YAML; service management still accepts the exact old protected `config.json` registration so an existing deployment can be stopped/uninstalled deliberately. It still rejects other paths, executables and extra arguments. Existing services are not migrated/re-registered automatically.
- Desktop-saved `desktop-policy.json`, IPC/diagnostic JSON and event JSONL are internal state/protocol formats, not the user-edited app configuration; they remain unchanged.
- Local root and `bin` copies were converted from `config.local.json` to `config.yaml`. Existing timing and `monitoring_enabled: true` were preserved; only the retired, already-invalid web `listen` key was omitted. The old local JSON files were moved, not retained as parallel active configurations. No notification/power-control options were added.

2026-11-16 actual checks: full `go test .\...` and `go vet .\...` passed. Tests cover native YAML with all nested configuration groups, encode/load round-trip (including Windows paths and sender IDs), relay configuration, comments, unknown/duplicate keys, exact 65,536-byte acceptance and 65,537-byte rejection, extra documents, invalid roots/types, existing JSON compatibility, CLI local/ProgramData defaults, explicit desktop YAML preservation and old/new protected service-command recognition. Existing saved-policy JSON and notification regressions pass.

`build.cmd` rebuilt both Windows apps successfully. The built daemon's `validate` passed for the YAML example and both migrated local files, reporting PC shutdown disabled. Built `--help` shows YAML defaults. No monitor, USB polling, Telegram request, service action or shutdown was started by these checks. Live restart/service migration and delivery remain pending; race-tool limitations are unchanged.

Assessment: file-based YAML configuration and offline validation are verified. Existing running processes retain their previous configuration until deliberately restarted. Earlier dated no-config-change/JSON evidence below is historical; code examples are now shown in YAML for current use.

## Purpose and user journey

Mục tiêu: người dùng biết chính xác chính sách đang áp dụng và không vô tình bật tắt nguồn vì cấu hình lỗi.

An administrator edits a local configuration, validates it with the CLI, then restarts the daemon. The desktop shows monitoring and effective policy without exposing secrets.

## Implementation

- Proposed entry points: `internal/config/config.go`, `internal/config/validate.go`.
- Versioned YAML, selected by the user on 2026-11-16. No-argument daemon startup uses `.\config.yaml` from the current directory. Explicit `--config` selects another file. Installed services always receive the absolute path of their protected executable-adjacent copy; console command defaults are independent. No invalid/missing-file fallback is performed. The earlier proposed `UPSmart` directory was never installed by this project.
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
- CFG-02: corrupt YAML, unknown/duplicate keys, multiple documents and invalid timing relationships fail explicitly. Historical non-loopback listener tests no longer apply after web removal.
- CFG-03: enabled output control without a verified capability is rejected.
- CFG-04: an unprivileged user cannot modify the service policy.
- CFG-05: editing the file alone does not change a running policy; restart and the desktop reflect the new active configuration.

## Decisions and boundaries

Go is confirmed. Retention and shutdown timing are explicitly configurable; no active shutdown thresholds were enabled. User-selected controls and activation boundaries are in [F-009](F-009-pc-shutdown.md). The optional [Telegram sender](F-017-telegram-notifications.md) references a token environment variable rather than storing the bot token in configuration.

## Results and next steps

2026-11-12: implemented `internal/config/config.go`. The original schema included `listen`; the current schema no longer has web/listener fields. Unknown keys, including the removed `listen` and `web_enabled` fields, are rejected.

Bounds: schema version 1; poll 1 second-1 minute; transaction timeout 100 ms-30 seconds and less than poll interval; stale 1 second-5 minutes and greater than poll + timeout. Files over 64 KiB, trailing JSON and invalid timing relationships fail explicitly. No hot reload.

Historical tests covered the original loopback binding. Current tests cover valid monitoring-disabled configuration, unknown/removed web fields, wrong version, invalid timing, trailing data and malformed JSON. The example configuration is validated; local configuration is left unchanged.

The repository example disables monitoring. The historical ignored local JSON file enabled monitoring after the user's read-only handover approval and used port 18765 because 8080 was occupied. The YAML migration above removes that retired listener field. No ProgramData configuration or ACLs were installed.

```powershell
# Only when no local configuration exists:
Copy-Item .\config.example.yaml .\config.yaml
# Enable monitoring only after closing legacy UPSmart.
.\bin\upsdaemon.exe validate --config .\config.yaml
.\bin\upsdaemon.exe serve --config .\config.yaml
```

Do not overwrite an existing local file without preserving its settings. Assessment: monitoring-schema validation is working; device selectors, persistent event options, administrator ACLs and future opt-in policies remain pending.

### Persistence/service candidate - 2026-11-12

Optional `event_history` is now supported; omission retains memory-only behavior with no automatic filesystem logging. Example fragment for a future explicitly approved console configuration:

```yaml
event_history:
  directory: 'C:\Users\dung\UPSmart\data\events'
  max_file_bytes: 5242880
  max_files: 5
```

Require an absolute local non-root directory, size 1024-52428800 bytes and count 1-10. UNC paths, relative paths, filesystem roots and invalid limits are rejected. Reducing the count intentionally removes numbered archives beyond the new retention limit. Unknown fields and original timing checks remain enforced.

Service installation always uses the protected `%ProgramData%\UPSmart\logs` directory; it preserves explicit retention limits, or uses 5 MiB x 5 files if omitted. Existing service configuration is never silently overwritten with differing values.

Tests for valid/invalid history options and existing configuration regressions passed. The user selected build-only: neither local/example configuration was changed and logging was not enabled. The fragment above is documentation, not active configuration. Next: obtain deployment approval before adding it; power-policy settings remain absent.

### Optional PC metrics and Telegram candidate - 2026-11-12

Two optional groups are now accepted. Omission disables the respective feature; existing configurations retain their behavior. Example additions, not changes to any active file:

```yaml
hardware:
  interval: 15s
telegram:
  token_env: UPSMART_TELEGRAM_TOKEN
  chat_id: "<destination-chat-id>"
  events: [startup, connected, connection_error, power_changed, low_battery_changed, fault_changed]
```

Hardware interval must be 10 seconds-5 minutes. Telegram settings require a supported nonempty event selection and valid token-variable/destination syntax. Validation does not send a message or require the environment variable to be populated. Application startup resolves the token and fails explicitly before binding if it is missing/invalid.

The token must be supplied locally in the executing account's environment; do not commit it, place it in configuration, or expose it in browser/status/log output. Console environment setup does not automatically configure LocalService; account-specific secret provisioning and actual service delivery remain unverified. Environment variables are not an encrypted credential vault.

Tests passed for both optional groups, combined selection, invalid intervals, missing event selection and attempted inline-secret/unknown fields. The active local/example files remain unchanged.

### Earlier Windows shutdown and Telegram controls - 2026-11-12 (manual transport superseded by F-018)

The previous monitoring-only rejection of shutdown fields is superseded by optional `power_control`. Omission leaves all shutdown controls disabled. Enabled controls require explicit `mode: "dry_run"` or `"live"`; there is no implicit live mode. Unknown control fields, including UPS output-off options, remain invalid.

Documentation-only dry-run example; these timings and illustrative IDs were **not** written to either configuration or activated:

```yaml
power_control:
  mode: dry_run
  automatic:
    enabled: true
    outage_after: 5m
    low_battery: true
    countdown: 30s
  manual:
    enabled: true
    countdown: 30s
    allowed_user_ids: [123456789]
telegram:
  token_env: UPSMART_TELEGRAM_TOKEN
  chat_id: "123456789"
  events: [power_changed, low_battery_changed, shutdown_pending, shutdown_cancelled, shutdown_requested, shutdown_dry_run, shutdown_failed]
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
