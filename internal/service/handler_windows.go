package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	systemservice "github.com/kardianos/service"
)

type program struct {
	application  Application
	cancel       context.CancelFunc
	done         chan struct{}
	err          error
	startTimeout time.Duration
	stopTimeout  time.Duration
	fatal        func(error)
}

func (p *program) Start(_ systemservice.Service) error {
	if p.application == nil || p.done != nil {
		return errors.New("service application is missing or already started")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	ready := make(chan struct{})
	var once sync.Once
	go func() {
		p.err = p.application(ctx, func() { once.Do(func() { close(ready) }) })
		if p.err == nil && ctx.Err() == nil {
			p.err = errors.New("monitoring application exited unexpectedly")
		}
		close(p.done)
	}()
	budget := p.startTimeout
	if budget <= 0 {
		budget = 10 * time.Second
	}
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case <-ready:
		go func() {
			<-p.done
			if ctx.Err() == nil {
				p.fatal(p.err)
			}
		}()
		return nil
	case <-p.done:
		cancel()
		return p.err
	case <-timer.C:
		cancel()
		return fmt.Errorf("service startup exceeded %s", budget)
	}
}

func (p *program) Stop(_ systemservice.Service) error {
	if p.cancel == nil {
		return errors.New("service application was not started")
	}
	p.cancel()
	budget := p.stopTimeout
	if budget <= 0 {
		budget = 10 * time.Second
	}
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case <-p.done:
		return p.err
	case <-timer.C:
		return fmt.Errorf("service stop exceeded %s", budget)
	}
}

type eventWriter struct{ logger systemservice.Logger }

func (w eventWriter) Write(data []byte) (int, error) {
	if err := w.logger.Info(strings.TrimSpace(string(data))); err != nil {
		fmt.Fprintf(os.Stderr, "Windows Event Log write failed: %v\n%s", err, data)
		return 0, err
	}
	return len(data), nil
}

func Run(configPath string, application Application) error {
	if Interactive() {
		return errors.New("service dispatcher is reserved for Windows Service Control Manager")
	}
	paths, err := deploymentPaths()
	if err != nil {
		return err
	}
	// kardianos/service does not apply WorkingDirectory on Windows.
	if err := os.Chdir(filepath.Dir(configPath)); err != nil {
		return fmt.Errorf("set service working directory: %w", err)
	}
	p := &program{application: application, fatal: func(err error) {
		log.Printf("Service monitoring failed: %v", err)
		os.Exit(1)
	}}
	svc, err := systemservice.New(p, definition(paths, configPath))
	if err != nil {
		return err
	}
	logger, err := svc.Logger(nil)
	if err != nil {
		return fmt.Errorf("open Windows Event Log: %w", err)
	}
	oldWriter := log.Writer()
	log.SetOutput(eventWriter{logger})
	defer log.SetOutput(oldWriter)
	return svc.Run()
}
