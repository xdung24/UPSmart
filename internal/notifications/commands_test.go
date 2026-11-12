package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func commandBot(t *testing.T, handler http.HandlerFunc) (*Commands, *atomic.Int32) {
	t.Helper()
	sender := testSender(t, handler)
	calls := new(atomic.Int32)
	bot, err := NewCommands(sender, []int64{42, 43}, func(name string) string {
		calls.Add(1)
		return "Cached UPS status for " + name
	})
	if err != nil {
		t.Fatal(err)
	}
	bot.endpoint = strings.TrimSuffix(sender.endpoint, "/sendMessage") + "/getUpdates"
	return bot, calls
}

func command(text string, userID int64, now time.Time) telegramUpdate {
	message := &telegramMessage{Text: text, Date: now.Unix(), From: &telegramUser{ID: userID}}
	message.Chat.ID = -100123456789
	return telegramUpdate{ID: 1, Message: message}
}

func queuedReply(t *testing.T, bot *Commands) string {
	t.Helper()
	select {
	case item := <-bot.sender.queue:
		return item.text
	default:
		t.Fatal("command did not produce a reply")
		return ""
	}
}

func TestTelegramCommandsAreCheckingOnly(t *testing.T) {
	bot, calls := commandBot(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network call") })
	for _, name := range []string{"/status", "/ups", "/battery", "/hardware"} {
		bot.handle(command(name, 42, bot.createdAt), bot.createdAt)
		if !strings.Contains(queuedReply(t, bot), "Cached UPS status for "+name) {
			t.Fatal("checking command did not read cached data")
		}
	}
	for _, name := range []string{"/shutdown", "/confirm code", "/cancel", "/status extra", "/status@otherbot"} {
		bot.handle(command(name, 42, bot.createdAt), bot.createdAt)
		if !strings.Contains(queuedReply(t, bot), "desktop-only") {
			t.Fatal("mutating/invalid command did not explain the read-only boundary")
		}
	}
	if calls.Load() != 4 {
		t.Fatal("power/invalid commands entered the status reader")
	}
}

func TestCommandAuthorizationRejectsUnsafeSourcesWithoutReplies(t *testing.T) {
	for _, scenario := range []string{"wrong user", "wrong chat", "bot", "anonymous", "forward", "legacy forward", "via bot", "stale", "future", "prestart", "edited"} {
		t.Run(scenario, func(t *testing.T) {
			bot, calls := commandBot(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network call") })
			now := bot.createdAt
			update := command("/status", 42, now)
			switch scenario {
			case "wrong user":
				update.Message.From.ID = 44
			case "wrong chat":
				update.Message.Chat.ID++
			case "bot":
				update.Message.From.IsBot = true
			case "anonymous":
				update.Message.SenderChat = json.RawMessage(`{"id":123}`)
			case "forward":
				update.Message.ForwardOrigin = json.RawMessage(`{"type":"user"}`)
			case "legacy forward":
				update.Message.ForwardDate = now.Unix()
			case "via bot":
				update.Message.ViaBot = &telegramUser{ID: 1}
			case "stale":
				now = now.Add(61 * time.Second)
			case "future":
				update.Message.Date = now.Add(7 * time.Second).Unix()
			case "prestart":
				update.Message.Date--
			case "edited":
				update.Message = nil
			}
			bot.handle(update, now)
			if calls.Load() != 0 || len(bot.sender.queue) != 0 {
				t.Fatal("unauthorized source read status or received a reply")
			}
		})
	}
}

func TestPollingDropsBacklogAndDuplicatesAndStops(t *testing.T) {
	var requests atomic.Int32
	var handlerErr atomic.Bool
	bot, calls := commandBot(t, func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Offset int64 `json:"offset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			handlerErr.Store(true)
		}
		n := requests.Add(1)
		update := command("/status", 42, time.Now())
		switch n {
		case 1:
			if payload.Offset != -1 {
				handlerErr.Store(true)
			}
			update.ID = 100
		case 2:
			if payload.Offset != 101 {
				handlerErr.Store(true)
			}
			update.ID = 101
		case 3:
			if payload.Offset != 102 {
				handlerErr.Store(true)
			}
			update.ID = 101
		default:
			<-r.Context().Done()
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []telegramUpdate{update}}); err != nil {
			handlerErr.Store(true)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); bot.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("poller did not stop")
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for requests.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if calls.Load() != 1 || requests.Load() < 4 || handlerErr.Load() {
		t.Fatalf("backlog/replay protection failed: reads=%d requests=%d error=%t", calls.Load(), requests.Load(), handlerErr.Load())
	}
	if bot.Status().Mode != "stopped" {
		t.Fatal("poller did not expose stop")
	}
}

func TestPollingFailuresAreBoundedAndRedacted(t *testing.T) {
	for _, test := range []struct {
		name      string
		code      int
		body      string
		permanent bool
		wait      time.Duration
	}{
		{"auth", 401, testToken, true, 0},
		{"conflict", 200, `{"ok":false,"error_code":409,"description":"` + testToken + `"}`, true, 0},
		{"redirect", 302, "", true, 0},
		{"rate limit", 429, `{"ok":false,"parameters":{"retry_after":5}}`, false, 5 * time.Second},
		{"server", 503, testToken, false, 0},
		{"malformed", 200, "{", false, 0},
		{"descending IDs", 200, `{"ok":true,"result":[{"update_id":4},{"update_id":3}]}`, false, 0},
		{"oversized", 200, strings.Repeat("x", (256<<10)+1), false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			bot, _ := commandBot(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "https://example.invalid/"+testToken)
				w.WriteHeader(test.code)
				fmt.Fprint(w, test.body)
			})
			updates, failure := bot.poll(context.Background(), 1, false)
			if failure == nil || updates != nil || failure.permanent != test.permanent || failure.wait != test.wait {
				t.Fatalf("unexpected polling failure: %+v", failure)
			}
			if strings.Contains(failure.Error(), testToken) || strings.Contains(failure.Error(), testOptions().ChatID) {
				t.Fatal("polling error disclosed private data")
			}
		})
	}
}
