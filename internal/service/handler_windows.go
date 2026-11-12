package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

type handler struct {
	application  Application
	report       func(error)
	stopTimeout  time.Duration
	startTimeout time.Duration
}

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, done := make(chan struct{}), make(chan error, 1)
	var once sync.Once
	go func() { done <- h.application(ctx, func() { once.Do(func() { close(ready) }) }) }()
	status := svc.Status{State: svc.StartPending, CheckPoint: 1, WaitHint: 10000}
	statuses <- status
	startBudget := h.startTimeout
	if startBudget <= 0 {
		startBudget = 10 * time.Second
	}
	startTimer := time.NewTimer(startBudget)
	defer startTimer.Stop()
	startDeadline := startTimer.C
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var stopDeadline <-chan time.Time
	var stopTimer *time.Timer
	defer func() {
		if stopTimer != nil {
			stopTimer.Stop()
		}
	}()
	for {
		select {
		case <-ready:
			ready = nil
			startTimer.Stop()
			startDeadline = nil
			if status.State == svc.StartPending {
				status = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
				statuses <- status
			}
		case request, ok := <-requests:
			if !ok {
				cancel()
				startTimer.Stop()
				startDeadline = nil
				h.report(errors.New("SCM control channel closed"))
				return false, 1
			}
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- status
			case svc.Stop, svc.Shutdown:
				if status.State != svc.StopPending {
					cancel()
					status = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: uint32(h.stopTimeout.Milliseconds())}
					statuses <- status
					stopTimer = time.NewTimer(h.stopTimeout)
					stopDeadline = stopTimer.C
				}
			}
		case err := <-done:
			if err == nil && status.State != svc.StopPending {
				err = errors.New("monitoring application exited unexpectedly")
			}
			if err != nil {
				h.report(err)
				return false, 1
			}
			return false, 0
		case <-tick.C:
			if status.State == svc.StartPending || status.State == svc.StopPending {
				status.CheckPoint++
				statuses <- status
			}
		case <-stopDeadline:
			h.report(fmt.Errorf("service stop exceeded %s", h.stopTimeout))
			return false, 1
		case <-startDeadline:
			h.report(fmt.Errorf("service startup exceeded %s", startBudget))
			return false, 1
		}
	}
}

type eventWriter struct{ sink *eventlog.Log }

func (w eventWriter) Write(data []byte) (int, error) {
	if err := w.sink.Info(1, strings.TrimSpace(string(data))); err != nil {
		fmt.Fprintf(os.Stderr, "Windows Event Log write failed: %v\n%s", err, data)
		return 0, err
	}
	return len(data), nil
}

func Run(application Application) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		return errors.New("service run is reserved for Windows Service Control Manager; use serve for console mode")
	}
	sink, err := eventlog.Open(Name)
	if err != nil {
		return fmt.Errorf("open Windows Event Log: %w", err)
	}
	defer sink.Close()
	oldWriter := log.Writer()
	log.SetOutput(eventWriter{sink})
	defer log.SetOutput(oldWriter)
	report := func(err error) {
		if writeErr := sink.Error(1, err.Error()); writeErr != nil {
			fmt.Fprintf(os.Stderr, "Service error: %v; Windows Event Log failed: %v\n", err, writeErr)
		}
	}
	return svc.Run(Name, &handler{application: application, report: report, stopTimeout: 10 * time.Second, startTimeout: 10 * time.Second})
}
