//go:build windows

package platform

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestShutdownAdapterUsesNonForcedImmediateRequestAndSurfacesFailure(t *testing.T) {
	rejected := errors.New("privilege not held")
	calls := 0
	err := requestShutdown(context.Background(), `C:\Windows\System32`, func(ctx context.Context, path string, args ...string) error {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second || filepath.Base(path) != "shutdown.exe" {
			t.Fatal("shutdown request is not bounded or uses an unexpected program")
		}
		if slices.Contains(args, "/f") || slices.Index(args, "/t") < 0 || args[slices.Index(args, "/t")+1] != "0" {
			t.Fatal("Windows must not implicitly or explicitly force application termination")
		}
		return rejected
	})
	if !errors.Is(err, rejected) || calls != 1 {
		t.Fatalf("OS rejection was hidden or retried: %v, calls=%d", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := requestShutdown(ctx, `C:\Windows\System32`, func(context.Context, string, ...string) error {
		t.Fatal("cancelled policy called OS adapter")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
