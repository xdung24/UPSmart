package monitor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"upsmart/internal/config"
	"upsmart/internal/control"
	"upsmart/internal/policy"
	"upsmart/internal/ups"
)

func TestDirectDefaultsAndSavedPolicyUseOnlyPerUserStorage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	cfg, err := DirectConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MonitoringEnabled || cfg.Hardware != nil || cfg.Telegram != nil || cfg.PowerControl != nil {
		t.Fatal("direct defaults unexpectedly enabled optional features or disabled USB")
	}
	expected := filepath.Join(root, "UPSmart", "desktop-policy.json")
	if cfg.Desktop.SettingsFile != expected {
		t.Fatal("direct settings not isolated to the user profile")
	}
	if err := control.SavePolicy(expected, policy.Options{Mode: "dry_run", Manual: &policy.ManualOptions{Enabled: true, Countdown: "10s"}}); err != nil {
		t.Fatal(err)
	}
	cfg, err = DirectConfig("")
	if err != nil || cfg.PowerControl == nil || !cfg.PowerControl.ManualEnabled() {
		t.Fatalf("previously saved direct policy not restored: %v", err)
	}
}

func TestExplicitDirectConfigurationIsNotReplacedOrRewritten(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	path := filepath.Join(root, "chosen.yaml")
	const document = `schema_version: 1
poll_interval: 3s
transaction_timeout: 1s
stale_after: 10s
monitoring_enabled: false
`
	if err := os.WriteFile(path, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := DirectConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MonitoringEnabled || cfg.PollInterval != "3s" {
		t.Fatal("explicit direct configuration was overridden")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != document {
		t.Fatal("active configuration was rewritten:", err)
	}
	if _, err := DirectConfig(filepath.Join(root, "missing.yaml")); err == nil {
		t.Fatal("missing explicit configuration silently used USB defaults")
	}
	if err := os.WriteFile(path, []byte("schema_version: 99\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DirectConfig(path); err == nil {
		t.Fatal("invalid explicit configuration silently used USB defaults")
	}
}

func TestExistingOwnerBlocksRuntimeBeforeAnyUSBOrReadiness(t *testing.T) {
	cfg := config.Config{
		SchemaVersion: 1, PollInterval: "2s",
		TransactionTimeout: "1s", StaleAfter: "10s", MonitoringEnabled: true,
	}
	busy := errors.New("another monitor already running")
	calls := 0
	err := run(context.Background(), cfg, ups.Worker{Executable: "must-not-execute"}, func() {
		t.Error("blocked runtime reported readiness")
	}, func() (func(), error) {
		calls++
		return nil, busy
	})
	if !errors.Is(err, busy) || calls != 1 {
		t.Fatal("runtime did not respect existing daemon ownership:", err)
	}
}
