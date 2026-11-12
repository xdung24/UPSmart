package ups

import (
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
	Devices []Device `json:"devices"`
	Status  string   `json:"status,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type Worker struct {
	Executable string
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

func (w Worker) Run(ctx context.Context, status bool) (Result, error) {
	mode := "discover"
	if status {
		mode = "status"
	}
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
