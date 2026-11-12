package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"upsmart/internal/config"
	"upsmart/internal/events"
	"upsmart/internal/ups"
)

const (
	readOnlyACL = "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;LS)(A;OICI;GRGX;;;BU)"
	logACL      = "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;LS)"
)

type deployment struct {
	programDir string
	dataDir    string
	logsDir    string
	executable string
	config     string
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
	program = filepath.Join(program, Name)
	data = filepath.Join(data, Name)
	return deployment{program, data, filepath.Join(data, "logs"), filepath.Join(program, "upsmart-monitor.exe"), filepath.Join(data, "config.json")}, nil
}

func install(sourceConfig string) (resultErr error) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("service installation requires an elevated Administrator terminal; nothing was installed")
	}
	cfg, err := config.Load(sourceConfig)
	if err != nil {
		return err
	}
	if cfg.Desktop != nil && cfg.Desktop.UserSID == "" {
		return errors.New("service desktop IPC requires desktop.user_sid for the authorized Windows user")
	}
	release, err := ups.AcquireOwner()
	if err != nil {
		return fmt.Errorf("stop console monitoring before installation: %w", err)
	}
	defer release()
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	existing, err := manager.OpenService(Name)
	if err == nil {
		existing.Close()
		return errors.New("service already exists; stop and uninstall its registration before replacing it")
	}
	if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	paths, err := deploymentPaths()
	if err != nil {
		return err
	}
	if cfg.Desktop != nil && !strings.EqualFold(cfg.Desktop.SettingsFile, filepath.Join(paths.logsDir, "desktop-policy.json")) {
		return errors.New("service desktop.settings_file must be the protected ProgramData UPSmartMonitor logs desktop-policy.json path")
	}
	for _, directory := range []struct{ path, acl string }{
		{paths.programDir, readOnlyACL}, {paths.dataDir, readOnlyACL}, {paths.logsDir, logACL},
	} {
		if err := ensureDirectory(directory.path, directory.acl); err != nil {
			return err
		}
	}
	retention := events.Options{Directory: paths.logsDir, MaxFileBytes: 5 << 20, MaxFiles: 5}
	if cfg.EventHistory != nil {
		retention.MaxFileBytes = cfg.EventHistory.MaxFileBytes
		retention.MaxFiles = cfg.EventHistory.MaxFiles
	}
	cfg.EventHistory = &retention
	if _, err := os.Stat(paths.config); err == nil {
		if err := verifySecurity(paths.config, readOnlyACL); err != nil {
			return err
		}
		existing, err := config.Load(paths.config)
		if err != nil {
			return fmt.Errorf("existing service configuration is invalid; preserved without overwrite: %w", err)
		}
		left, err := json.Marshal(existing)
		if err != nil {
			return err
		}
		right, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		if !bytes.Equal(left, right) {
			return errors.New("existing service configuration differs; review it as Administrator and pass that file explicitly; it was not overwritten")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		if err := writeProtected(paths.config, bytes.NewReader(append(data, '\n')), readOnlyACL, false); err != nil {
			return err
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(executable, paths.executable) {
		input, err := os.Open(executable)
		if err != nil {
			return err
		}
		copyErr := writeProtected(paths.executable, input, readOnlyACL, true)
		closeErr := input.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
	} else if err := verifySecurity(paths.executable, readOnlyACL); err != nil {
		return err
	}
	if err := eventlog.InstallAsEventCreate(Name, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		return fmt.Errorf("register Event Log source (protected deployment files preserved): %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, eventlog.Remove(Name))
		}
	}()
	service, err := manager.CreateService(Name, paths.executable, mgr.Config{
		DisplayName: "UPSmart Monitor", Description: "ARES UPS monitoring, local desktop controls and optional read-only dashboard",
		StartType: mgr.StartAutomatic, DelayedAutoStart: true, ErrorControl: mgr.ErrorNormal,
		ServiceStartName: `NT AUTHORITY\LocalService`,
	}, "service", "run", "--config", paths.config)
	if err != nil {
		return err
	}
	defer service.Close()
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, service.Delete())
		}
	}()
	if err := service.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 30 * time.Second}, {Type: mgr.NoAction},
	}, 86400); err != nil {
		return err
	}
	if err := service.SetRecoveryActionsOnNonCrashFailures(false); err != nil {
		return err
	}
	fmt.Printf("Installed %s as LocalService (not started).\nExecutable: %s\nConfiguration: %s\n", Name, paths.executable, paths.config)
	return nil
}

func ensureDirectory(path, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes := windows.SecurityAttributes{SecurityDescriptor: sd}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	if err := windows.CreateDirectory(name, &attributes); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return fmt.Errorf("create protected directory: %w", err)
	}
	return verifySecurity(path, sddl)
}

func verifySecurity(path, sddl string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(name)
	if err != nil {
		return err
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("refusing deployment reparse point: %s", path)
	}
	actual, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	expected, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	if actual.String() != expected.String() {
		return fmt.Errorf("deployment ACL/owner does not match protected policy: %s; inspect it rather than changing permissions implicitly", path)
	}
	return nil
}

func writeProtected(path string, source io.Reader, sddl string, replace bool) (resultErr error) {
	if _, err := os.Lstat(path); err == nil {
		if !replace {
			return fmt.Errorf("refusing to overwrite %s", path)
		}
		if err := verifySecurity(path, sddl); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp := path + ".new"
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return err
	}
	attributes := windows.SecurityAttributes{SecurityDescriptor: sd}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, &attributes, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), temp)
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
	if err := verifySecurity(temp, sddl); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
