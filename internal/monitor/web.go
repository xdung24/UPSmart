package monitor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"upsmart/internal/config"
)

func WebAddress(configPath, executable string) (string, error) {
	load := func(path string) (string, error) {
		cfg, err := config.Load(path)
		if err != nil {
			return "", err
		}
		if !cfg.WebEnabled() {
			return "", nil
		}
		return cfg.Listen, nil
	}
	if configPath != "" {
		return load(configPath)
	}
	for _, path := range []string{filepath.Join(filepath.Dir(executable), "config.local.json"), "config.local.json"} {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return "", fmt.Errorf("inspect web discovery config: %w", err)
		}
		return load(path)
	}
	return "127.0.0.1:8080", nil
}
