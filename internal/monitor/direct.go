package monitor

import (
	"fmt"
	"os"
	"path/filepath"

	"upsmart/internal/config"
	"upsmart/internal/control"
)

func DirectConfig(path string) (config.Config, error) {
	cfg := config.Config{
		SchemaVersion: 1,
		PollInterval:  "2s", TransactionTimeout: "1s", StaleAfter: "10s",
		MonitoringEnabled: true,
	}
	if path != "" {
		var err error
		cfg, err = config.Load(path)
		if err != nil {
			return cfg, err
		}
	}
	if cfg.Desktop == nil {
		directory, err := os.UserCacheDir()
		if err != nil {
			return cfg, fmt.Errorf("locate desktop settings directory: %w", err)
		}
		cfg.Desktop = &control.Options{SettingsFile: filepath.Join(directory, "UPSmartMonitor", "desktop-policy.json")}
	}
	effective, err := cfg.Effective()
	if err != nil {
		return cfg, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Desktop.SettingsFile), 0700); err != nil {
		return cfg, fmt.Errorf("create desktop policy directory: %w", err)
	}
	return effective, nil
}
