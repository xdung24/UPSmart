package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"upsmart/internal/control"
	"upsmart/internal/events"
	"upsmart/internal/notifications"
	"upsmart/internal/policy"
)

const validConfig = `{"schema_version":1,"poll_interval":"2s","transaction_timeout":"1s","stale_after":"10s","monitoring_enabled":false}`

func TestLoadYAMLConfigurationAndRoundTrip(t *testing.T) {
	root := t.TempDir()
	settings := filepath.Join(root, "desktop-policy.json")
	document := fmt.Sprintf(`# Monitoring configuration
schema_version: 1
poll_interval: 3s
transaction_timeout: 1s
stale_after: 10s
monitoring_enabled: true
event_history:
  directory: %q
  max_file_bytes: 4096
  max_files: 3
hardware:
  interval: 15s
telegram:
  token_env: UPSMART_TELEGRAM_TOKEN
  chat_id: "-100123456789"
  allowed_user_ids: [42]
  events:
    - startup
    - power_changed
power_control:
  mode: dry_run
  automatic:
    enabled: true
    outage_after: 5m
    low_battery: true
    countdown: 30s
  manual:
    enabled: true
    countdown: 10s
    allowed_user_ids: [42]
desktop:
  user_sid: S-1-5-21-1-2-3-1001
  settings_file: %q
`, filepath.Join(root, "events"), settings)
	want := Config{
		SchemaVersion: 1, PollInterval: "3s", TransactionTimeout: "1s", StaleAfter: "10s", MonitoringEnabled: true,
		EventHistory: &events.Options{Directory: filepath.Join(root, "events"), MaxFileBytes: 4096, MaxFiles: 3},
		Hardware:     &HardwareOptions{Interval: "15s"},
		Telegram: &notifications.Options{TokenEnv: "UPSMART_TELEGRAM_TOKEN", ChatID: "-100123456789",
			AllowedUserIDs: []int64{42}, Events: []string{"startup", "power_changed"}},
		PowerControl: &policy.Options{Mode: "dry_run",
			Automatic: &policy.AutomaticOptions{Enabled: true, OutageAfter: "5m", LowBattery: true, Countdown: "30s"},
			Manual:    &policy.ManualOptions{Enabled: true, Countdown: "10s", AllowedUserIDs: []int64{42}}},
		Desktop: &control.Options{UserSID: "S-1-5-21-1-2-3-1001", SettingsFile: settings},
	}
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("YAML settings were lost: got=%+v error=%v", got, err)
	}
	data, err := yaml.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := Load(path)
	if err != nil || !reflect.DeepEqual(roundTrip, want) {
		t.Fatalf("YAML serialization changed settings: %+v error=%v", roundTrip, err)
	}
}

func TestYAMLValidationAndRelay(t *testing.T) {
	const base = "schema_version: 1\npoll_interval: 2s\ntransaction_timeout: 1s\nstale_after: 10s\nmonitoring_enabled: false\n"
	for _, test := range []struct {
		name, document string
		valid          bool
	}{
		{"relay", base + "telegram:\n  relay_address: 127.0.0.1:50061\n  events:\n    - startup\n", true},
		{"comments", base + "# no optional features\n", true},
		{"size limit", base + strings.Repeat("#", 65536-len(base)), true},
		{"oversized", base + strings.Repeat("#", 65537-len(base)), false},
		{"unknown nested field", base + "telegram:\n  relay_adress: localhost:50061\n  events: [startup]\n", false},
		{"unknown policy field", base + "power_control:\n  mode: dry_run\n  automatic:\n    enabled: false\n    count_down: 30s\n", false},
		{"duplicate root key", base + "monitoring_enabled: true\n", false},
		{"duplicate nested key", base + "hardware:\n  interval: 15s\n  interval: 30s\n", false},
		{"second document", base + "---\nschema_version: 1\n", false},
		{"empty second document", base + "---\n", false},
		{"sequence", "- schema_version: 1\n", false},
		{"scalar", "not a configuration\n", false},
		{"empty", "", false},
		{"invalid boolean", strings.Replace(base, "false", "invalid", 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(test.document), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t error=%v", test.valid, err)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	for name, text := range map[string]string{
		"valid":             validConfig,
		"unknown":           strings.Replace(validConfig, `"monitoring_enabled"`, `"shutdown_enabled"`, 1),
		"legacy web listen": strings.Replace(validConfig, `"poll_interval"`, `"listen":"127.0.0.1:8080","poll_interval"`, 1),
		"legacy web toggle": strings.Replace(validConfig, `"poll_interval"`, `"web_enabled":false,"poll_interval"`, 1),
		"version":           strings.Replace(validConfig, `"schema_version":1`, `"schema_version":2`, 1),
		"overlap":           strings.Replace(validConfig, `"1s"`, `"2s"`, 1),
		"premature stale":   strings.Replace(validConfig, `"10s"`, `"3s"`, 1),
		"zero interval":     strings.Replace(validConfig, `"2s"`, `"0s"`, 1),
		"trailing data":     validConfig + `{}`,
		"malformed":         `{`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}

			cfg, err := Load(path)
			if name == "valid" {
				if err != nil || cfg.MonitoringEnabled {
					t.Fatalf("expected monitoring disabled: %+v, %v", cfg, err)
				}
			} else if err == nil {
				t.Fatal("expected invalid configuration")
			}
		})
	}
}

func TestEventHistoryValidation(t *testing.T) {
	base := Config{SchemaVersion: 1, PollInterval: "2s", TransactionTimeout: "1s", StaleAfter: "10s"}
	valid := events.Options{Directory: t.TempDir(), MaxFileBytes: 5 << 20, MaxFiles: 5}
	base.EventHistory = &valid
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}

	for _, options := range []events.Options{
		{Directory: "relative", MaxFileBytes: 1024, MaxFiles: 1},
		{Directory: filepath.VolumeName(valid.Directory) + string(os.PathSeparator), MaxFileBytes: 1024, MaxFiles: 1},
		{Directory: valid.Directory, MaxFileBytes: 1023, MaxFiles: 1},
		{Directory: valid.Directory, MaxFileBytes: 1024, MaxFiles: 0},
		{Directory: valid.Directory, MaxFileBytes: 1024, MaxFiles: 11},
	} {
		base.EventHistory = &options
		if err := base.Validate(); err == nil {
			t.Fatalf("invalid history options accepted: %+v", options)
		}
	}
}

func TestOptionalMonitorSettings(t *testing.T) {
	for _, test := range []struct {
		name, extra string
		valid       bool
	}{
		{"hardware", `"hardware":{"interval":"15s"}`, true},
		{"hardware too frequent", `"hardware":{"interval":"1s"}`, false},
		{"hardware missing interval", `"hardware":{}`, false},
		{"telegram", `"telegram":{"token_env":"UPSMART_TELEGRAM_TOKEN","chat_id":"12345","events":["startup","power_changed"]}`, true},
		{"telegram relay", `"telegram":{"relay_address":"127.0.0.1:50061","events":["startup","power_changed"]}`, true},
		{"telegram relay IPv6", `"telegram":{"relay_address":"[::1]:50061","events":["startup"]}`, true},
		{"telegram relay hostname", `"telegram":{"relay_address":"watcher.local:50061","events":["startup"]}`, true},
		{"relay missing host", `"telegram":{"relay_address":":50061","events":["startup"]}`, false},
		{"relay invalid port", `"telegram":{"relay_address":"localhost:65536","events":["startup"]}`, false},
		{"relay URL", `"telegram":{"relay_address":"http://localhost:50061","events":["startup"]}`, false},
		{"relay token conflict", `"telegram":{"relay_address":"localhost:50061","token_env":"BOT_TOKEN","events":["startup"]}`, false},
		{"relay chat conflict", `"telegram":{"relay_address":"localhost:50061","chat_id":"12345","events":["startup"]}`, false},
		{"relay commands conflict", `"telegram":{"relay_address":"localhost:50061","allowed_user_ids":[42],"events":["startup"]}`, false},
		{"relay missing events", `"telegram":{"relay_address":"localhost:50061","events":[]}`, false},
		{"telegram no events", `"telegram":{"token_env":"UPSMART_TELEGRAM_TOKEN","chat_id":"12345","events":[]}`, false},
		{"telegram secret in config", `"telegram":{"token":"secret","chat_id":"12345","events":["startup"]}`, false},
		{"both", `"hardware":{"interval":"15s"},"telegram":{"token_env":"UPSMART_TELEGRAM_TOKEN","chat_id":"12345","events":["connected"]}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			text := strings.TrimSuffix(validConfig, "}") + "," + test.extra + "}"
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}

			_, err := Load(path)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t, error=%v", test.valid, err)
			}
		})
	}
}

func TestPowerControlRequiresExplicitModeTriggersAndDesktopAuthorization(t *testing.T) {
	desktop := `"desktop":{"settings_file":` + strconv.Quote(filepath.Join(t.TempDir(), "policy.json")) + `}`
	manual := `"manual":{"enabled":true,"countdown":"30s","allowed_user_ids":[42]}`
	automatic := `"automatic":{"enabled":true,"outage_after":"5m","low_battery":true,"countdown":"30s"}`
	for _, test := range []struct {
		name, extra string
		monitoring  bool
		valid       bool
	}{
		{"automatic dry run", `"power_control":{"mode":"dry_run",` + automatic + `}`, true, true},
		{"automatic without monitoring", `"power_control":{"mode":"live",` + automatic + `}`, false, false},
		{"missing mode", `"power_control":{` + automatic + `}`, true, false},
		{"missing trigger", `"power_control":{"mode":"dry_run","automatic":{"enabled":true,"countdown":"30s"}}`, true, false},
		{"manual without desktop IPC", `"power_control":{"mode":"dry_run",` + manual + `}`, false, false},
		{"manual desktop dry run", desktop + `,"power_control":{"mode":"dry_run",` + manual + `}`, false, true},
		{"manual desktop live", desktop + `,"power_control":{"mode":"live",` + manual + `}`, false, true},
		{"desktop only", desktop, false, true},
		{"unknown control", `"power_control":{"mode":"dry_run","ups_power_off":true}`, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := validConfig
			if test.monitoring {
				base = strings.Replace(base, `"monitoring_enabled":false`, `"monitoring_enabled":true`, 1)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(strings.TrimSuffix(base, "}")+","+test.extra+"}"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t error=%v", test.valid, err)
			}
		})
	}
}
