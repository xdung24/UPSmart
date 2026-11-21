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
		default:
			os.Stdout.WriteString(`{"devices":[],"error":"test device absent"}`)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
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
