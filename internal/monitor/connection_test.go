package monitor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"upsmart/internal/control"
	"upsmart/internal/state"
	"upsmart/internal/web"
)

type fakeClient struct {
	calls  int
	fail   int
	schema int
}

func (c *fakeClient) Request(_ context.Context, method, path string, _ io.Reader, result any) error {
	c.calls++
	if method != http.MethodGet || path != "/snapshot" {
		return errors.New("unexpected request")
	}
	if c.calls <= c.fail {
		return errors.New("no daemon")
	}
	result.(*control.Snapshot).UPS.SchemaVersion = c.schema
	return nil
}

func TestDaemonConnectionNeverStartsDirectMonitor(t *testing.T) {
	client := &fakeClient{schema: 1}
	session, err := Connect(context.Background(), Connections{Pipe: client, Web: func() (Client, error) {
		t.Error("pipe connection unnecessarily loaded web configuration")
		return nil, errors.New("must not load")
	}}, func(context.Context, func()) error {
		t.Error("daemon connection attempted to start a direct monitor")
		return errors.New("must not start")
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Direct() || client.calls != 1 {
		t.Fatal("compatible daemon was not selected")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.Request(context.Background(), http.MethodGet, "/snapshot", nil, new(control.Snapshot)); err != nil {
		t.Fatal("closing the client session stopped daemon access:", err)
	}
}

func TestMissingDaemonStartsDirectAndQuitJoinsIt(t *testing.T) {
	client := &fakeClient{fail: 1, schema: 1}
	stopped := false
	session, err := Connect(context.Background(), Connections{Pipe: client}, func(ctx context.Context, ready func()) error {
		ready()
		<-ctx.Done()
		stopped = true
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if !session.Direct() {
		t.Fatal("absent daemon did not select direct mode")
	}
	if err := session.Request(context.Background(), http.MethodGet, "/snapshot", nil, new(control.Snapshot)); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil || !stopped {
		t.Fatal("quit did not cancel and join direct monitoring:", err)
	}
	calls := client.calls
	if err := session.Request(context.Background(), http.MethodGet, "/snapshot", nil, new(control.Snapshot)); err == nil || client.calls != calls {
		t.Fatal("stopped direct session silently connected to a new owner")
	}
}

func TestDaemonWinningStartupRaceIsUsedWithoutTakeover(t *testing.T) {
	client := &fakeClient{fail: 1, schema: 1}
	session, err := Connect(context.Background(), Connections{Pipe: client}, func(context.Context, func()) error {
		return errors.New("another monitor already owns USB")
	})
	if err != nil || session.Direct() || client.calls != 2 {
		t.Fatalf("daemon startup race was not recovered safely: %v", err)
	}
}

func TestDirectStartupFailureAndUnsupportedDaemonAreVisible(t *testing.T) {
	t.Run("owner blocks direct mode", func(t *testing.T) {
		client := &fakeClient{fail: 2}
		busy := errors.New("another instance already running")
		_, err := Connect(context.Background(), Connections{Pipe: client}, func(context.Context, func()) error { return busy })
		if !errors.Is(err, busy) || !strings.Contains(err.Error(), "direct USB startup failed") {
			t.Fatal("existing owner was bypassed or hidden:", err)
		}
	})
	t.Run("unsupported daemon cannot trigger USB fallback", func(t *testing.T) {
		_, err := Connect(context.Background(), Connections{Pipe: &fakeClient{schema: 2}}, func(context.Context, func()) error {
			t.Error("unsupported daemon triggered direct monitoring")
			return nil
		})
		if err == nil || !strings.Contains(err.Error(), "schema 2") {
			t.Fatal("unsupported schema not reported:", err)
		}
	})
}

func TestCancelledStartupDoesNotStartDirectMonitor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Connect(ctx, Connections{Pipe: &fakeClient{fail: 1}}, func(context.Context, func()) error {
		t.Error("cancelled startup started USB monitoring")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCancellingDuringDirectStartupJoinsPartialRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, stopped := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := Connect(ctx, Connections{Pipe: &fakeClient{fail: 1}}, func(ctx context.Context, _ func()) error {
			close(started)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		})
		result <- err
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal("startup cancellation lost:", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("cancelled startup returned before releasing the partial runtime")
	}
}

func TestDirectRuntimeFailurePreventsSilentOwnerChange(t *testing.T) {
	fail := make(chan struct{})
	runtimeErr := errors.New("pipe listener failed")
	client := &fakeClient{fail: 1, schema: 1}
	session, err := Connect(context.Background(), Connections{Pipe: client}, func(ctx context.Context, ready func()) error {
		ready()
		select {
		case <-fail:
			return runtimeErr
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	close(fail)
	<-session.done
	if err := session.Request(context.Background(), http.MethodGet, "/snapshot", nil, new(control.Snapshot)); !errors.Is(err, runtimeErr) || client.calls != 1 {
		t.Fatal("runtime failure was hidden by another owner:", err)
	}
	if err := session.Close(); !errors.Is(err, runtimeErr) {
		t.Fatal("runtime failure lost on close:", err)
	}
}

func TestQuitPreservesCleanupFailureAlongsideCancellation(t *testing.T) {
	cleanupErr := errors.New("event journal close failed")
	session, err := Connect(context.Background(), Connections{Pipe: &fakeClient{fail: 1, schema: 1}}, func(ctx context.Context, ready func()) error {
		ready()
		<-ctx.Done()
		return errors.Join(ctx.Err(), cleanupErr)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); !errors.Is(err, cleanupErr) {
		t.Fatal("cancellation concealed a real cleanup failure:", err)
	}
}

func TestWebConnectionWinsWithoutStartingUSB(t *testing.T) {
	server := httptest.NewUnstartedServer(nil)
	address := server.Listener.Addr().String()
	server.Config.Handler = web.NewServer(address, state.New(10*time.Second, false)).Handler
	server.Start()
	defer server.Close()
	client, err := control.NewWebClient(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := Connect(context.Background(), Connections{
		Pipe: &fakeClient{fail: 1},
		Web:  func() (Client, error) { return client, nil },
	}, func(context.Context, func()) error {
		t.Error("available web API triggered USB startup")
		return errors.New("must not start")
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Direct() || !session.ReadOnly() {
		t.Fatal("web connection not marked read-only")
	}
	var snapshot control.Snapshot
	if err := session.Request(context.Background(), http.MethodGet, "/snapshot", nil, &snapshot); err != nil ||
		snapshot.UPS.Connection.State != "disabled" || snapshot.Hardware.Mode != "disabled" || len(snapshot.Events.Items) == 0 {
		t.Fatal("session did not use web transport:", err)
	}
}

func TestIncompatibleWebAPIStopsSelectionBeforeUSB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"schema_version":2}`))
	}))
	defer server.Close()
	client, err := control.NewWebClient(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = Connect(context.Background(), Connections{
		Pipe: &fakeClient{fail: 1}, Web: func() (Client, error) { return client, nil },
	}, func(context.Context, func()) error {
		t.Error("incompatible web response silently enabled USB")
		return nil
	})
	if !errors.Is(err, control.ErrInvalidWebResponse) {
		t.Fatal("web incompatibility was hidden:", err)
	}
}

func TestWebStartupRaceAndInvalidDiscovery(t *testing.T) {
	t.Run("web daemon wins startup race", func(t *testing.T) {
		resolutions := 0
		session, err := Connect(context.Background(), Connections{
			Pipe: &fakeClient{fail: 2},
			Web: func() (Client, error) {
				resolutions++
				return &fakeClient{fail: 1, schema: 1}, nil
			},
		}, func(context.Context, func()) error { return errors.New("daemon owns USB") })
		if err != nil || !session.ReadOnly() || session.Direct() || resolutions != 1 {
			t.Fatal("web startup race did not preserve daemon ownership:", err)
		}
	})
	t.Run("invalid discovery config is not silently replaced", func(t *testing.T) {
		invalid := errors.New("invalid web discovery configuration")
		_, err := Connect(context.Background(), Connections{
			Pipe: &fakeClient{fail: 1},
			Web:  func() (Client, error) { return nil, invalid },
		}, func(context.Context, func()) error {
			t.Error("invalid configuration silently enabled USB")
			return nil
		})
		if !errors.Is(err, invalid) {
			t.Fatal("discovery error lost:", err)
		}
	})
}

func TestBothTransportsUnavailableStartDirectExactlyOnce(t *testing.T) {
	calls := 0
	session, err := Connect(context.Background(), Connections{
		Pipe: &fakeClient{fail: 1},
		Web:  func() (Client, error) { return &fakeClient{fail: 1}, nil },
	}, func(ctx context.Context, ready func()) error {
		calls++
		ready()
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if !session.Direct() || session.ReadOnly() || calls != 1 {
		t.Fatal("unavailable remote transports did not select one owned runtime")
	}
}
