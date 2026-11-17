package service

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"upsmart/internal/config"
	"upsmart/internal/control"
	"upsmart/internal/events"
	"upsmart/internal/policy"
)

func testDeployment(t *testing.T) deployment {
	t.Helper()
	root := t.TempDir()
	program, data := filepath.Join(root, "Program Files", Name), filepath.Join(root, "ProgramData", Name)
	return deployment{program, data, filepath.Join(data, "logs"), filepath.Join(program, "upsdaemon.exe")}
}

func TestServiceDeploymentPersistsEffectiveConfigurationBesideBinary(t *testing.T) {
	paths := testDeployment(t)
	source := filepath.Join(t.TempDir(), "portable.exe")
	if err := os.WriteFile(source, []byte("executable"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := installedConfigPath(paths, filepath.Join(t.TempDir(), "custom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := serviceConfiguration(config.Config{
		SchemaVersion: 1, PollInterval: "4s", TransactionTimeout: "1s", StaleAfter: "10s",
	}, paths)
	if err != nil {
		t.Fatal(err)
	}
	userSID, err := control.CurrentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Desktop == nil || cfg.Desktop.UserSID != userSID ||
		cfg.Desktop.SettingsFile != filepath.Join(paths.logsDir, "desktop-policy.json") || cfg.PowerControl != nil {
		t.Fatal("installation did not enable desktop IPC for the installing account with power controls disabled")
	}
	for _, interval := range []string{"4s", "5s"} {
		cfg.PollInterval = interval
		if err := deployFiles(paths, source, path, cfg); err != nil {
			t.Fatal(err)
		}
		installed, err := config.Load(path)
		if err != nil || !reflect.DeepEqual(installed, cfg) {
			t.Fatalf("installed settings differ from effective options: %+v %v", installed, err)
		}
	}
	if filepath.Dir(path) != filepath.Dir(paths.executable) || filepath.Base(path) != "custom.yaml" {
		t.Fatalf("configuration was not co-located: %s", path)
	}
	data, err := os.ReadFile(paths.executable)
	if err != nil || string(data) != "executable" {
		t.Fatalf("executable copy failed: %q %v", data, err)
	}
	def := definition(paths, path)
	if def.UserName != "" || !reflect.DeepEqual(def.Arguments, []string{"--config", path}) ||
		def.WorkingDirectory != paths.programDir || def.Executable != paths.executable {
		t.Fatalf("service does not follow the selected LocalSystem/config contract: %+v", def)
	}
}

func TestInvalidServiceConfigurationDoesNotWriteDeployment(t *testing.T) {
	paths := testDeployment(t)
	if err := deployFiles(paths, "", filepath.Join(paths.programDir, "config.yaml"), config.Config{}); err == nil {
		t.Fatal("invalid configuration was deployed")
	}
	if _, err := os.Stat(paths.programDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid settings created a deployment directory")
	}
	if _, err := installedConfigPath(paths, "upsdaemon.exe"); err == nil {
		t.Fatal("configuration can overwrite the installed executable")
	}
	_, err := serviceConfiguration(config.Config{
		SchemaVersion: 1, PollInterval: "2s", TransactionTimeout: "1s", StaleAfter: "10s",
		EventHistory: &events.Options{Directory: "relative", MaxFileBytes: 1024, MaxFiles: 1},
	}, paths)
	if err == nil {
		t.Fatal("service path normalization hid invalid source configuration")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("copy failed") }

func TestAtomicDeploymentPreservesExistingFileOnCopyFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeAtomic(path, io.MultiReader(strings.NewReader("partial"), failingReader{})); err == nil {
		t.Fatal("copy failure was hidden")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original" {
		t.Fatalf("existing file was corrupted: %q %v", got, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary deployment file was not cleaned: %v %v", entries, err)
	}
}

func TestDeploymentRejectsReparseTargets(t *testing.T) {
	root := t.TempDir()
	target, link := filepath.Join(root, "target.yaml"), filepath.Join(root, "link.yaml")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
			t.Skip("symbolic links need Developer Mode or elevation")
		}
		t.Fatal(err)
	}
	if err := writeAtomic(link, strings.NewReader("replacement")); err == nil {
		t.Fatal("deployment followed a reparse point")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "original" {
		t.Fatalf("reparse target was changed: %q %v", got, err)
	}
}

func TestServiceDesktopAuthorizationPreservesExplicitAccount(t *testing.T) {
	paths := testDeployment(t)
	explicit := &control.Options{
		UserSID: "S-1-5-21-1-2-3-1001", SettingsFile: filepath.Join(paths.logsDir, "desktop-policy.json"),
	}
	cfg, err := serviceConfiguration(config.Config{
		SchemaVersion: 1, PollInterval: "2s", TransactionTimeout: "1s", StaleAfter: "10s", Desktop: explicit,
	}, paths)
	if err != nil || !reflect.DeepEqual(cfg.Desktop, explicit) {
		t.Fatalf("explicit desktop account changed: %v", err)
	}
}

func TestAutomaticDesktopSetupCannotActivateExistingSavedPowerPolicy(t *testing.T) {
	paths := testDeployment(t)
	if err := os.MkdirAll(paths.logsDir, 0700); err != nil {
		t.Fatal(err)
	}
	saved := policy.Options{Mode: "live", Manual: &policy.ManualOptions{Enabled: true, Countdown: "30s"}}
	if err := control.SavePolicy(filepath.Join(paths.logsDir, "desktop-policy.json"), saved); err != nil {
		t.Fatal(err)
	}
	_, err := serviceConfiguration(config.Config{
		SchemaVersion: 1, PollInterval: "2s", TransactionTimeout: "1s", StaleAfter: "10s",
	}, paths)
	if err == nil || !strings.Contains(err.Error(), "saved service power policy already exists") {
		t.Fatalf("automatic desktop setup did not require review of existing policy: %v", err)
	}
}

func TestServiceDesktopDefaultsPrecedeManualPolicyValidation(t *testing.T) {
	cfg, err := serviceConfiguration(config.Config{
		SchemaVersion: 1, PollInterval: "2s", TransactionTimeout: "1s", StaleAfter: "10s",
		PowerControl: &policy.Options{Mode: "dry_run", Manual: &policy.ManualOptions{Enabled: true, Countdown: "30s"}},
	}, testDeployment(t))
	if err != nil || cfg.Desktop == nil || !cfg.PowerControl.ManualEnabled() || cfg.PowerControl.Mode != "dry_run" {
		t.Fatalf("service desktop defaults were not applied before validating an explicit manual policy: %v", err)
	}
}
