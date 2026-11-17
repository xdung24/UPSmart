package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServiceProgramReadinessAndOrderlyStop(t *testing.T) {
	p := &program{
		application: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return nil },
		fatal:       func(err error) { t.Errorf("unexpected fatal error: %v", err) },
	}
	if err := p.Start(nil); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(nil); err != nil {
		t.Fatal(err)
	}
}

func TestServiceProgramStartupFailureIsReturned(t *testing.T) {
	failure := errors.New("configuration invalid")
	p := &program{application: func(context.Context, func()) error { return failure }}
	if err := p.Start(nil); !errors.Is(err, failure) {
		t.Fatalf("startup failure hidden: %v", err)
	}
}

func TestServiceProgramStartupAndStopRemainBounded(t *testing.T) {
	startup := &program{
		application:  func(ctx context.Context, _ func()) error { <-ctx.Done(); return nil },
		startTimeout: 20 * time.Millisecond,
	}
	if err := startup.Start(nil); err == nil || !strings.Contains(err.Error(), "startup exceeded") {
		t.Fatalf("startup deadline hidden: %v", err)
	}
	release := make(chan struct{})
	defer close(release)
	stopping := &program{
		application: func(_ context.Context, ready func()) error { ready(); <-release; return nil },
		stopTimeout: 20 * time.Millisecond,
		fatal:       func(err error) { t.Errorf("unexpected fatal error: %v", err) },
	}
	if err := stopping.Start(nil); err != nil {
		t.Fatal(err)
	}
	if err := stopping.Stop(nil); err == nil || !strings.Contains(err.Error(), "stop exceeded") {
		t.Fatalf("stop deadline hidden: %v", err)
	}
}

func TestServiceProgramReportsUnexpectedRuntimeExit(t *testing.T) {
	release, reported := make(chan struct{}), make(chan error, 1)
	failure := errors.New("monitor failed")
	p := &program{
		application: func(_ context.Context, ready func()) error { ready(); <-release; return failure },
		fatal:       func(err error) { reported <- err },
	}
	if err := p.Start(nil); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-reported:
		if !errors.Is(err, failure) {
			t.Fatalf("unexpected runtime error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("exited monitor would leave a success-shaped running service")
	}
	if err := p.Stop(nil); !errors.Is(err, failure) {
		t.Fatalf("stop lost runtime failure: %v", err)
	}
}

func TestServiceDispatcherCannotRunAsAConsoleCommand(t *testing.T) {
	called := false
	err := Run(filepath.Join(t.TempDir(), "config.yaml"), func(context.Context, func()) error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "reserved for Windows Service Control Manager") || called {
		t.Fatalf("console invocation reached service application: called=%t error=%v", called, err)
	}
}
