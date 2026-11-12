package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func waitStatus(t *testing.T, statuses <-chan svc.Status, desired svc.State) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case status := <-statuses:
			if status.State == desired {
				return
			}
		case <-timer.C:
			t.Fatalf("did not reach service state %d", desired)
		}
	}
}

func TestServiceStopCancelsApplicationAndReportsPending(t *testing.T) {
	requests := make(chan svc.ChangeRequest, 2)
	statuses := make(chan svc.Status, 16)
	done := make(chan uint32, 1)
	h := &handler{
		application: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return nil },
		report:      func(err error) { t.Errorf("unexpected service error: %v", err) },
		stopTimeout: time.Second,
	}
	go func() { _, code := h.Execute(nil, requests, statuses); done <- code }()
	waitStatus(t, statuses, svc.StartPending)
	waitStatus(t, statuses, svc.Running)
	requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
	waitStatus(t, statuses, svc.Running)
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	waitStatus(t, statuses, svc.StopPending)
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("stop failed with code %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("service stop did not finish")
	}
}

func TestServiceStartupFailureIsReportedWithoutRunning(t *testing.T) {
	statuses := make(chan svc.Status, 16)
	var reported error
	h := &handler{
		application: func(context.Context, func()) error { return errors.New("configuration invalid") },
		report:      func(err error) { reported = err }, stopTimeout: time.Second,
	}
	_, code := h.Execute(nil, make(chan svc.ChangeRequest), statuses)
	if code == 0 || reported == nil || !strings.Contains(reported.Error(), "configuration invalid") {
		t.Fatalf("startup failure hidden: %d %v", code, reported)
	}
	close(statuses)
	for status := range statuses {
		if status.State == svc.Running {
			t.Fatal("failed startup reported running")
		}
	}
}

func TestServiceStopBudgetIsBounded(t *testing.T) {
	requests := make(chan svc.ChangeRequest, 1)
	statuses := make(chan svc.Status, 16)
	release := make(chan struct{})
	defer close(release)
	done := make(chan uint32, 1)
	reported := make(chan error, 1)
	h := &handler{
		application: func(context.Context, func()) error { <-release; return nil },
		report:      func(err error) { reported <- err }, stopTimeout: 25 * time.Millisecond,
	}
	go func() { _, code := h.Execute(nil, requests, statuses); done <- code }()
	waitStatus(t, statuses, svc.StartPending)
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	waitStatus(t, statuses, svc.StopPending)
	select {
	case code := <-done:
		if code == 0 || !strings.Contains((<-reported).Error(), "stop exceeded") {
			t.Fatal("stop timeout was not an explicit service failure")
		}
	case <-time.After(time.Second):
		t.Fatal("handler exceeded test stop budget")
	}
}

func TestServiceStartupBudgetIsBounded(t *testing.T) {
	statuses := make(chan svc.Status, 16)
	release := make(chan struct{})
	defer close(release)
	reported := make(chan error, 1)
	h := &handler{
		application: func(context.Context, func()) error { <-release; return nil },
		report:      func(err error) { reported <- err }, startTimeout: 25 * time.Millisecond,
		stopTimeout: time.Second,
	}
	start := time.Now()
	_, code := h.Execute(nil, make(chan svc.ChangeRequest), statuses)
	if code == 0 || !strings.Contains((<-reported).Error(), "startup exceeded") || time.Since(start) > time.Second {
		t.Fatal("startup timeout was not bounded and explicit")
	}
}

func TestServiceDispatcherCannotRunAsAConsoleCommand(t *testing.T) {
	called := false
	err := Run(func(context.Context, func()) error { called = true; return nil })
	if err == nil || !strings.Contains(err.Error(), "reserved for Windows Service Control Manager") || called {
		t.Fatalf("console invocation reached service application: called=%t error=%v", called, err)
	}
}
