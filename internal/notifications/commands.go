package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CommandStatus struct {
	Mode     string `json:"mode"`
	Error    string `json:"error,omitempty"`
	Rejected uint64 `json:"rejected_commands"`
}

type telegramUser struct {
	ID    int64 `json:"id"`
	IsBot bool  `json:"is_bot"`
}

type telegramMessage struct {
	Date int64         `json:"date"`
	From *telegramUser `json:"from"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	Text          string          `json:"text"`
	ForwardOrigin json.RawMessage `json:"forward_origin"`
	ForwardDate   int64           `json:"forward_date"`
	ViaBot        *telegramUser   `json:"via_bot"`
	SenderChat    json.RawMessage `json:"sender_chat"`
}

type telegramUpdate struct {
	ID      int64            `json:"update_id"`
	Message *telegramMessage `json:"message"`
}

// Commands owns read-only polling; only Status is concurrent.
// Replies use Sender's existing bounded, serial, rate-limited delivery worker.
type Commands struct {
	mu        sync.Mutex
	status    CommandStatus
	started   bool
	sender    *Sender
	read      func(string) string
	chatID    int64
	users     map[int64]bool
	endpoint  string
	client    *http.Client
	createdAt time.Time
}

func NewCommands(sender *Sender, allowed []int64, read func(string) string) (*Commands, error) {
	if sender == nil || read == nil {
		return nil, errors.New("Telegram commands require a sender and status reader")
	}
	chatID, err := strconv.ParseInt(sender.chatID, 10, 64)
	if err != nil || chatID == 0 || len(allowed) == 0 || len(allowed) > 16 {
		return nil, errors.New("Telegram commands require a numeric chat and explicit allowed user IDs")
	}
	users := make(map[int64]bool, len(allowed))
	for _, id := range allowed {
		if id <= 0 || users[id] {
			return nil, errors.New("Telegram allowed user IDs must be positive and unique")
		}
		users[id] = true
	}
	client := *sender.client
	if transport, ok := client.Transport.(*http.Transport); ok {
		client.Transport = transport.Clone()
	}
	client.Timeout = 10 * time.Second
	client.CheckRedirect = noRedirect
	return &Commands{
		status: CommandStatus{Mode: "starting"}, sender: sender, read: read,
		chatID: chatID, users: users,
		endpoint: "https://api.telegram.org/bot" + sender.token + "/getUpdates",
		client:   &client, createdAt: time.Now(),
	}, nil
}

func (b *Commands) Status() CommandStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

func (b *Commands) report(mode, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.Mode, b.status.Error = mode, reason
}

func (b *Commands) reject() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.Rejected++
}

func (b *Commands) reply(text string) {
	if err := b.sender.Reply(text); err != nil {
		b.report("degraded", err.Error())
	}
}

func hasObject(raw json.RawMessage) bool {
	return len(raw) != 0 && string(raw) != "null"
}

func (b *Commands) handle(update telegramUpdate, now time.Time) {
	m := update.Message
	if m == nil {
		return // Edited messages, channel posts and callback queries are not commands.
	}
	age := now.Sub(time.Unix(m.Date, 0))
	if m.From == nil || m.From.IsBot || !b.users[m.From.ID] || m.Chat.ID != b.chatID ||
		m.Date < b.createdAt.Unix() || age > time.Minute || age < -5*time.Second ||
		hasObject(m.ForwardOrigin) || m.ForwardDate != 0 || m.ViaBot != nil || hasObject(m.SenderChat) {
		b.reject()
		return
	}
	parts := strings.Fields(m.Text)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return
	}
	switch parts[0] {
	case "/status", "/ups", "/battery", "/hardware":
		if len(parts) == 1 {
			b.reply(b.read(parts[0]))
			return
		}
	}
	b.reply("Read-only commands: /status, /ups, /battery, /hardware, /help. Power and configuration changes are desktop-only.")
}

type pollFailure struct {
	reason    string
	permanent bool
	wait      time.Duration
}

func (e *pollFailure) Error() string { return e.reason }

func (b *Commands) poll(ctx context.Context, offset int64, bootstrap bool) ([]telegramUpdate, *pollFailure) {
	timeout, limit := 5, 20
	if bootstrap {
		timeout, limit = 0, 1
	}
	body, err := json.Marshal(struct {
		Offset  int64    `json:"offset"`
		Timeout int      `json:"timeout"`
		Limit   int      `json:"limit"`
		Allowed []string `json:"allowed_updates"`
	}{offset, timeout, limit, []string{"message"}})
	if err != nil {
		return nil, &pollFailure{reason: "Telegram polling payload could not be encoded", permanent: true}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &pollFailure{reason: "Telegram polling request could not be constructed", permanent: true}
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(req)
	if err != nil {
		return nil, &pollFailure{reason: "Telegram polling network request failed"}
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	closeErr := response.Body.Close()
	var result struct {
		OK         *bool            `json:"ok"`
		Result     []telegramUpdate `json:"result"`
		ErrorCode  int              `json:"error_code"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	valid := readErr == nil && closeErr == nil && len(raw) <= 256<<10 && json.Unmarshal(raw, &result) == nil
	code := response.StatusCode
	if valid && result.OK != nil && !*result.OK && result.ErrorCode != 0 {
		code = result.ErrorCode
	}
	switch {
	case response.StatusCode >= 300 && response.StatusCode < 400:
		return nil, &pollFailure{reason: "Telegram polling redirect refused", permanent: true}
	case code == http.StatusTooManyRequests:
		if valid && result.Parameters.RetryAfter > 0 && result.Parameters.RetryAfter <= 3600 {
			return nil, &pollFailure{reason: "Telegram polling rate limited", wait: time.Duration(result.Parameters.RetryAfter) * time.Second}
		}
		return nil, &pollFailure{reason: "Telegram polling rate limit could not be safely honored; restart after checking setup", permanent: true}
	case code == http.StatusConflict:
		return nil, &pollFailure{reason: "Telegram polling conflict; check for another poller or an existing webhook", permanent: true}
	case code >= 500 || code == http.StatusRequestTimeout:
		return nil, &pollFailure{reason: "Telegram polling temporarily unavailable"}
	case code < 200 || code >= 300:
		return nil, &pollFailure{reason: "Telegram polling rejected; check authentication/setup", permanent: true}
	case !valid || result.OK == nil || !*result.OK || result.Result == nil || len(result.Result) > limit:
		return nil, &pollFailure{reason: "Telegram polling response was invalid"}
	}
	var last int64 = -1
	for _, update := range result.Result {
		if update.ID < 0 || update.ID >= 1<<62 || update.ID <= last {
			return nil, &pollFailure{reason: "Telegram update identities were invalid"}
		}
		last = update.ID
	}
	return result.Result, nil
}

// Run drops the pre-start backlog before accepting any commands. A reconnect
// exceeding the message age limit cannot replay old requests.
func (b *Commands) Run(ctx context.Context) {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return
	}
	b.started = true
	b.mu.Unlock()
	defer b.client.CloseIdleConnections()
	defer func() {
		if b.Status().Mode != "blocked" {
			b.report("stopped", b.Status().Error)
		}
	}()
	var offset int64 = -1
	bootstrap := true
	delay := time.Second
	for ctx.Err() == nil {
		updates, failure := b.poll(ctx, offset, bootstrap)
		if ctx.Err() != nil {
			return
		}
		if failure != nil {
			if failure.permanent {
				b.report("blocked", failure.reason)
				return
			}
			b.report("degraded", failure.reason)
			wait := max(delay, failure.wait)
			if !waitUntil(ctx, time.Now().Add(wait)) {
				return
			}
			delay = min(delay*2, 30*time.Second)
			continue
		}
		delay = time.Second
		b.report("polling", "")
		for _, update := range updates {
			if !bootstrap && update.ID < offset {
				continue
			}
			offset = update.ID + 1
			if !bootstrap {
				b.handle(update, time.Now())
			}
		}
		if bootstrap {
			bootstrap = false
			if offset == -1 {
				offset = 0
			}
		}
		// Also bounds a broken server that returns empty/duplicate batches immediately.
		if !waitUntil(ctx, time.Now().Add(250*time.Millisecond)) {
			return
		}
	}
}
