package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"upsmart/internal/state"
)

var ErrInvalidWebResponse = errors.New("invalid web API response")

type WebClient struct {
	address string
	http    *http.Client
}

func NewWebClient(address string) (*WebClient, error) {
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host != "127.0.0.1" || portErr != nil || number < 1 || number > 65535 {
		return nil, errors.New("web address must be 127.0.0.1:<port>; LAN access is not supported")
	}
	return &WebClient{address: address, http: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			DialContext:     (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
			MaxConnsPerHost: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 10 * time.Second,
		},
	}}, nil
}

func (c *WebClient) Close() { c.http.CloseIdleConnections() }

func (c *WebClient) Request(ctx context.Context, method, path string, body io.Reader, result any) error {
	if method != http.MethodGet || path != "/snapshot" || body != nil {
		return errors.New("web connection is read-only; policy editing and power controls require the local named pipe, not a Windows service")
	}
	output, ok := result.(*Snapshot)
	if !ok || output == nil {
		return errors.New("web snapshot requires a snapshot output")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var snapshot Snapshot
	if err := c.get(ctx, "/api/v1/status", &snapshot.UPS); err != nil {
		return err
	}
	if snapshot.UPS.SchemaVersion != 1 {
		return fmt.Errorf("%w: unsupported snapshot schema %d", ErrInvalidWebResponse, snapshot.UPS.SchemaVersion)
	}
	var hardware *state.HardwareStatus
	if err := c.get(ctx, "/api/v1/hardware", &hardware); err != nil {
		return err
	}
	if hardware == nil || hardware.Mode == "" {
		return fmt.Errorf("%w: missing hardware status", ErrInvalidWebResponse)
	}
	var events *state.EventPage
	if err := c.get(ctx, "/api/v1/events?limit=100", &events); err != nil {
		return err
	}
	if events == nil {
		return fmt.Errorf("%w: missing event history", ErrInvalidWebResponse)
	}
	snapshot.Hardware, snapshot.Events = *hardware, *events
	*output = snapshot
	return nil
}

func (c *WebClient) get(ctx context.Context, path string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+c.address+path, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("web API %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("web API %s returned HTTP %d", path, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return fmt.Errorf("read web API %s: %w", path, err)
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("%w: %s exceeds 1 MiB", ErrInvalidWebResponse, path)
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("%w: decode %s: %w", ErrInvalidWebResponse, path, err)
	}
	return nil
}
