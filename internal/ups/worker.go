package ups

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

type Device struct {
	VendorID           string   `json:"vendor_id"`
	ProductID          string   `json:"product_id"`
	Product            string   `json:"product"`
	Manufacturer       string   `json:"manufacturer"`
	DescriptorWarnings []string `json:"descriptor_warnings,omitempty"`
	Capabilities       []string `json:"capabilities"`
}

type Result struct {
	Devices            []Device `json:"devices"`
	Status             string   `json:"status,omitempty"`
	WindowsACLineState string   `json:"windows_ac_line_state,omitempty"`
	Error              string   `json:"error,omitempty"`
	RefreshOnly        bool     `json:"refresh_only,omitempty"`
}

type Worker struct {
	Executable string
	session    *helperSession
	lastResult Result
}

type limitedBuffer struct {
	bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 32768 {
		return 0, errors.New("HID helper output exceeded 32 KiB")
	}
	return b.Buffer.Write(p)
}

type helperLine struct {
	data []byte
	err  error
}

type helperSession struct {
	command *exec.Cmd
	input   io.WriteCloser
	lines   chan helperLine
	stderr  limitedBuffer
}

func startHelperSession(executable string) (*helperSession, error) {
	cmd := exec.Command(executable, "__hid", "session")
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	session := &helperSession{command: cmd, input: input, lines: make(chan helperLine, 1)}
	cmd.Stderr = &session.stderr
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		return nil, err
	}
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 1024), 32769)
		for scanner.Scan() {
			session.lines <- helperLine{data: append([]byte(nil), scanner.Bytes()...)}
		}
		session.lines <- helperLine{err: scanner.Err()}
		close(session.lines)
	}()
	return session, nil
}

func (s *helperSession) stop() error {
	_ = s.input.Close()
	if s.command.Process != nil {
		_ = s.command.Process.Kill()
	}
	return s.command.Wait()
}

func (w *Worker) Close() error {
	if w.session == nil {
		return nil
	}
	err := w.session.stop()
	w.session = nil
	return err
}

func (w *Worker) sessionRun(ctx context.Context) (Result, error) {
	startupRefreshes := 0
	for {
		if w.session == nil {
			session, err := startHelperSession(w.Executable)
			if err != nil {
				return Result{}, err
			}
			w.session = session
		}
		if _, err := io.WriteString(w.session.input, "status\n"); err != nil {
			_ = w.Close()
			return Result{}, fmt.Errorf("HID helper session write failed: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = w.Close()
			return Result{}, fmt.Errorf("HID helper: %w", ctx.Err())
		case line, ok := <-w.session.lines:
			if !ok || line.err != nil {
				_ = w.Close()
				return Result{}, errors.New("HID helper session ended before returning status")
			}
			var result Result
			d := json.NewDecoder(bytes.NewReader(line.data))
			if err := d.Decode(&result); err != nil {
				_ = w.Close()
				return result, fmt.Errorf("decode HID helper: %w", err)
			}
			if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
				_ = w.Close()
				return Result{}, errors.New("unexpected trailing HID helper output")
			}
			if result.Error != "" {
				return result, errors.New(result.Error)
			}
			if result.RefreshOnly {
				if w.lastResult.Status != "" {
					return w.lastResult, nil
				}
				startupRefreshes++
				if startupRefreshes >= 6 {
					return result, errors.New("HID refresh did not produce a valid initial Q1 sample")
				}
				continue
			}
			w.lastResult = result
			return result, nil
		}
	}
}

func (w *Worker) Run(ctx context.Context, status bool) (Result, error) {
	if status {
		return w.sessionRun(ctx)
	}
	mode := "discover"
	cmd := exec.CommandContext(ctx, w.Executable, "__hid", mode)
	var stdout, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// Native indexed-string calls have no reliable Go-context cancellation.
	// Terminating this isolated process bounds native work without leaking threads.
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Result{}, fmt.Errorf("HID helper: %w", ctx.Err())
		}
		return Result{}, fmt.Errorf("HID helper failed: %w: %s", err, stderr.String())
	}
	var result Result
	d := json.NewDecoder(&stdout)
	if err := d.Decode(&result); err != nil {
		return result, fmt.Errorf("decode HID helper: %w", err)
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Result{}, errors.New("unexpected trailing HID helper output")
	}
	if result.Error != "" {
		return result, errors.New(result.Error)
	}
	return result, nil
}
