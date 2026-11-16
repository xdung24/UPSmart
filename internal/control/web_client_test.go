package control

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"upsmart/internal/policy"
	"upsmart/internal/state"
	"upsmart/internal/web"
)

func TestWebClientReadsActualCachedAPIWithoutIPCOrHardware(t *testing.T) {
	store := state.New(10*time.Second, false)
	store.UpdatePolicy(policy.Status{Enabled: true, ManualEnabled: true, State: "pending"}, "")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	server := web.NewServer(address, store)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	client, err := NewWebClient(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var snapshot Snapshot
	if err := client.Request(context.Background(), http.MethodGet, "/snapshot", nil, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.UPS.SchemaVersion != 1 || snapshot.UPS.Connection.State != "disabled" ||
		snapshot.UPS.Telemetry != nil || !snapshot.UPS.Policy.ManualEnabled ||
		snapshot.Hardware.Mode != "disabled" || len(snapshot.Events.Items) == 0 {
		t.Fatal("web adapter lost or fabricated cached API data")
	}
}

func TestWebClientNeverSendsControlsOrFollowsRedirects(t *testing.T) {
	var calls atomic.Int32
	var sourceCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	client, err := NewWebClient(strings.TrimPrefix(redirect.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, operation := range [][2]string{{"GET", "/policy"}, {"PUT", "/policy"}, {"POST", "/shutdown"}, {"POST", "/cancel"}} {
		if err := client.Request(context.Background(), operation[0], operation[1], nil, new(Snapshot)); err == nil {
			t.Fatal("web adapter accepted a policy/control operation")
		}
	}
	if sourceCalls.Load() != 0 {
		t.Fatal("web adapter sent a control request over HTTP")
	}
	if err := client.Request(context.Background(), "GET", "/snapshot", nil, new(Snapshot)); err == nil {
		t.Fatal("web adapter followed a redirect")
	}
	if calls.Load() != 0 || sourceCalls.Load() != 1 {
		t.Fatal("web adapter contacted the redirect destination")
	}
}

func TestWebClientRejectsBadSnapshotsWithoutPartialPublication(t *testing.T) {
	for _, kind := range []string{"oversized", "invalid JSON", "unsupported schema", "hardware failure", "null events", "exact size limit"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/status":
					body := `{"schema_version":1}`
					switch kind {
					case "oversized":
						body += strings.Repeat(" ", (1<<20)+1-len(body))
					case "exact size limit":
						body += strings.Repeat(" ", (1<<20)-len(body))
					case "invalid JSON":
						body = `{"schema_version":`
					case "unsupported schema":
						body = `{"schema_version":2}`
					}
					w.Write([]byte(body))
				case "/api/v1/hardware":
					if kind == "hardware failure" {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					json.NewEncoder(w).Encode(map[string]string{"mode": "disabled"})
				case "/api/v1/events":
					if kind == "null events" {
						w.Write([]byte("null"))
						return
					}
					w.Write([]byte(`{"items":[],"next_cursor":null}`))
				default:
					t.Error("unexpected route:", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, err := NewWebClient(strings.TrimPrefix(server.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			snapshot := Snapshot{UPS: state.Snapshot{SchemaVersion: 99}}
			err = client.Request(context.Background(), "GET", "/snapshot", nil, &snapshot)
			if kind == "exact size limit" {
				if err != nil || snapshot.UPS.SchemaVersion != 1 {
					t.Fatal("exactly 1 MiB valid JSON was rejected:", err)
				}
			} else if err == nil || snapshot.UPS.SchemaVersion != 99 {
				t.Fatal("invalid/partial response was published as success:", err)
			}
		})
	}
}

func TestWebClientRejectsNonLoopbackTargets(t *testing.T) {
	for _, address := range []string{"example.com:8080", "localhost:8080", "192.168.1.1:8080", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1:8080/path"} {
		if _, err := NewWebClient(address); err == nil {
			t.Fatal("accepted noncanonical/nonloopback web address:", address)
		}
	}
}
