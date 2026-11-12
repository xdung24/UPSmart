package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"upsmart/internal/events"
)

const validConfig = `{"schema_version":1,"listen":"127.0.0.1:8080","poll_interval":"2s","transaction_timeout":"1s","stale_after":"10s","monitoring_enabled":false}`

func TestLoadConfig(t *testing.T) {
	for name, text := range map[string]string{
		"valid":           validConfig,
		"unknown":         strings.Replace(validConfig, `"monitoring_enabled"`, `"shutdown_enabled"`, 1),
		"LAN":             strings.Replace(validConfig, "127.0.0.1", "0.0.0.0", 1),
		"version":         strings.Replace(validConfig, `"schema_version":1`, `"schema_version":2`, 1),
		"overlap":         strings.Replace(validConfig, `"1s"`, `"2s"`, 1),
		"premature stale": strings.Replace(validConfig, `"10s"`, `"3s"`, 1),
		"zero interval":   strings.Replace(validConfig, `"2s"`, `"0s"`, 1),
		"trailing data":   validConfig + `{}`,
		"malformed":       `{`,
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
	base := Config{SchemaVersion: 1, Listen: "127.0.0.1:8080", PollInterval: "2s", TransactionTimeout: "1s", StaleAfter: "10s"}
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
		{"desktop only", desktop + `,"web_enabled":false`, false, true},
		{"no control surface", `"web_enabled":false`, false, false},
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
