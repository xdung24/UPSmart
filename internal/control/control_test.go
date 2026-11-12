package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"upsmart/internal/policy"
	"upsmart/internal/state"
)

func TestDesktopPolicyPersistenceConfirmationAndCancellation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "power-policy.json")
	store := state.New(10*time.Second, false)
	calls := 0
	controller, err := policy.New(policy.Options{}, store.PolicyInput, func(context.Context) error {
		calls++
		return nil
	}, store.UpdatePolicy)
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewBridge(store, controller, func(options policy.Options) error { return SavePolicy(file, options) })
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var data []byte
		if body != nil {
			var err error
			data, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		w := httptest.NewRecorder()
		bridge.ServeHTTP(w, httptest.NewRequest(method, "http://upsmart.local"+path, bytes.NewReader(data)))
		return w
	}
	settings := func() PolicySettings {
		t.Helper()
		w := request("GET", "/policy", nil)
		var value PolicySettings
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	old := settings()
	change := old
	change.Options = policy.Options{Mode: "dry_run", Manual: &policy.ManualOptions{Enabled: true, Countdown: "5s"}}
	if w := request("PUT", "/policy", change); w.Code != 200 {
		t.Fatalf("save failed: %s", w.Body.String())
	}
	saved, err := LoadPolicy(file)
	if err != nil || saved == nil || !saved.ManualEnabled() || !controller.Status().ManualEnabled {
		t.Fatal("policy was not persisted and activated", err)
	}
	if w := request("POST", "/shutdown", ShutdownRequest{Mode: "live", Countdown: "5s"}); w.Code != 409 {
		t.Fatal("mode changed after confirmation but shutdown was accepted")
	}
	if w := request("POST", "/shutdown", ShutdownRequest{Mode: "dry_run", Countdown: "5s"}); w.Code != 200 {
		t.Fatalf("confirmed desktop shutdown not scheduled: %s", w.Body.String())
	}
	if w := request("PUT", "/policy", settings()); w.Code != 409 {
		t.Fatal("pending policy reset was accepted")
	}
	if w := request("POST", "/cancel", nil); w.Code != 200 {
		t.Fatal("desktop cancel failed")
	}
	if w := request("PUT", "/policy", old); w.Code != 409 {
		t.Fatal("stale settings overwrote newer configuration")
	}
	controller.Tick(context.Background(), time.Now().Add(time.Hour))
	if calls != 0 || controller.Status().State == "pending" {
		t.Fatal("cancelled/dry-run desktop action reached OS adapter")
	}
}

func TestFailedSavePreservesActivePolicyAndCorruptionIsExplicit(t *testing.T) {
	store := state.New(time.Second, false)
	controller, err := policy.New(policy.Options{}, store.PolicyInput, nil, store.UpdatePolicy)
	if err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("simulated disk failure")
	err = controller.Reconfigure(policy.Options{Mode: "dry_run", Manual: &policy.ManualOptions{Enabled: true, Countdown: "5s"}}, func() error { return rejected })
	if !errors.Is(err, rejected) || controller.Status().Enabled {
		t.Fatal("failed persistence changed active policy", err)
	}
	path := filepath.Join(t.TempDir(), "policy.json")
	for _, contents := range []string{"{", `{} {}`, strings.Repeat(" ", 16384) + "{}"} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadPolicy(path); err == nil {
			t.Fatal("invalid saved policy silently accepted")
		}
	}
}

func TestLocalControlRejectsMalformedRequestsAndWrongHost(t *testing.T) {
	store := state.New(time.Second, false)
	controller, err := policy.New(policy.Options{}, store.PolicyInput, nil, store.UpdatePolicy)
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewBridge(store, controller, nil)
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{"PUT", "/policy", "{", 400},
		{"PUT", "/policy", `{"revision":"x","options":{}} {}`, 400},
		{"PUT", "/policy", strings.Repeat("x", 16385), 400},
		{"POST", "/shutdown", "", 400},
		{"POST", "/shutdown", `{"mode":"live","countdown":"5s"} {}`, 400},
		{"DELETE", "/policy", "", 404},
	} {
		w := httptest.NewRecorder()
		bridge.ServeHTTP(w, httptest.NewRequest(tc.method, "http://upsmart.local"+tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	bridge.ServeHTTP(w, httptest.NewRequest("GET", "http://evil.example/snapshot", nil))
	if w.Code != 403 {
		t.Fatal("unexpected IPC host accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w = httptest.NewRecorder()
	bridge.ServeHTTP(w, httptest.NewRequest("POST", "http://upsmart.local/cancel", nil).WithContext(ctx))
	if w.Code != http.StatusRequestTimeout {
		t.Fatal("cancelled queued request entered execution")
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("untrusted transport details")
}

func TestInterruptedMutationDisclosesUnknownAcceptance(t *testing.T) {
	client := &Client{http: &http.Client{Transport: failingTransport{}}}
	var result policy.Status
	err := client.Request(context.Background(), "POST", "/shutdown", nil, &result)
	if err == nil || !strings.Contains(err.Error(), "acceptance is unknown") || strings.Contains(err.Error(), "untrusted transport details") {
		t.Fatal("ambiguous mutation was reported as definitely unaccepted or leaked transport details", err)
	}
}
