package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"upsmart/internal/control"
)

type Client interface {
	Request(context.Context, string, string, io.Reader, any) error
}

type Session struct {
	client Client
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

// Connect chooses an owner once. Connection loss never silently transfers control.
func Connect(ctx context.Context, client Client, start func(context.Context, func()) error) (*Session, error) {
	probe := func() (bool, error) {
		var snapshot control.Snapshot
		if err := client.Request(ctx, http.MethodGet, "/snapshot", nil, &snapshot); err != nil {
			return false, err
		}
		if snapshot.UPS.SchemaVersion != 1 {
			return true, fmt.Errorf("unsupported monitor snapshot schema %d", snapshot.UPS.SchemaVersion)
		}
		return true, nil
	}
	reachable, probeErr := probe()
	if reachable {
		if probeErr != nil {
			return nil, probeErr
		}
		return &Session{client: client}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directCtx, cancel := context.WithCancel(ctx)
	session := &Session{client: client, cancel: cancel, done: make(chan struct{})}
	ready := make(chan struct{})
	go func() {
		session.err = start(directCtx, func() { close(ready) })
		close(session.done)
	}()
	select {
	case <-ready:
		select {
		case <-session.done:
		default:
			return session, nil
		}
	case <-session.done:
	case <-ctx.Done():
	}
	stopErr := session.Close()
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, stopErr)
	}
	// A daemon may have acquired ownership between the initial probe and startup.
	if _, err := probe(); err == nil {
		return &Session{client: client}, nil
	}
	if stopErr == nil {
		stopErr = errors.New("direct monitor stopped before becoming available")
	}
	return nil, fmt.Errorf("daemon unavailable (%v); direct USB startup failed: %w", probeErr, stopErr)
}

func (s *Session) Direct() bool { return s.done != nil }

func (s *Session) Request(ctx context.Context, method, path string, body io.Reader, result any) error {
	if s.done != nil {
		select {
		case <-s.done:
			if s.err != nil {
				return fmt.Errorf("direct USB monitor stopped; restart the desktop to reconnect: %w", s.err)
			}
			return errors.New("direct USB monitor stopped; restart the desktop to reconnect")
		default:
		}
	}
	return s.client.Request(ctx, method, path, body, result)
}

func (s *Session) Close() error {
	if s.cancel == nil {
		return nil
	}
	s.cancel()
	<-s.done
	if s.err == context.Canceled {
		return nil
	}
	return s.err
}
