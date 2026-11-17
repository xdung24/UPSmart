package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"upsmart/internal/config"
	"upsmart/internal/control"
	"upsmart/internal/events"
)

type deployment struct {
	programDir string
	dataDir    string
	logsDir    string
	executable string
}

func deploymentPaths() (deployment, error) {
	program, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return deployment{}, err
	}
	data, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return deployment{}, err
	}
	program, data = filepath.Join(program, Name), filepath.Join(data, Name)
	return deployment{program, data, filepath.Join(data, "logs"), filepath.Join(program, "upsdaemon.exe")}, nil
}

func installedConfigPath(paths deployment, source string) (string, error) {
	path := filepath.Join(paths.programDir, filepath.Base(source))
	if strings.EqualFold(path, paths.executable) {
		return "", errors.New("configuration filename must differ from upsdaemon.exe")
	}
	return path, nil
}

func serviceConfiguration(cfg config.Config, paths deployment) (config.Config, error) {
	settingsPath := filepath.Join(paths.logsDir, "desktop-policy.json")
	desktop := control.Options{SettingsFile: settingsPath}
	if cfg.Desktop != nil {
		desktop = *cfg.Desktop
	} else {
		saved, err := control.LoadPolicy(settingsPath)
		if err != nil {
			return cfg, err
		}
		if saved != nil {
			return cfg, errors.New("saved service power policy already exists; review it and explicitly select desktop-settings-file before enabling desktop IPC")
		}
	}
	if desktop.UserSID == "" {
		userSID, err := control.CurrentUserSID()
		if err != nil {
			return cfg, fmt.Errorf("resolve installing Windows account for desktop IPC: %w", err)
		}
		desktop.UserSID = userSID
	}
	if !strings.EqualFold(desktop.SettingsFile, settingsPath) {
		return cfg, errors.New("service desktop-settings-file must be the ProgramData UPSmart logs desktop-policy.json path")
	}
	cfg.Desktop = &desktop
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	retention := events.Options{Directory: paths.logsDir, MaxFileBytes: 5 << 20, MaxFiles: 5}
	if cfg.EventHistory != nil {
		retention.MaxFileBytes, retention.MaxFiles = cfg.EventHistory.MaxFileBytes, cfg.EventHistory.MaxFiles
	}
	cfg.EventHistory = &retention
	return cfg.Effective()
}

func deployFiles(paths deployment, sourceExecutable, configPath string, cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	if len(data) > 65536 {
		return errors.New("generated service configuration exceeds 64 KiB")
	}
	for _, directory := range []string{paths.programDir, paths.dataDir, paths.logsDir} {
		if err := rejectReparse(directory); err != nil {
			return err
		}
		if err := os.MkdirAll(directory, 0755); err != nil {
			return err
		}
	}
	if !strings.EqualFold(sourceExecutable, paths.executable) {
		input, err := os.Open(sourceExecutable)
		if err != nil {
			return err
		}
		copyErr := writeAtomic(paths.executable, input)
		if err := errors.Join(copyErr, input.Close()); err != nil {
			return fmt.Errorf("copy installed executable: %w", err)
		}
	}
	// Persist overrides too; copying the source verbatim would lose CLI-selected settings.
	if err := writeAtomic(configPath, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("write installed configuration: %w", err)
	}
	return nil
}

func rejectReparse(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(name)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("refusing deployment reparse point: %s", path)
	}
	return nil
}

func writeAtomic(path string, source io.Reader) (resultErr error) {
	if err := rejectReparse(path); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".upsmart-install-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer func() {
		if err := os.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	_, copyErr := io.Copy(file, source)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
