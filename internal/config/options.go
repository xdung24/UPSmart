package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thought-machine/go-flags"
	"gopkg.in/yaml.v3"

	"upsmart/internal/control"
	"upsmart/internal/events"
	"upsmart/internal/notifications"
	"upsmart/internal/policy"
)

type Switch bool

func (s *Switch) UnmarshalFlag(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	*s = Switch(parsed)
	return nil
}

type Parameters struct {
	SchemaVersion      int    `long:"schema-version" default:"1" yaml:"schema-version" description:"Configuration schema version"`
	PollInterval       string `long:"poll-interval" default:"2s" yaml:"poll-interval" description:"UPS polling interval"`
	TransactionTimeout string `long:"transaction-timeout" default:"1s" yaml:"transaction-timeout" description:"UPS transaction deadline"`
	StaleAfter         string `long:"stale-after" default:"10s" yaml:"stale-after" description:"Reading freshness limit"`
	MonitoringEnabled  Switch `long:"monitoring-enabled" optional:"true" optional-value:"true" yaml:"monitoring-enabled" description:"Enable UPS polling (accepts =false)"`

	EventDirectory    string `long:"event-history-directory" yaml:"event-history-directory,omitempty" description:"Absolute event history directory; omission disables persistence"`
	EventMaxFileBytes *int64 `long:"event-history-max-file-bytes" yaml:"event-history-max-file-bytes,omitempty" description:"Event file size limit (default 5242880 when persistence is enabled)"`
	EventMaxFiles     *int   `long:"event-history-max-files" yaml:"event-history-max-files,omitempty" description:"Retained event files (default 5 when persistence is enabled)"`
	HardwareInterval  string `long:"hardware-interval" yaml:"hardware-interval,omitempty" description:"Enable PC metrics with this sampling interval"`

	NotifyAddress   string   `long:"notify-addr" yaml:"notify-addr,omitempty" description:"rdpwatcher notification relay host:port"`
	TelegramToken   string   `long:"telegram-token-env" yaml:"telegram-token-env,omitempty" description:"Direct Telegram token environment variable name, never a token"`
	TelegramChat    string   `long:"telegram-chat-id" yaml:"telegram-chat-id,omitempty" description:"Direct Telegram destination"`
	TelegramEvents  []string `long:"telegram-events" yaml:"telegram-events,omitempty" description:"Notification event; repeat for multiple events"`
	TelegramUsers   []int64  `long:"telegram-allowed-user-ids" yaml:"telegram-allowed-user-ids,omitempty" description:"Checking-only Telegram user ID; repeat for multiple users"`
	PowerMode       string   `long:"power-mode" yaml:"power-mode,omitempty" description:"Explicit dry_run or live mode for enabled PC shutdown"`
	Automatic       Switch   `long:"automatic-shutdown" optional:"true" optional-value:"true" yaml:"automatic-shutdown,omitempty" description:"Enable automatic PC shutdown (accepts =false)"`
	OutageAfter     string   `long:"outage-after" yaml:"outage-after,omitempty" description:"Outage duration before automatic shutdown"`
	LowBattery      Switch   `long:"low-battery-shutdown" optional:"true" optional-value:"true" yaml:"low-battery-shutdown,omitempty" description:"Enable the verified low-battery trigger (accepts =false)"`
	AutoCountdown   string   `long:"automatic-countdown" yaml:"automatic-countdown,omitempty" description:"Explicit automatic shutdown countdown"`
	Manual          Switch   `long:"manual-shutdown" optional:"true" optional-value:"true" yaml:"manual-shutdown,omitempty" description:"Enable authorized desktop shutdown (accepts =false)"`
	ManualCountdown string   `long:"manual-countdown" yaml:"manual-countdown,omitempty" description:"Explicit desktop shutdown countdown"`
	ManualUsers     []int64  `long:"manual-allowed-user-ids" yaml:"manual-allowed-user-ids,omitempty" description:"Legacy metadata only; does not authorize shutdown"`
	DesktopUser     string   `long:"desktop-user-sid" yaml:"desktop-user-sid,omitempty" description:"Windows user SID authorized for desktop IPC"`
	DesktopSettings string   `long:"desktop-settings-file" yaml:"desktop-settings-file,omitempty" description:"Absolute desktop policy JSON path"`
}

type Options struct {
	ConfigFile string `short:"c" long:"config" default:"config.yaml" description:"YAML configuration file"`
	Parameters
	Install  bool `long:"install" description:"Install the Windows service as LocalSystem; do not start it"`
	Remove   bool `long:"remove" description:"Remove the stopped service and installed binary; preserve configuration/history"`
	Start    bool `long:"start" description:"Start the installed service"`
	Stop     bool `long:"stop" description:"Stop the installed service"`
	Status   bool `long:"status" description:"Show service status"`
	Validate bool `long:"validate" description:"Validate effective configuration without starting"`
}

func (o Options) Action() (string, error) {
	action := ""
	for _, entry := range []struct {
		enabled bool
		name    string
	}{{o.Install, "install"}, {o.Remove, "remove"}, {o.Start, "start"}, {o.Stop, "stop"}, {o.Status, "status"}, {o.Validate, "validate"}} {
		if entry.enabled {
			if action != "" {
				return "", errors.New("choose only one of --install, --remove, --start, --stop, --status or --validate")
			}
			action = entry.name
		}
	}
	return action, nil
}

func Parse(args []string, output io.Writer) (Options, error) {
	args = append([]string(nil), args...)
	for i, arg := range args {
		if arg == "--conf" {
			args[i] = "--config"
		} else if value, ok := strings.CutPrefix(arg, "--conf="); ok {
			args[i] = "--config=" + value
		}
	}
	parse := func(args []string) (Options, error) {
		var options Options
		parser := flags.NewParser(&options, flags.HelpFlag)
		parser.Name = "upsdaemon"
		parser.ShortDescription = "ARES UPS monitor"
		parser.LongDescription = "Defaults < YAML < command line. YAML keys match long flags. Missing files use monitoring-disabled defaults. Service installation deploys the effective YAML beside upsdaemon.exe and runs as LocalSystem. Diagnostics: discover, read-status, read-hardware. PC shutdown requires explicit opt-in; UPS output control is unavailable."
		remaining, err := parser.ParseArgs(args)
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			parser.WriteHelp(output)
		}
		if err == nil && len(remaining) != 0 {
			err = fmt.Errorf("unexpected positional arguments: %q", remaining)
		}
		return options, err
	}
	preliminary, err := parse(args)
	if err != nil {
		return preliminary, err
	}
	action, err := preliminary.Action()
	if err != nil {
		return preliminary, err
	}
	path := preliminary.ConfigFile
	if path == "" {
		return preliminary, errors.New("configuration path must not be empty")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return preliminary, err
	}
	// Management must still work when the application's configuration is broken.
	if action == "remove" || action == "start" || action == "stop" || action == "status" {
		preliminary.ConfigFile = path
		return preliminary, nil
	}
	fileArgs, err := argumentsFromFile(path)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(output, "Configuration %s not found; using defaults and command-line overrides.\n", path)
	} else if err != nil {
		return preliminary, err
	}
	options, err := parse(append(fileArgs, args...))
	options.ConfigFile = path
	return options, err
}

func argumentsFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return nil, err
	}
	if len(data) > 65536 {
		return nil, errors.New("config exceeds 64 KiB")
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("config must be a YAML mapping")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("config must contain exactly one YAML document")
	}
	root := document.Content[0]
	legacy := false
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "schema_version" {
			legacy = true
		}
	}
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if legacy {
		var old Config
		if err := strict.Decode(&old); err != nil {
			return nil, fmt.Errorf("decode legacy config: %w", err)
		}
		if err := old.Validate(); err != nil {
			return nil, fmt.Errorf("legacy config: %w", err)
		}
		data, err = Marshal(old)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, &document); err != nil {
			return nil, err
		}
		root = document.Content[0]
	} else {
		var checked Parameters
		if err := strict.Decode(&checked); err != nil {
			return nil, fmt.Errorf("decode config: %w", err)
		}
	}
	var args []string
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i].Value, root.Content[i+1]
		if value.Kind == yaml.SequenceNode {
			for _, item := range value.Content {
				args = append(args, "--"+key+"="+item.Value)
			}
		} else if value.Kind == yaml.ScalarNode && value.Tag != "!!null" {
			args = append(args, "--"+key+"="+value.Value)
		} else {
			return nil, fmt.Errorf("config parameter %q must be a scalar or list", key)
		}
	}
	return args, nil
}

func (p Parameters) Config() Config {
	c := Config{
		SchemaVersion: p.SchemaVersion, PollInterval: p.PollInterval,
		TransactionTimeout: p.TransactionTimeout, StaleAfter: p.StaleAfter, MonitoringEnabled: bool(p.MonitoringEnabled),
	}
	if p.EventDirectory != "" || p.EventMaxFileBytes != nil || p.EventMaxFiles != nil {
		c.EventHistory = &events.Options{Directory: p.EventDirectory, MaxFileBytes: 5 << 20, MaxFiles: 5}
		if p.EventMaxFileBytes != nil {
			c.EventHistory.MaxFileBytes = *p.EventMaxFileBytes
		}
		if p.EventMaxFiles != nil {
			c.EventHistory.MaxFiles = *p.EventMaxFiles
		}
	}
	if p.HardwareInterval != "" {
		c.Hardware = &HardwareOptions{Interval: p.HardwareInterval}
	}
	if p.NotifyAddress != "" || p.TelegramToken != "" || p.TelegramChat != "" || len(p.TelegramEvents)+len(p.TelegramUsers) != 0 {
		c.Telegram = &notifications.Options{RelayAddress: p.NotifyAddress, TokenEnv: p.TelegramToken,
			ChatID: p.TelegramChat, Events: p.TelegramEvents, AllowedUserIDs: p.TelegramUsers}
	}
	if p.PowerMode != "" || bool(p.Automatic) || p.OutageAfter != "" || bool(p.LowBattery) ||
		p.AutoCountdown != "" || bool(p.Manual) || p.ManualCountdown != "" || len(p.ManualUsers) != 0 {
		c.PowerControl = &policy.Options{Mode: p.PowerMode}
		if bool(p.Automatic) || p.OutageAfter != "" || bool(p.LowBattery) || p.AutoCountdown != "" {
			c.PowerControl.Automatic = &policy.AutomaticOptions{Enabled: bool(p.Automatic),
				OutageAfter: p.OutageAfter, LowBattery: bool(p.LowBattery), Countdown: p.AutoCountdown}
		}
		if bool(p.Manual) || p.ManualCountdown != "" || len(p.ManualUsers) != 0 {
			c.PowerControl.Manual = &policy.ManualOptions{Enabled: bool(p.Manual), Countdown: p.ManualCountdown, AllowedUserIDs: p.ManualUsers}
		}
	}
	if p.DesktopUser != "" || p.DesktopSettings != "" {
		c.Desktop = &control.Options{UserSID: p.DesktopUser, SettingsFile: p.DesktopSettings}
	}
	return c
}

func Marshal(c Config) ([]byte, error) {
	p := Parameters{SchemaVersion: c.SchemaVersion, PollInterval: c.PollInterval,
		TransactionTimeout: c.TransactionTimeout, StaleAfter: c.StaleAfter, MonitoringEnabled: Switch(c.MonitoringEnabled)}
	if c.EventHistory != nil {
		p.EventDirectory, p.EventMaxFileBytes, p.EventMaxFiles = c.EventHistory.Directory, &c.EventHistory.MaxFileBytes, &c.EventHistory.MaxFiles
	}
	if c.Hardware != nil {
		p.HardwareInterval = c.Hardware.Interval
	}
	if c.Telegram != nil {
		p.NotifyAddress, p.TelegramToken, p.TelegramChat = c.Telegram.RelayAddress, c.Telegram.TokenEnv, c.Telegram.ChatID
		p.TelegramEvents, p.TelegramUsers = c.Telegram.Events, c.Telegram.AllowedUserIDs
	}
	if c.PowerControl != nil {
		p.PowerMode = c.PowerControl.Mode
		if a := c.PowerControl.Automatic; a != nil {
			p.Automatic, p.OutageAfter, p.LowBattery, p.AutoCountdown = Switch(a.Enabled), a.OutageAfter, Switch(a.LowBattery), a.Countdown
		}
		if m := c.PowerControl.Manual; m != nil {
			p.Manual, p.ManualCountdown, p.ManualUsers = Switch(m.Enabled), m.Countdown, m.AllowedUserIDs
		}
	}
	if c.Desktop != nil {
		p.DesktopUser, p.DesktopSettings = c.Desktop.UserSID, c.Desktop.SettingsFile
	}
	return yaml.Marshal(p)
}
