package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func Manage(action, configPath string) error {
	if action == "install" {
		return install(configPath)
	}
	if action != "status" && action != "start" && action != "stop" && action != "uninstall" {
		return fmt.Errorf("unknown service action %q", action)
	}
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer windows.CloseServiceHandle(manager)
	name, err := windows.UTF16PtrFromString(Name)
	if err != nil {
		return err
	}
	access := uint32(windows.SERVICE_QUERY_STATUS | windows.SERVICE_QUERY_CONFIG)
	switch action {
	case "start":
		access |= windows.SERVICE_START
	case "stop":
		access |= windows.SERVICE_STOP
	case "uninstall":
		access |= windows.DELETE
	}
	handle, err := windows.OpenService(manager, name, access)
	if err != nil {
		return fmt.Errorf("open %s (administrative rights are needed for changes): %w", Name, err)
	}
	service := &mgr.Service{Name: Name, Handle: handle}
	defer service.Close()
	cfg, err := service.Config()
	if err != nil {
		return err
	}
	paths, err := deploymentPaths()
	if err != nil {
		return err
	}
	expected := strings.Join([]string{syscall.EscapeArg(paths.executable), "service", "run", "--config", syscall.EscapeArg(paths.config)}, " ")
	if !strings.EqualFold(cfg.BinaryPathName, expected) {
		return errors.New("registered service command differs from this application's protected deployment; refusing management")
	}
	status, err := service.Query()
	if err != nil {
		return err
	}
	switch action {
	case "status":
		return json.NewEncoder(os.Stdout).Encode(struct {
			Name    string    `json:"name"`
			State   svc.State `json:"state"`
			PID     uint32    `json:"pid"`
			Account string    `json:"account"`
			Command string    `json:"command"`
		}{Name, status.State, status.ProcessId, cfg.ServiceStartName, cfg.BinaryPathName})
	case "start":
		if status.State == svc.Running {
			fmt.Println("Service is already running")
			return nil
		}
		if status.State != svc.Stopped {
			return fmt.Errorf("service is transitioning (state %d); wait before starting", status.State)
		}
		if err := service.Start(); err != nil {
			return err
		}
		if err := waitState(service, svc.Running); err != nil {
			return err
		}
		fmt.Println("Service is running")
	case "stop":
		if status.State == svc.Stopped {
			fmt.Println("Service is already stopped")
			return nil
		}
		if status.State != svc.StopPending {
			if _, err := service.Control(svc.Stop); err != nil {
				return err
			}
		}
		if err := waitState(service, svc.Stopped); err != nil {
			return err
		}
		fmt.Println("Service stopped; no PC or UPS power command was sent")
	case "uninstall":
		if status.State != svc.Stopped {
			return errors.New("stop the service before uninstalling; no process will be stopped implicitly")
		}
		if err := service.Delete(); err != nil {
			return err
		}
		if err := eventlog.Remove(Name); err != nil {
			return fmt.Errorf("service deleted but Event Log source removal failed: %w", err)
		}
		fmt.Println("Service registration removed; executable, configuration and history preserved")
	}
	return nil
}

func waitState(service *mgr.Service, target svc.State) error {
	deadline := time.Now().Add(15 * time.Second)
	for {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == target {
			return nil
		}
		if target == svc.Running && status.State == svc.Stopped {
			return fmt.Errorf("service startup failed (Windows exit %d, service exit %d); check Windows Application Event Log", status.Win32ExitCode, status.ServiceSpecificExitCode)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for service state %d (current %d)", target, status.State)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
