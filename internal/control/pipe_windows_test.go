//go:build windows

package control

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"upsmart/internal/policy"
	"upsmart/internal/state"
)

func TestNamedPipeRoundTripWithoutTCPOrHardware(t *testing.T) {
	for _, mode := range []string{"console account", "explicit desktop account"} {
		t.Run(mode, func(t *testing.T) { testNamedPipeRoundTrip(t, mode == "explicit desktop account") })
	}
}

func testNamedPipeRoundTrip(t *testing.T, explicitUser bool) {
	t.Helper()
	options := Options{SettingsFile: filepath.Join(t.TempDir(), "policy.json")}
	if explicitUser {
		sid, err := CurrentUserSID()
		if err != nil {
			t.Fatal(err)
		}
		options.UserSID = sid
	}
	path := fmt.Sprintf(`\\.\pipe\UPSmart-Tests-%d`, time.Now().UnixNano())
	listener, err := listen(path, options)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	store := state.New(time.Second, false)
	controller, err := policy.New(policy.Options{}, store.PolicyInput, nil, store.UpdatePolicy)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: NewBridge(store, controller, nil), ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	client := newClient(path)
	defer client.Close()
	var snapshot Snapshot
	if err := client.Request(context.Background(), "GET", "/snapshot", nil, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.UPS.Connection.State != "disabled" || snapshot.UPS.Telemetry != nil || snapshot.UPS.Policy.Enabled {
		t.Fatal("IPC fabricated monitoring or policy")
	}
}
