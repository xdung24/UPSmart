package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	systemservice "github.com/kardianos/service"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"upsmart/internal/config"
	"upsmart/internal/ups"
)

func LogError(err error) error {
	sink, logErr := eventlog.Open(Name)
	if logErr != nil {
		return logErr
	}
	return errors.Join(sink.Error(1, err.Error()), sink.Close())
}

func definition(paths deployment, configPath string) *systemservice.Config {
	return &systemservice.Config{
		Name: Name, DisplayName: "UPSmart Monitor",
		Description: "ARES UPS monitoring with native desktop controls",
		Executable:  paths.executable, Arguments: []string{"--config", configPath},
		WorkingDirectory: filepath.Dir(configPath),
	}
}

func Manage(action, sourceConfig string, cfg config.Config) error {
	if action != "install" && action != "remove" && action != "start" && action != "stop" && action != "status" {
		return fmt.Errorf("unknown service action %q", action)
	}
	if action != "status" && !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("service changes require an elevated Administrator terminal; nothing was changed")
	}
	paths, err := deploymentPaths()
	if err != nil {
		return err
	}
	configPath, err := installedConfigPath(paths, sourceConfig)
	if err != nil {
		return err
	}
	system, err := systemservice.New(&program{}, definition(paths, configPath))
	if err != nil {
		return err
	}
	switch action {
	case "install":
		if _, sourceErr := os.Stat(sourceConfig); errors.Is(sourceErr, os.ErrNotExist) {
			if _, installedErr := os.Stat(configPath); installedErr == nil {
				return fmt.Errorf("source configuration is missing but installed settings exist; pass --config %q explicitly to preserve or override them", configPath)
			} else if !errors.Is(installedErr, os.ErrNotExist) {
				return installedErr
			}
		} else if sourceErr != nil {
			return sourceErr
		}
		cfg, err = serviceConfiguration(cfg, paths)
		if err != nil {
			return err
		}
		if _, err := system.Status(); err == nil {
			return errors.New("service already exists; stop and remove its registration before installing again")
		} else if !errors.Is(err, systemservice.ErrNotInstalled) {
			return fmt.Errorf("inspect existing service: %w", err)
		}
		release, err := ups.AcquireOwner()
		if err != nil {
			return fmt.Errorf("stop console monitoring before installation: %w", err)
		}
		defer release()
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		if err := deployFiles(paths, executable, configPath, cfg); err != nil {
			return err
		}
		if err := systemservice.Control(system, "install"); err != nil {
			return fmt.Errorf("register service (deployment files preserved): %w", err)
		}
		fmt.Printf("Installed %s as LocalSystem (not started).\nExecutable: %s\nConfiguration: %s\n", Name, paths.executable, configPath)
		fmt.Println("Desktop IPC configured for the authorized Windows account; the desktop does not need Administrator rights.")
	case "remove":
		state, err := queryStatus()
		if err != nil {
			return err
		}
		if state.State != svc.Stopped {
			return errors.New("stop the service with --stop before --remove; no process will be stopped implicitly")
		}
		if err := systemservice.Control(system, "uninstall"); err != nil {
			return err
		}
		if err := os.Remove(paths.executable); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("service removed but installed binary removal failed (run --remove from the portable binary): %w", err)
		}
		fmt.Println("Service and installed binary removed; configuration and history preserved")
	case "status":
		state, err := queryStatus()
		if err != nil {
			return err
		}
		label, known := map[svc.State]string{
			svc.Running: "running", svc.Stopped: "stopped",
			svc.StartPending: "starting", svc.StopPending: "stopping",
			svc.Paused: "paused", svc.PausePending: "pausing", svc.ContinuePending: "continuing",
		}[state.State]
		if !known {
			return fmt.Errorf("unknown service state: %d", state.State)
		}
		fmt.Printf("%s: %s (PID %d)\n", Name, label, state.ProcessId)
	default:
		if err := systemservice.Control(system, action); err != nil {
			return err
		}
		fmt.Printf("Service %s request completed\n", action)
	}
	return nil
}

func queryStatus() (svc.Status, error) {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return svc.Status{}, err
	}
	defer windows.CloseServiceHandle(manager)
	name, err := windows.UTF16PtrFromString(Name)
	if err != nil {
		return svc.Status{}, err
	}
	handle, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return svc.Status{}, err
	}
	service := &mgr.Service{Handle: handle, Name: Name}
	defer service.Close()
	return service.Query()
}
