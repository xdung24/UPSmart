package notifications

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"upsmart/internal/notifications/relaypb"
)

type relayService interface {
	SendMessage(context.Context, *relaypb.NotifyRequest) (*relaypb.NotifyResponse, error)
}

type fakeRelay func(context.Context, *relaypb.NotifyRequest) (*relaypb.NotifyResponse, error)

func (f fakeRelay) SendMessage(ctx context.Context, request *relaypb.NotifyRequest) (*relaypb.NotifyResponse, error) {
	return f(ctx, request)
}

func testRelay(t *testing.T, handler fakeRelay) *Sender {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "notify.Notifier",
		HandlerType: (*relayService)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "SendMessage",
			Handler: func(service any, ctx context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				var request relaypb.NotifyRequest
				if err := decode(&request); err != nil {
					return nil, err
				}
				return service.(relayService).SendMessage(ctx, &request)
			},
		}},
	}, handler)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil {
			t.Errorf("serve fake relay: %v", err)
		}
	}()
	t.Cleanup(func() {
		server.Stop()
		<-done
	})
	sender, err := New(Options{RelayAddress: listener.Addr().String(), Events: []string{"power_changed"}})
	if err != nil {
		t.Fatal(err)
	}
	sender.minInterval = 0
	sender.backoff = time.Millisecond
	return sender
}

func TestRelaySendsSelectedLiveEventsWithoutBotCredentials(t *testing.T) {
	messages := make(chan string, 2)
	sender := testRelay(t, func(ctx context.Context, request *relaypb.NotifyRequest) (*relaypb.NotifyResponse, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Error("relay RPC lacks bounded deadline")
		}
		messages <- request.Message
		return &relaypb.NotifyResponse{Sent: true}, nil
	})
	if _, err := NewCommands(sender, []int64{42}, func(string) string { return "status" }); err == nil {
		t.Fatal("relay allowed direct bot polling")
	}
	first := record(1, "power_changed")
	first.Message = "on_battery: [PC] *literal* `text` \\"
	sender.Submit(first)
	sender.Submit(first)
	sender.Submit(record(2, "startup"))
	startSender(t, sender)
	awaitStatus(t, sender, func(s Status) bool { return s.LastSuccess != nil })
	got := <-messages
	want := "UPSmart\n" + first.Time.Format(time.RFC3339) + "\npower\\_changed: on\\_battery: \\[PC] \\*literal\\* \\`text\\` \\\\"
	if got != want {
		t.Fatalf("unexpected relay payload: %q, want %q", got, want)
	}
	if len(messages) != 0 || sender.Status().Dropped != 0 {
		t.Fatal("duplicate or unselected event was sent")
	}
}

func TestRelayFailuresAndRetry(t *testing.T) {
	for _, test := range []struct {
		name     string
		response *relaypb.NotifyResponse
		err      error
		calls    int32
		mode     string
	}{
		{"not sent", &relaypb.NotifyResponse{Error: testToken}, nil, 1, "degraded"},
		{"contradictory response", &relaypb.NotifyResponse{Sent: true, Error: testToken}, nil, 1, "degraded"},
		{"empty response", &relaypb.NotifyResponse{}, nil, 1, "degraded"},
		{"unavailable", nil, status.Error(codes.Unavailable, testToken), 3, "degraded"},
		{"wrong service", nil, status.Error(codes.Unimplemented, testToken), 1, "blocked"},
		{"oversized response", &relaypb.NotifyResponse{Error: strings.Repeat("x", maxBodyBytes+1)}, nil, 1, "degraded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			sender := testRelay(t, func(context.Context, *relaypb.NotifyRequest) (*relaypb.NotifyResponse, error) {
				calls.Add(1)
				return test.response, test.err
			})
			sender.Submit(record(1, "power_changed"))
			startSender(t, sender)
			result := awaitStatus(t, sender, func(s Status) bool { return s.Dropped == 1 })
			if calls.Load() != test.calls || result.Mode != test.mode || result.LastSuccess != nil ||
				result.Error == "" || strings.Contains(result.Error, testToken) {
				t.Fatalf("unsafe or incorrect failure: calls=%d status=%+v", calls.Load(), result)
			}
		})
	}
}

func TestRelayTransientRecovery(t *testing.T) {
	var calls atomic.Int32
	sender := testRelay(t, func(context.Context, *relaypb.NotifyRequest) (*relaypb.NotifyResponse, error) {
		if calls.Add(1) == 1 {
			return nil, status.Error(codes.Unavailable, "temporary")
		}
		return &relaypb.NotifyResponse{Sent: true}, nil
	})
	sender.Submit(record(1, "power_changed"))
	startSender(t, sender)
	result := awaitStatus(t, sender, func(s Status) bool { return s.LastSuccess != nil })
	if calls.Load() != 2 || result.Dropped != 0 || result.Mode != "enabled" {
		t.Fatalf("relay did not recover: calls=%d status=%+v", calls.Load(), result)
	}
}

func TestRelayCancellationAndExpiry(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "expire"}[expire], func(t *testing.T) {
			entered := make(chan struct{}, 1)
			exited := make(chan struct{}, 1)
			sender := testRelay(t, func(ctx context.Context, _ *relaypb.NotifyRequest) (*relaypb.NotifyResponse, error) {
				entered <- struct{}{}
				<-ctx.Done()
				exited <- struct{}{}
				return nil, ctx.Err()
			})
			if expire {
				sender.expiry = 300 * time.Millisecond
			}
			sender.Submit(record(1, "power_changed"))
			cancel, done := startSender(t, sender)
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("relay RPC not started")
			}
			if expire {
				awaitStatus(t, sender, func(s Status) bool { return s.Dropped == 1 })
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("relay worker did not stop")
			}
			select {
			case <-exited:
			case <-time.After(time.Second):
				t.Fatal("relay RPC was not cancelled")
			}
			if s := sender.Status(); s.Dropped != 1 || s.LastSuccess != nil {
				t.Fatalf("cancelled/expired event not accounted for: %+v", s)
			}
		})
	}
}

func TestRelayMarkdownBound(t *testing.T) {
	for _, input := range []string{strings.Repeat("_", 4000), strings.Repeat("ế", 4000), strings.Repeat("\\", 4000)} {
		text := relayMarkdown(input)
		if utf8.RuneCountInString(text) != maxTextRunes || !utf8.ValidString(text) ||
			strings.Count(text, "\\")%2 != 0 && strings.HasSuffix(text, "\\") {
			t.Fatal("relay message exceeded limit or ended with an incomplete escape")
		}
	}
}
