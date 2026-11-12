package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"upsmart/internal/policy"
	"upsmart/internal/state"
	"upsmart/internal/ups"
)

func TestShutdownPolicyFromTelemetryToReadOnlyAPIWithFakeOS(t *testing.T) {
	store := state.New(10*time.Second, true)
	start := time.Now()
	sample := func(bits string) ups.Result {
		return ups.Result{Devices: []ups.Device{{Product: "MEC0003"}}, Status: "(230.0 000.0 230.0 010 50.0 27.0 30.0 " + bits + "\r"}
	}
	calls := 0
	control, err := policy.New(policy.Options{
		Mode: "live", Automatic: &policy.AutomaticOptions{Enabled: true, OutageAfter: "1s", Countdown: "5s"},
	}, store.PolicyInput, func(context.Context) error { calls++; return nil }, store.UpdatePolicy)
	if err != nil {
		t.Fatal(err)
	}
	apply := func(now time.Time, bits string) {
		t.Helper()
		if err := store.Apply(now, sample(bits), nil); err != nil {
			t.Fatal(err)
		}
		control.Tick(context.Background(), now)
	}
	apply(start, "10001000")
	control.Tick(context.Background(), start.Add(time.Second))
	server := NewServer("127.0.0.1:8080", store)
	w := httptest.NewRecorder()
	server.Handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/status", nil))
	var snapshot state.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || snapshot.Policy.State != "pending" || snapshot.Policy.Deadline == nil ||
		!snapshot.Policy.Deadline.Equal(start.Add(6*time.Second)) || !snapshot.Policy.AutomaticEnabled ||
		snapshot.Policy.DryRun || snapshot.Policy.UPSPowerOff || calls != 0 ||
		snapshot.Policy.OutageAfter != "1s" || snapshot.Policy.AutomaticCountdown != "5s" {
		t.Fatalf("API did not expose actual pending policy: %+v; fake OS calls=%d", snapshot.Policy, calls)
	}
	for _, path := range []string{"/api/v1/shutdown", "/api/v1/cancel", "/api/v1/status"} {
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, httptest.NewRequest("POST", "http://127.0.0.1:8080"+path, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("live policy opened a mutation route: %s HTTP %d", path, w.Code)
		}
	}
	apply(start.Add(6*time.Second), "00001000")
	if got := store.Snapshot(start.Add(6 * time.Second)); got.Policy.Deadline != nil || got.Policy.State == "pending" || calls != 0 {
		t.Fatal("restoration failed to cancel deadline before dispatch")
	}
	apply(start.Add(7*time.Second), "10001000")
	control.Tick(context.Background(), start.Add(8*time.Second))
	control.Tick(context.Background(), start.Add(13*time.Second))
	control.Tick(context.Background(), start.Add(14*time.Second))
	if got := store.Snapshot(start.Add(14 * time.Second)); got.Policy.State != "requested" || calls != 1 {
		t.Fatalf("policy did not dispatch once to injected fake: %+v calls=%d", got.Policy, calls)
	}
}

func TestHTTPBoundariesAndReadiness(t *testing.T) {
	store := state.New(10*time.Second, true)
	server := NewServer("127.0.0.1:8080", store)
	for _, test := range []struct {
		name, method, path, host, origin string
		want                             int
	}{
		{"dashboard", "GET", "/", "", "", 200},
		{"assets", "GET", "/app.js", "", "", 200},
		{"removed reader license", "GET", "/cputemp-license.txt", "", "", 404},
		{"live", "GET", "/healthz", "", "", 200},
		{"not ready", "GET", "/readyz", "", "", 503},
		{"status absent", "GET", "/api/v1/status", "", "", 200},
		{"hardware disabled", "GET", "/api/v1/hardware", "", "", 200},
		{"hardware head", "HEAD", "/api/v1/hardware", "", "", 200},
		{"hardware wrong host", "GET", "/api/v1/hardware", "evil.example", "", 403},
		{"hardware cross origin", "GET", "/api/v1/hardware", "", "http://evil.example", 403},
		{"hardware mutation", "POST", "/api/v1/hardware", "", "", 405},
		{"head", "HEAD", "/api/v1/status", "", "", 200},
		{"head error", "HEAD", "/api/v1/status", "evil.example", "", 403},
		{"wrong host", "GET", "/", "evil.example", "", 403},
		{"cross origin", "GET", "/api/v1/status", "", "http://evil.example", 403},
		{"same origin", "GET", "/", "", "http://127.0.0.1:8080", 200},
		{"mutation", "POST", "/api/v1/status", "", "", 405},
		{"no shutdown endpoint", "GET", "/api/v1/shutdown", "", "", 404},
		{"bad limit", "GET", "/api/v1/events?limit=201", "", "", 400},
		{"bad cursor", "GET", "/api/v1/events?before=bad!", "", "", 400},
		{"repeated parameter", "GET", "/api/v1/events?limit=1&limit=2", "", "", 400},
		{"unknown parameter", "GET", "/api/v1/events?file=secret", "", "", 400},
		{"missing asset", "GET", "/other", "", "", 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, "http://127.0.0.1:8080"+test.path, nil)
			if test.host != "" {
				r.Host = test.host
			}
			if test.origin != "" {
				r.Header.Set("Origin", test.origin)
			}
			w := httptest.NewRecorder()
			server.Handler.ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("HTTP %d; wanted %d: %s", w.Code, test.want, w.Body.String())
			}
			if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("missing boundary headers or unexpected CORS")
			}
			if test.method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD returned a body")
			}
			if test.name == "status absent" {
				var snapshot state.Snapshot
				if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
					t.Fatal(err)
				}
				if test.name == "hardware disabled" {
					var hardware state.HardwareStatus
					if err := json.Unmarshal(w.Body.Bytes(), &hardware); err != nil {
						t.Fatal(err)
					}
					if hardware.Mode != "disabled" || hardware.SampledAt != nil || string(hardware.Data) != "null" || !hardware.Stale {
						t.Fatalf("disabled hardware fabricated readings: %+v", hardware)
					}
				}
				if snapshot.Telemetry != nil || snapshot.Policy.Enabled || snapshot.Policy.UPSPowerOff {
					t.Fatal("unavailable status fabricated telemetry or enabled controls")
				}
			}
		})
	}
	result := ups.Result{Devices: []ups.Device{{Product: "MEC0003"}}, Status: "(230.0 000.0 230.0 010 50.0 27.0 30.0 00001001\r"}
	if err := store.Apply(time.Now(), result, nil); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	server.Handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8080/readyz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected ready with valid sample, got %d", w.Code)
	}
}
