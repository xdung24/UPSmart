package control

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"upsmart/internal/policy"
	"upsmart/internal/state"
)

const PipeName = `\\.\pipe\UPSmartMonitor-Desktop`

type Options struct {
	UserSID      string `json:"user_sid,omitempty"`
	SettingsFile string `json:"settings_file"`
}

func (o Options) Validate() error {
	if o.UserSID != "" && !regexp.MustCompile(`^S-1-(5-21|12-1)(-[0-9]+){4}$`).MatchString(o.UserSID) {
		return errors.New("desktop.user_sid must be a specific Windows user SID, not a group or account name")
	}
	if !filepath.IsAbs(o.SettingsFile) || filepath.Base(o.SettingsFile) == "." ||
		filepath.Ext(o.SettingsFile) != ".json" || len(o.SettingsFile) > 240 ||
		len(filepath.VolumeName(o.SettingsFile)) != 2 {
		return errors.New("desktop.settings_file must be an absolute local .json file path")
	}
	return nil
}

type Snapshot struct {
	UPS      state.Snapshot       `json:"ups"`
	Hardware state.HardwareStatus `json:"hardware"`
	Events   state.EventPage      `json:"events"`
}

type PolicySettings struct {
	Options  policy.Options `json:"options"`
	Revision string         `json:"revision"`
}

type ShutdownRequest struct {
	Mode      string `json:"mode"`
	Countdown string `json:"countdown"`
}

func revision(value policy.Status) string {
	text := fmt.Sprintf("%t|%t|%t|%t|%s|%t|%s|%s", value.Enabled, value.AutomaticEnabled,
		value.ManualEnabled, value.DryRun, value.OutageAfter, value.LowBatteryEnabled,
		value.AutomaticCountdown, value.ManualCountdown)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}

type Bridge struct {
	store  *state.Store
	policy *policy.Controller
	save   func(policy.Options) error
	mu     sync.Mutex
	slots  chan struct{}
}

func NewBridge(store *state.Store, controller *policy.Controller, save func(policy.Options) error) *Bridge {
	return &Bridge{store: store, policy: controller, save: save, slots: make(chan struct{}, 8)}
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != "upsmart.local" {
		http.Error(w, "invalid local IPC host", http.StatusForbidden)
		return
	}
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
	default:
		http.Error(w, "desktop IPC busy", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.Context().Err() != nil {
		http.Error(w, "desktop request cancelled before execution", http.StatusRequestTimeout)
		return
	}
	var value any
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/snapshot":
		now := time.Now()
		page, err := b.store.Page(100, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		value = Snapshot{b.store.Snapshot(now), b.store.HardwareSnapshot(now), page}
	case r.Method == http.MethodGet && r.URL.Path == "/policy":
		value = PolicySettings{b.policy.Configuration(), revision(b.policy.Status())}
	case r.Method == http.MethodPut && r.URL.Path == "/policy":
		var settings PolicySettings
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&settings); err != nil {
			http.Error(w, "invalid policy document", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			http.Error(w, "policy must contain one JSON document", http.StatusBadRequest)
			return
		}
		if settings.Revision != revision(b.policy.Status()) {
			http.Error(w, "policy changed; reopen settings before saving", http.StatusConflict)
			return
		}
		err := b.policy.Reconfigure(settings.Options, func() error {
			if b.save == nil {
				return errors.New("durable desktop policy storage is not configured")
			}
			return b.save(settings.Options)
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		value = b.policy.Status()
	case r.Method == http.MethodPost && (r.URL.Path == "/shutdown" || r.URL.Path == "/cancel"):
		var err error
		if r.URL.Path == "/shutdown" {
			var expected ShutdownRequest
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			if err := d.Decode(&expected); err != nil {
				http.Error(w, "shutdown requires the confirmed mode and countdown", http.StatusBadRequest)
				return
			}
			if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
				http.Error(w, "shutdown requires exactly one confirmation document", http.StatusBadRequest)
				return
			}
			current := b.policy.Configuration()
			if expected.Mode != current.Mode || current.Manual == nil || expected.Countdown != current.Manual.Countdown {
				http.Error(w, "policy changed; review and confirm shutdown again", http.StatusConflict)
				return
			}
			err = b.policy.Manual(time.Now())
		} else {
			if r.ContentLength != 0 {
				http.Error(w, "cancel request must have an empty body", http.StatusBadRequest)
				return
			}
			err = b.policy.Cancel(time.Now())
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		value = b.policy.Status()
	default:
		http.Error(w, "unsupported desktop IPC operation", http.StatusNotFound)
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "could not encode desktop state", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func LoadPolicy(path string) (*policy.Options, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open saved desktop policy: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > 16384 {
		return nil, errors.New("saved desktop policy cannot be inspected or exceeds 16 KiB")
	}
	d := json.NewDecoder(io.LimitReader(file, 16385))
	d.DisallowUnknownFields()
	var options policy.Options
	if err := d.Decode(&options); err != nil {
		return nil, fmt.Errorf("decode saved desktop policy: %w", err)
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("saved desktop policy must contain exactly one document")
	}
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("saved desktop policy: %w", err)
	}
	return &options, nil
}

func SavePolicy(path string, options policy.Options) (resultErr error) {
	if err := options.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(options, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > 16384 {
		return errors.New("saved desktop policy exceeds 16 KiB")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".upsmart-policy-*.tmp")
	if err != nil {
		return fmt.Errorf("create policy transaction (parent directory must already exist): %w", err)
	}
	name := file.Name()
	defer func() {
		if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	_, writeErr := file.Write(append(data, '\n'))
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return fmt.Errorf("write policy transaction: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("commit policy transaction: %w", err)
	}
	return nil
}

type Client struct {
	http *http.Client
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) Request(ctx context.Context, method, path string, body io.Reader, result any) error {
	request, err := http.NewRequestWithContext(ctx, method, "http://upsmart.local"+path, body)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		if method == http.MethodPost || method == http.MethodPut {
			return errors.New("desktop request interrupted; backend acceptance is unknown; reconnect and check policy state before retrying")
		}
		return errors.New("desktop backend unavailable; start the configured monitor/service with desktop IPC enabled")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return errors.New("desktop backend response was unreadable or oversized")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("backend rejected operation: %s", string(data))
	}
	if err := json.Unmarshal(data, result); err != nil {
		return errors.New("desktop backend returned an invalid response")
	}
	return nil
}
