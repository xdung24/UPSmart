package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"upsmart/internal/config"
	"upsmart/internal/notifications"
	"upsmart/internal/ups"
)

const launchConfiguration = `schema_version: 1
poll_interval: 2s
transaction_timeout: 1s
stale_after: 10s
monitoring_enabled: false
`

func TestServeReportsMissingTelegramTokenBeforeStarting(t *testing.T) {
	t.Setenv("upsdaemon_TEST_MISSING_TOKEN", "")
	cfg := config.Config{
		SchemaVersion: 1, PollInterval: "2s",
		TransactionTimeout: "1s", StaleAfter: "10s",
		Telegram: &notifications.Options{TokenEnv: "upsdaemon_TEST_MISSING_TOKEN", ChatID: "12345", Events: []string{"startup"}},
	}
	err := serve(context.Background(), cfg, ups.Worker{}, nil)
	if err == nil || !strings.Contains(err.Error(), "initialize Telegram") {
		t.Fatalf("missing Telegram token was not surfaced before startup: %v", err)
	}
}

func TestNoArgumentsStartsMonitorWithLocalConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.yaml", []byte(launchConfiguration), 0600); err != nil {
		t.Fatal(err)
	}
	called := 0
	finished := errors.New("test monitor finished")
	err := runWithMonitor(nil, func(_ context.Context, cfg config.Config, _ ups.Worker, ready func()) error {
		called++
		if cfg.MonitoringEnabled || ready != nil {
			t.Fatal("default launch did not select the local console configuration")
		}
		return finished
	})
	if !errors.Is(err, finished) || called != 1 {
		t.Fatalf("default launch did not invoke and return from monitor: calls=%d error=%v", called, err)
	}
}

func TestNoArgumentsLoadsLocalConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.yaml", []byte("schema_version: 99\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := run(nil)
	if err == nil || !strings.Contains(err.Error(), "schema_version must be 1") {
		t.Fatalf("no-argument launch did not load local configuration: %v", err)
	}
}

func TestNoArgumentsUsesSafeDefaultsWithoutLocalConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	called := false
	err := runWithMonitor(nil, func(_ context.Context, cfg config.Config, _ ups.Worker, _ func()) error {
		called = true
		if cfg.MonitoringEnabled || cfg.PowerControl != nil || cfg.Telegram != nil {
			t.Fatalf("unsafe defaults: %+v", cfg)
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("safe default launch failed: called=%t error=%v", called, err)
	}
}

func TestExplicitConfigurationIsNotReplacedByDefaultLaunch(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("config.yaml", []byte("invalid local configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "chosen.yaml")
	if err := os.WriteFile(path, []byte(launchConfiguration), 0600); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"validate", "--config", path}); err != nil {
		t.Fatalf("explicit command/configuration was changed by default-launch behavior: %v", err)
	}
}

func TestExplicitCommandsUseTheSameLocalYAMLDefault(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ProgramData", root)
	if err := os.WriteFile("config.yaml", []byte(launchConfiguration), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate"}); err != nil {
		t.Fatal("validate did not select working-directory YAML:", err)
	}
}

func TestDirectFlagsOverrideYAMLBeforeMonitoring(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.yaml", []byte("monitoring-enabled: true\npoll-interval: 3s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	err := runWithMonitor([]string{"--monitoring-enabled=false", "--poll-interval", "4s"},
		func(_ context.Context, cfg config.Config, _ ups.Worker, _ func()) error {
			called = true
			if cfg.MonitoringEnabled || cfg.PollInterval != "4s" {
				t.Fatalf("CLI overrides lost: %+v", cfg)
			}
			return nil
		})
	if err != nil || !called {
		t.Fatalf("direct flag launch failed: called=%t error=%v", called, err)
	}
}
