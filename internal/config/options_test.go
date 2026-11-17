package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFlatParametersDefaultsFileCLIAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chosen.yaml")
	data := `poll-interval: 3s
monitoring-enabled: true
notify-addr: 127.0.0.1:50061
telegram-events: [startup]
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][]string{{"-c", path}, {"--conf=" + path}, {"--config", path}} {
		var output bytes.Buffer
		args := append(selection, "--poll-interval=4s", "--monitoring-enabled=false", "--telegram-events=connected")
		options, err := Parse(args, &output)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := options.Parameters.Config().Effective()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.SchemaVersion != 1 || cfg.PollInterval != "4s" || cfg.TransactionTimeout != "1s" ||
			cfg.MonitoringEnabled || cfg.Telegram.RelayAddress != "127.0.0.1:50061" ||
			!reflect.DeepEqual(cfg.Telegram.Events, []string{"startup", "connected"}) {
			t.Fatalf("precedence or repeatable parameters lost: %+v", cfg)
		}
		serialized, err := Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(serialized, []byte("poll_interval")) {
			t.Fatal("serialization used the retired nested schema")
		}
		roundTrip := filepath.Join(t.TempDir(), "installed.yaml")
		if err := os.WriteFile(roundTrip, serialized, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(roundTrip)
		if err != nil || !reflect.DeepEqual(got, cfg) {
			t.Fatalf("flat configuration round-trip lost settings: %+v, %v", got, err)
		}
	}
}

func TestFlatConfigurationValidationAndManagementIsolation(t *testing.T) {
	for _, document := range []string{
		"poll-interval: 2s\npoll-interval: 3s\n",
		"notify-adrr: localhost:50061\n",
		"install: true\n",
		"monitoring-enabled: invalid\n",
		"telegram-events:\n  nested: startup\n",
		"monitoring-enabled: null\n",
		"poll-interval: 2s\n---\n",
		"{}\n---\npoll-interval: 2s\n",
		"[]\n",
		strings.Repeat("#", 65537),
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(document), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Parse([]string{"-c", path}, &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid configuration accepted: %.100q", document)
		}
		options, err := Parse([]string{"--stop", "-c", path}, &bytes.Buffer{})
		if err != nil || !options.Stop {
			t.Fatalf("broken app config blocked management: %v", err)
		}
	}
	if _, err := Parse([]string{"--install", "--remove"}, &bytes.Buffer{}); err == nil {
		t.Fatal("conflicting actions accepted")
	}
	if _, err := Parse([]string{"--config="}, &bytes.Buffer{}); err == nil {
		t.Fatal("empty explicit configuration path accepted")
	}
}

func TestMissingConfigurationHasExplicitSafeDefaults(t *testing.T) {
	var output bytes.Buffer
	path := filepath.Join(t.TempDir(), "missing.yaml")
	options, err := Parse([]string{"--config", path}, &output)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := options.Parameters.Config().Effective()
	if err != nil || cfg.MonitoringEnabled || cfg.PowerControl != nil || cfg.Telegram != nil || output.Len() == 0 {
		t.Fatalf("missing configuration was unsafe or silent: %+v %v %q", cfg, err, output.String())
	}
	options, err = Parse([]string{"--config", path, "--monitoring-enabled"}, &output)
	if err != nil || !options.Parameters.Config().MonitoringEnabled {
		t.Fatalf("bare boolean flag did not enable the requested option: %v", err)
	}
}
