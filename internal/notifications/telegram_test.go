package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"upsmart/internal/events"
)

const testToken = "123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghi"

func testOptions() Options {
	return Options{
		TokenEnv: "UPSMART_TEST_TELEGRAM_TOKEN", ChatID: "-100123456789",
		Events: []string{"startup", "connected", "connection_error", "power_changed", "low_battery_changed", "fault_changed"},
	}
}

func testSender(t *testing.T, handler http.HandlerFunc) *Sender {
	t.Helper()
	t.Setenv(testOptions().TokenEnv, testToken)
	sender, err := New(testOptions())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	sender.endpoint = server.URL + "/bot" + testToken + "/sendMessage"
	sender.client = server.Client()
	sender.minInterval = 0
	sender.backoff = time.Millisecond
	return sender
}

func startSender(t *testing.T, sender *Sender) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sender.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("sender did not stop after cancellation")
		}
	})
	return cancel, done
}

func record(sequence uint64, kind string) events.Record {
	return events.Record{
		SchemaVersion: 1, SessionID: "live-session", Sequence: sequence,
		Time: time.Now().UTC(), Type: kind, Severity: "info", Message: "Power mode: on_battery",
	}
}

func awaitStatus(t *testing.T, sender *Sender, predicate func(Status) bool) Status {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		status := sender.Status()
		if predicate(status) {
			return status
		}
		select {
		case <-deadline.C:
			t.Fatalf("status condition not reached: %+v", status)
		case <-ticker.C:
		}
	}
}

func TestOptionsAndEnvironmentValidation(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*Options)
	}{
		{"empty events", func(o *Options) { o.Events = nil }},
		{"summary unsupported", func(o *Options) { o.Events = []string{"summary"} }},
		{"stop unsupported", func(o *Options) { o.Events = []string{"stop"} }},
		{"unknown", func(o *Options) { o.Events = []string{"made_up"} }},
		{"duplicate", func(o *Options) { o.Events = []string{"startup", "startup"} }},
		{"literal token", func(o *Options) { o.TokenEnv = testToken }},
		{"invalid destination", func(o *Options) { o.ChatID = "https://example.invalid/" }},
		{"empty destination", func(o *Options) { o.ChatID = "" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			opts := testOptions()
			change.edit(&opts)
			if err := opts.Validate(); err == nil {
				t.Fatal("invalid options accepted")
			} else if strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), opts.ChatID) && opts.ChatID != "" {
				t.Fatalf("error disclosed input: %v", err)
			}
		})
	}
	for _, chat := range []string{"12345", "-100123456789", "@my_channel"} {
		opts := testOptions()
		opts.ChatID = chat
		if err := opts.Validate(); err != nil {
			t.Fatalf("valid destination rejected: %v", err)
		}
	}
	for _, token := range []string{"", "literal-secret", testToken + "/redirect", "12345:short"} {
		t.Setenv(testOptions().TokenEnv, token)
		if _, err := New(testOptions()); err == nil {
			t.Fatal("invalid token accepted")
		} else if token != "" && strings.Contains(err.Error(), token) {
			t.Fatalf("error disclosed secret: %v", err)
		}
	}
	opts := testOptions()
	opts.TokenEnv = "UPSMART_TEST_TELEGRAM_MISSING_TOKEN"
	t.Setenv(opts.TokenEnv, "")
	if err := os.Unsetenv(opts.TokenEnv); err != nil {
		t.Fatal(err)
	}
	if _, err := New(opts); err == nil {
		t.Fatal("missing token accepted")
	}
}

func TestNewSnapshotsSecretAndUsesFixedProductionEndpoint(t *testing.T) {
	t.Setenv(testOptions().TokenEnv, testToken)
	sender, err := New(testOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(testOptions().TokenEnv, "invalid-changed-secret")
	if sender.token != testToken || sender.endpoint != "https://api.telegram.org/bot"+testToken+"/sendMessage" {
		t.Fatal("secret was not snapshotted or production destination is not fixed HTTPS")
	}
	if sender.client.Timeout <= 0 || sender.client.Timeout > 10*time.Second {
		t.Fatal("production client has no bounded timeout")
	}
}

func TestSuccessFilteringDeduplicationAndPlainText(t *testing.T) {
	var mu sync.Mutex
	var messages []string
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected JSON POST")
		}
		if r.URL.Path != "/bot"+testToken+"/sendMessage" {
			t.Error("unexpected API path")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if len(payload) != 2 || payload["chat_id"] != testOptions().ChatID {
			t.Error("expected only destination and plain text, without parse_mode")
		}
		mu.Lock()
		messages = append(messages, payload["text"].(string))
		mu.Unlock()
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	})
	sender.selected = map[string]bool{"power_changed": true}
	first := record(1, "power_changed")
	first.Message = "Power mode: on_battery <b>plain text</b> " + testToken + " " + testOptions().ChatID + " " + strings.Repeat("ế", 5000)
	sender.Submit(first)
	sender.Submit(first)
	sender.Submit(record(2, "startup"))
	sender.Submit(record(3, "notification_failure"))
	next := record(1, "power_changed")
	next.SessionID = "other-live-session"
	sender.Submit(next)
	if len(sender.queue) != 2 {
		t.Fatalf("filter/dedup queued %d events, want 2", len(sender.queue))
	}
	startSender(t, sender)
	awaitStatus(t, sender, func(status Status) bool {
		mu.Lock()
		defer mu.Unlock()
		return len(messages) == 2 && status.LastSuccess != nil
	})
	mu.Lock()
	defer mu.Unlock()
	for _, message := range messages {
		if utf8.RuneCountInString(message) > maxTextRunes || !utf8.ValidString(message) {
			t.Error("message not bounded valid UTF-8")
		}
		if strings.Contains(message, testToken) || strings.Contains(message, testOptions().ChatID) {
			t.Error("message disclosed embedded credential")
		}
	}
	if !strings.Contains(messages[0], first.Time.Format(time.RFC3339)) || !strings.Contains(messages[0], "<b>plain text</b>") {
		t.Error("timestamp or literal plain text missing")
	}
	if !strings.HasPrefix(messages[0], "🔋 Mains power lost - running on battery\n🕒 ") {
		t.Errorf("power event did not use the outage icon/title: %q", messages[0])
	}
	status := sender.Status()
	*status.LastSuccess = time.Time{}
	if sender.Status().LastSuccess.IsZero() {
		t.Error("caller mutated internal status timestamp")
	}
}

func TestNotificationFormattingUsesDistinctEventIcons(t *testing.T) {
	now := time.Date(2026, 11, 22, 1, 2, 3, 0, time.UTC)
	for _, test := range []struct {
		kind, message, prefix string
	}{
		{"power_changed", "Power mode: on_battery", "🔋 Mains power lost - running on battery\n"},
		{"power_changed", "Power mode: online", "⚡ Mains power restored\n"},
		{"connected", "Verified Q1 response received", "✅ UPS connected\n"},
		{"connection_error", "HID helper failed", "⚠️ UPS connection problem\n"},
		{"fault_changed", "UPS fault: true", "🚨 UPS fault status changed\n"},
		{"unknown_event", "Example", "🔔 Notification\n"},
	} {
		text := formatNotification(test.kind, test.message, now)
		if !strings.HasPrefix(text, test.prefix+"🕒 "+now.Format(time.RFC3339)+"\n") {
			t.Errorf("%s formatted unexpectedly: %q", test.kind, text)
		}
	}
}

func TestExplicitTelegramTestWaitsForAcceptance(t *testing.T) {
	messages := make(chan string, 1)
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		messages <- payload["text"].(string)
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	})
	startSender(t, sender)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sender.Test(ctx); err != nil {
		t.Fatal(err)
	}
	message := <-messages
	if !strings.HasPrefix(message, "🧪 UPSmart test notification\n🕒 ") ||
		!strings.Contains(message, "Test notification requested from desktop settings") ||
		strings.Contains(message, testToken) {
		t.Fatalf("unexpected test notification: %q", message)
	}
	if sender.Status().LastSuccess == nil {
		t.Fatal("accepted test did not update delivery health")
	}
}

func TestRejectedResponsesBlockWithoutRetryAndRedact(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
		body string
	}{
		{"false HTTP200", 200, `{"ok":false,"description":"secret"}`},
		{"false HTTP200 auth", 200, `{"ok":false,"error_code":401}`},
		{"auth", 401, `{"ok":false,"error_code":401}`},
		{"destination", 400, `{"ok":false,"error_code":400}`},
		{"forbidden", 403, `{"ok":false,"error_code":403}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.code)
				fmt.Fprint(w, strings.TrimSuffix(test.body, "}")+`,"untrusted":"`+testToken+" "+testOptions().ChatID+` https://sensitive.invalid/"}`)
			})
			sender.Submit(record(1, "startup"))
			sender.Submit(record(2, "connected"))
			_, done := startSender(t, sender)
			<-done
			status := sender.Status()
			if status.Mode != "blocked" || status.LastSuccess != nil || status.Dropped != 2 || calls.Load() != 1 {
				t.Fatalf("unexpected blocked state: %+v; requests=%d", status, calls.Load())
			}
			raw, _ := json.Marshal(status)
			for _, secret := range []string{testToken, testOptions().ChatID, "https://", "sensitive.invalid"} {
				if strings.Contains(string(raw), secret) {
					t.Error("status leaked sensitive response data")
				}
			}
			sender.Submit(record(3, "power_changed"))
			if sender.Status().Dropped != 3 || calls.Load() != 1 {
				t.Error("blocked sender accepted more work")
			}
		})
	}
}

func TestRateLimitHonorsRetryAfter(t *testing.T) {
	var calls atomic.Int32
	var mu sync.Mutex
	var received []time.Time
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, time.Now())
		mu.Unlock()
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":1}}`)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})
	sender.Submit(record(1, "startup"))
	startSender(t, sender)
	status := awaitStatus(t, sender, func(status Status) bool { return status.LastSuccess != nil })
	if status.Dropped != 0 || calls.Load() != 2 {
		t.Fatalf("rate-limited delivery did not recover: %+v", status)
	}
	mu.Lock()
	defer mu.Unlock()
	if received[1].Sub(received[0]) < time.Second {
		t.Error("retry happened earlier than retry_after")
	}
}

func TestRateLimitBeyondExpiryDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":9223372036854775807}}`)
	})
	sender.Submit(record(1, "startup"))
	startSender(t, sender)
	status := awaitStatus(t, sender, func(status Status) bool { return status.Dropped == 1 })
	if calls.Load() != 1 || status.LastSuccess != nil || !strings.Contains(status.Error, "expiry") {
		t.Fatalf("unbounded rate-limit retry: %+v; requests=%d", status, calls.Load())
	}
}

func TestTransientFailureRecoversAndWorkerSerializesRequests(t *testing.T) {
	var calls, active, maximum atomic.Int32
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"ok":false}`)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	})
	for sequence := 1; sequence <= 5; sequence++ {
		sender.Submit(record(uint64(sequence), "power_changed"))
	}
	startSender(t, sender)
	awaitStatus(t, sender, func(status Status) bool { return calls.Load() == 6 && status.LastSuccess != nil })
	if maximum.Load() != 1 || sender.Status().Dropped != 0 {
		t.Fatalf("worker not serial or retry lost an event: maximum=%d status=%+v", maximum.Load(), sender.Status())
	}
}

func TestTransientAndInvalidResponsesHaveBoundedRetries(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
		body string
	}{
		{"server failure", 503, `{"ok":false,"error_code":503}`},
		{"malformed", 200, `{"ok":`},
		{"missing ok", 200, `{}`},
		{"oversized", 200, `{"ok":true,"padding":"` + strings.Repeat("x", maxBodyBytes) + `"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.code)
				fmt.Fprint(w, test.body)
			})
			sender.Submit(record(1, "startup"))
			startSender(t, sender)
			status := awaitStatus(t, sender, func(status Status) bool { return status.Dropped == 1 })
			if calls.Load() != int32(sender.attempts) || status.LastSuccess != nil || status.Error == "" {
				t.Fatalf("retry budget not enforced: %+v; requests=%d", status, calls.Load())
			}
		})
	}
}

func TestRedirectIsNeverFollowed(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalls.Add(1)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer destination.Close()
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/bot"+testToken+"/sendMessage", http.StatusTemporaryRedirect)
	})
	// Even a caller-supplied redirect policy must not bypass the worker's policy.
	sender.client.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	sender.Submit(record(1, "startup"))
	startSender(t, sender)
	status := awaitStatus(t, sender, func(status Status) bool { return status.Mode == "blocked" })
	if destinationCalls.Load() != 0 || !strings.Contains(status.Error, "redirect") {
		t.Fatalf("credential-bearing redirect followed or hidden: %+v", status)
	}
}

func TestCancellationStopsActiveRequestAndCountsQueuedEvents(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	release := make(chan struct{})
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		defer close(exited)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	sender.Submit(record(1, "startup"))
	sender.Submit(record(2, "connected"))
	cancel, done := startSender(t, sender)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker never sent request")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("active request not cancelled")
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("cancelled connection remained active at server")
	}
	status := sender.Status()
	if status.Mode != "stopped" || status.Dropped != 2 || status.LastSuccess != nil {
		t.Fatalf("cancelled work not accounted for: %+v", status)
	}
	sender.Submit(record(3, "power_changed"))
	if sender.Status().Dropped != 3 {
		t.Fatal("submission after stop was not counted")
	}
}

func TestShutdownClosesIdleConnections(t *testing.T) {
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	t.Setenv(testOptions().TokenEnv, testToken)
	sender, err := New(testOptions())
	if err != nil {
		t.Fatal(err)
	}
	sender.endpoint = server.URL + "/sendMessage"
	sender.Submit(record(1, "startup"))
	cancel, done := startSender(t, sender)
	awaitStatus(t, sender, func(status Status) bool { return status.LastSuccess != nil })
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("idle transport connection survived worker shutdown")
	}
}

func TestExpiryCancelsActiveRequest(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	sender.expiry = 150 * time.Millisecond
	sender.Submit(record(1, "startup"))
	startSender(t, sender)
	status := awaitStatus(t, sender, func(status Status) bool { return status.Dropped == 1 })
	if calls.Load() != 1 || status.LastSuccess != nil {
		t.Fatalf("in-flight request did not expire: %+v; requests=%d", status, calls.Load())
	}
}

func TestCancellationInterruptsRateLimitWait(t *testing.T) {
	var calls atomic.Int32
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":60}}`)
	})
	sender.Submit(record(1, "startup"))
	cancel, done := startSender(t, sender)
	awaitStatus(t, sender, func(Status) bool { return calls.Load() == 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rate-limit wait ignored cancellation")
	}
	if calls.Load() != 1 || sender.Status().Dropped != 1 {
		t.Fatal("cancelled retry was sent or not counted")
	}
}

func TestQueueOverflowNonblockingAndDeduplicated(t *testing.T) {
	var calls atomic.Int32
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"ok":true}`)
	})
	started := time.Now()
	for sequence := 1; sequence <= queueCapacity+5; sequence++ {
		item := record(uint64(sequence), "power_changed")
		sender.Submit(item)
		sender.Submit(item)
	}
	if time.Since(started) > time.Second {
		t.Error("submission blocked without a worker")
	}
	status := sender.Status()
	if len(sender.queue) != queueCapacity || status.Dropped != 5 || status.Error == "" || calls.Load() != 0 {
		t.Fatalf("overflow not bounded/visible: %+v; queued=%d", status, len(sender.queue))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sender.Run(ctx)
	if sender.Status().Dropped != queueCapacity+5 || len(sender.queue) != 0 || calls.Load() != 0 {
		t.Error("cancelled queue was not discarded without network I/O")
	}
}

func TestExpiryAndInvalidIdentityNeverSend(t *testing.T) {
	var calls atomic.Int32
	sender := testSender(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"ok":true}`)
	})
	expired := record(1, "startup")
	expired.Time = time.Now().Add(-10 * time.Minute)
	sender.Submit(expired)
	invalid := record(2, "startup")
	invalid.SessionID = ""
	sender.Submit(invalid)
	invalid = record(3, "startup")
	invalid.Time = time.Time{}
	sender.Submit(invalid)
	if sender.Status().Dropped != 3 || len(sender.queue) != 0 || calls.Load() != 0 {
		t.Fatal("expired or unidentified event was accepted")
	}
	// An event can also expire after it has been queued.
	sender.queue <- pending{text: "old alert", expires: time.Now().Add(-time.Second)}
	startSender(t, sender)
	awaitStatus(t, sender, func(status Status) bool { return status.Dropped == 4 })
	if calls.Load() != 0 {
		t.Fatal("queued stale event was sent")
	}
}
