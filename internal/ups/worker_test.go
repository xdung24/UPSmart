package ups

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "__hid" {
		switch os.Getenv("UPSMART_TEST_HELPER") {
		case "blocked":
			time.Sleep(time.Minute)
		case "oversized":
			os.Stdout.WriteString(strings.Repeat("x", 40000))
		case "refresh-only":
			os.Stdout.WriteString(`{"devices":[{"product":"MEC0003"}],"refresh_only":true}`)
		default:
			os.Stdout.WriteString(`{"devices":[],"error":"test device absent"}`)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRefreshOnlySessionUsesLastVerifiedQ1(t *testing.T) {
	t.Setenv("UPSMART_TEST_HELPER", "refresh-only")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	worker := Worker{
		Executable: exe,
		lastResult: Result{
			Devices: []Device{{Product: "MEC0003"}},
			Status:  "(230.0 000.0 230.0 010 50.0 27.0 30.0 00001000\r",
		},
	}
	result, err := worker.Run(context.Background(), true)
	if err != nil || result.Status != worker.lastResult.Status {
		t.Fatalf("refresh-only response did not retain verified Q1: %+v %v", result, err)
	}
	if err := worker.Close(); err != nil {
		t.Logf("helper stop: %v", err)
	}
}

func TestWorkerBoundsBlockedNativeWork(t *testing.T) {
	t.Setenv("UPSMART_TEST_HELPER", "blocked")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	worker := Worker{Executable: exe}
	_, err = worker.Run(ctx, true)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("helper termination exceeded 3 seconds")
	}
}

func TestWorkerRejectsHelperErrorsAndOversizedOutput(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"absent", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("UPSMART_TEST_HELPER", mode)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			worker := Worker{Executable: exe}
			if _, err := worker.Run(ctx, false); err == nil {
				t.Fatal("expected explicit helper error")
			}
		})
	}
}
