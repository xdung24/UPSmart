package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"upsmart/internal/config"
	"upsmart/internal/control"
)

func TestWebAddressDiscoveryUsesExplicitAdjacentAndWorkingConfigs(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(bin, "desktop.exe")
	write := func(path, listen string, disabled bool) {
		t.Helper()
		cfg := config.Config{
			SchemaVersion: 1, Listen: listen, PollInterval: "2s",
			TransactionTimeout: "1s", StaleAfter: "10s",
		}
		if disabled {
			web := false
			cfg.Web = &web
			cfg.Desktop = &control.Options{SettingsFile: filepath.Join(root, "policy.json")}
		}
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(path, want string) {
		t.Helper()
		got, err := WebAddress(path, executable)
		if err != nil || got != want {
			t.Fatalf("discovery got %q, want %q: %v", got, want, err)
		}
	}
	check("", "127.0.0.1:8080")
	write("config.local.json", "127.0.0.1:18765", false)
	check("", "127.0.0.1:18765")
	adjacent := filepath.Join(bin, "config.local.json")
	write(adjacent, "127.0.0.1:18888", false)
	check("", "127.0.0.1:18888")
	write("chosen.json", "127.0.0.1:19999", false)
	check("chosen.json", "127.0.0.1:19999")
	write("chosen.json", "127.0.0.1:19999", true)
	check("chosen.json", "")
	if _, err := WebAddress("missing.json", executable); err == nil {
		t.Fatal("missing explicit config silently selected another address")
	}
	if err := os.WriteFile(adjacent, []byte(`{"schema_version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := WebAddress("", executable); err == nil {
		t.Fatal("invalid adjacent config silently selected another address")
	}
}
