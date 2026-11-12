package config

import (
	"os"
	"path/filepath"
	"testing"

	"upsmart/internal/control"
	"upsmart/internal/policy"
)

func TestEffectiveSavedPolicyIsValidatedAndNeverHiddenByBaseConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	cfg := Config{SchemaVersion: 1, Listen: "127.0.0.1:8080", PollInterval: "2s", TransactionTimeout: "1s", StaleAfter: "10s",
		Desktop: &control.Options{SettingsFile: path}}
	settings := policy.Options{Mode: "live", Manual: &policy.ManualOptions{Enabled: true, Countdown: "30s"}}
	if err := control.SavePolicy(path, settings); err != nil {
		t.Fatal(err)
	}
	effective, err := cfg.Effective()
	if err != nil || effective.PowerControl == nil || effective.PowerControl.Mode != "live" || !effective.PowerControl.ManualEnabled() {
		t.Fatal("saved live policy was hidden by monitoring-only base configuration", err)
	}
	settings.Automatic = &policy.AutomaticOptions{Enabled: true, LowBattery: true, Countdown: "5s"}
	if err := control.SavePolicy(path, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Effective(); err == nil {
		t.Fatal("saved automatic policy bypassed monitoring dependency")
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Effective(); err == nil {
		t.Fatal("corrupt saved policy silently fell back to disabled")
	}
}
