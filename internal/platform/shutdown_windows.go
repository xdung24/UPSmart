//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

func Shutdown(ctx context.Context) error {
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("locate Windows shutdown utility: %w", err)
	}
	return requestShutdown(ctx, directory, func(ctx context.Context, path string, args ...string) error {
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.WaitDelay = 200 * time.Millisecond
		return cmd.Run()
	})
}

func requestShutdown(parent context.Context, directory string, run func(context.Context, string, ...string) error) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	// Windows implies /f for a nonzero /t. The application owns the countdown.
	err := run(ctx, filepath.Join(directory, "shutdown.exe"), "/s", "/t", "0", "/d", "p:6:12", "/c", "UPSmart configured Windows shutdown")
	if err != nil {
		if ctx.Err() != nil {
			return errors.New("Windows shutdown request interrupted; OS acceptance is unknown; no automatic retry")
		}
		return fmt.Errorf("Windows shutdown request failed (check account shutdown privilege): %w", err)
	}
	return nil
}
