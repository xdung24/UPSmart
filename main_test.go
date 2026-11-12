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

const launchConfiguration = `{"schema_version":1,"listen":"127.0.0.1:8080","poll_interval":"2s","transaction_timeout":"1s","stale_after":"10s","monitoring_enabled":false}`

func TestServeRejectsUnsafeBindingBeforeStarting(t *testing.T) {
	cfg := config.Config{SchemaVersion: 1, Listen: "0.0.0.0:8080", PollInterval: "2s", TransactionTimeout: "1s", StaleAfter: "10s"}
	err := serve(context.Background(), cfg, ups.Worker{}, nil)
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("unsafe binding was not rejected before startup: %v", err)
	}
}

func TestServeReportsMissingTelegramTokenBeforeStarting(t *testing.T) {
	t.Setenv("UPSMART_TEST_MISSING_TOKEN", "")
	cfg := config.Config{
		SchemaVersion: 1, Listen: "127.0.0.1:8080", PollInterval: "2s",
		TransactionTimeout: "1s", StaleAfter: "10s",
		Telegram: &notifications.Options{TokenEnv: "UPSMART_TEST_MISSING_TOKEN", ChatID: "12345", Events: []string{"startup"}},
	}
	err := serve(context.Background(), cfg, ups.Worker{}, nil)
	if err == nil || !strings.Contains(err.Error(), "initialize Telegram") {
		t.Fatalf("missing Telegram token was not surfaced before startup: %v", err)
	}
}

func TestNoArgumentsStartsMonitorWithLocalConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.local.json", []byte(launchConfiguration), 0600); err != nil {
		t.Fatal(err)
	}
	called := 0
	finished := errors.New("test monitor finished")
	err := runWithMonitor(nil, func(_ context.Context, cfg config.Config, _ ups.Worker, ready func()) error {
		called++
		if cfg.Listen != "127.0.0.1:8080" || ready != nil {
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
	if err := os.WriteFile("config.local.json", []byte(`{"schema_version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	err := run(nil)
	if err == nil || !strings.Contains(err.Error(), "schema_version must be 1") {
		t.Fatalf("no-argument launch did not load local configuration: %v", err)
	}
}

func TestNoArgumentsReportsMissingLocalConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	err := run(nil)
	if err == nil || !strings.Contains(err.Error(), "open config") || !strings.Contains(err.Error(), "config.local.json") {
		t.Fatalf("missing local configuration did not produce an actionable error: %v", err)
	}
}

func TestExplicitConfigurationIsNotReplacedByDefaultLaunch(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("config.local.json", []byte("invalid local configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "chosen.json")
	if err := os.WriteFile(path, []byte(launchConfiguration), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "--config", path}); err != nil {
		t.Fatalf("explicit command/configuration was changed by default-launch behavior: %v", err)
	}
}
