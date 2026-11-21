// Package notifications provides opt-in, best-effort Telegram alerts for live
// monitoring events. It has no history replay or periodic summary scheduler.
package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"upsmart/internal/events"
)

const (
	queueCapacity = 64
	maxSessions   = 16
	maxBodyBytes  = 32 << 10
	maxTextRunes  = 3500
)

var (
	envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	tokenPattern   = regexp.MustCompile(`^[0-9]{5,20}:[A-Za-z0-9_-]{35}$`)
	chatPattern    = regexp.MustCompile(`^(?:-?[1-9][0-9]{0,19}|@[A-Za-z][A-Za-z0-9_]{4,31})$`)
)

// Options is enabled by its presence in the application's configuration.
// RelayAddress selects rdpwatcher instead of direct bot access.
// TokenEnv names an environment variable, not a token or a token file.
type Options struct {
	RelayAddress   string   `json:"relay_address,omitempty" yaml:"relay_address,omitempty"`
	TokenEnv       string   `json:"token_env,omitempty" yaml:"token_env,omitempty"`
	ChatID         string   `json:"chat_id,omitempty" yaml:"chat_id,omitempty"`
	Events         []string `json:"events" yaml:"events"`
	AllowedUserIDs []int64  `json:"allowed_user_ids,omitempty" yaml:"allowed_user_ids,omitempty"`
}

func knownEvent(kind string) bool {
	switch kind {
	case "startup", "connection_error", "connected", "power_changed",
		"low_battery_changed", "fault_changed", "shutdown_pending", "shutdown_cancelled",
		"shutdown_requesting", "shutdown_requested", "shutdown_dry_run", "shutdown_failed", "shutdown_state_changed":
		return true
	default:
		return false
	}
}

func (o Options) Validate() error {
	if o.RelayAddress != "" {
		if err := validateRelayAddress(o.RelayAddress); err != nil {
			return err
		}
		if o.TokenEnv != "" || o.ChatID != "" || len(o.AllowedUserIDs) > 0 {
			return errors.New("notifications.relay_address cannot be combined with token_env, chat_id or allowed_user_ids; relay supports notifications only")
		}
	} else {
		if !envNamePattern.MatchString(o.TokenEnv) {
			return errors.New("notifications.token_env must name an environment variable")
		}
		if !chatPattern.MatchString(o.ChatID) {
			return errors.New("notifications.chat_id must be a numeric destination or channel username")
		}
	}
	if len(o.Events) == 0 || len(o.Events) > 13 {
		return errors.New("notifications.events must explicitly select supported monitoring events")
	}
	if len(o.AllowedUserIDs) > 16 {
		return errors.New("Telegram commands permit at most 16 allowed user IDs")
	}
	users := make(map[int64]bool)
	for _, id := range o.AllowedUserIDs {
		if id <= 0 || users[id] {
			return errors.New("Telegram command user IDs must be positive and unique")
		}
		users[id] = true
	}
	if len(users) > 0 && !regexp.MustCompile(`^-?[1-9][0-9]{0,15}$`).MatchString(o.ChatID) {
		return errors.New("Telegram checking commands require an explicit numeric chat_id")
	}
	seen := make(map[string]bool, len(o.Events))
	for _, kind := range o.Events {
		if !knownEvent(kind) || seen[kind] {
			return errors.New("notifications.events contains an unsupported or duplicate event type")
		}
		seen[kind] = true
	}
	return nil
}

// Reply queues a command response without making the policy wait for Telegram.
func (s *Sender) Reply(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		s.status.Dropped++
		return errors.New("Telegram sender is stopped or blocked")
	}
	text = s.redact(truncate(text, maxTextRunes))
	select {
	case s.queue <- pending{text: text, expires: time.Now().Add(30 * time.Second)}:
		return nil
	default:
		s.status.Dropped++
		s.status.Mode, s.status.Error = "degraded", "Telegram response queue is full"
		return errors.New("Telegram response queue is full")
	}
}

// Status describes API/relay acceptance, not delivery to or acknowledgement by a person.
// Dropped includes queue overflow, expiry, failed delivery and cancellation.
type Status struct {
	Mode        string     `json:"mode"`
	Error       string     `json:"error,omitempty"`
	LastSuccess *time.Time `json:"last_success,omitempty"`
	Dropped     uint64     `json:"dropped_events"`
}

type pending struct {
	text       string
	expires    time.Time
	completion chan error
}

// Sender owns a bounded, memory-only queue. Submit never performs network I/O.
// Call Run once in a background goroutine and cancel its context on shutdown.
// Live event sequences must be submitted in order within a service session.
type Sender struct {
	mu           sync.Mutex
	status       Status
	started      bool
	closed       bool
	selected     map[string]bool
	sessions     map[string]uint64
	order        []string
	queue        chan pending
	token        string
	chatID       string
	endpoint     string
	client       *http.Client
	relayAddress string

	// Private injection points keep production routing and budgets fixed.
	expiry      time.Duration
	minInterval time.Duration
	backoff     time.Duration
	attempts    int
}

// New resolves direct-mode bot credentials at initialization. Relay mode needs
// no Telegram credentials and opens its connection only while Run is active.
func New(options Options) (*Sender, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	var token string
	if options.RelayAddress == "" {
		var exists bool
		token, exists = os.LookupEnv(options.TokenEnv)
		if !exists || !tokenPattern.MatchString(token) {
			return nil, errors.New("notifications bot token environment variable is missing or invalid")
		}
	}
	selected := make(map[string]bool, len(options.Events))
	for _, kind := range options.Events {
		selected[kind] = true
	}
	return &Sender{
		status: Status{Mode: "enabled"}, selected: selected,
		sessions: make(map[string]uint64), queue: make(chan pending, queueCapacity),
		token: token, chatID: options.ChatID,
		relayAddress: options.RelayAddress,
		endpoint:     "https://api.telegram.org/bot" + token + "/sendMessage",
		client: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: noRedirect,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				MaxConnsPerHost:       1,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
			},
		},
		expiry: 2 * time.Minute, minInterval: time.Second,
		backoff: time.Second, attempts: 3,
	}, nil
}

func noRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

// Submit accepts only selected live event types. Parent code must never submit
// recovered history. High-water marks suppress duplicate/older sequences, with
// bounded retention for the last 16 sessions (normally only one per Sender).
func (s *Sender) Submit(event events.Record) {
	if !s.selected[event.Type] {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.SessionID == "" || len(event.SessionID) > 128 || event.Sequence == 0 || event.Time.IsZero() {
		s.status.Dropped++
		if !s.closed {
			s.status.Mode = "degraded"
			s.status.Error = "Notification event has no valid live identity or timestamp"
		}
		return
	}
	last, exists := s.sessions[event.SessionID]
	if exists && event.Sequence <= last {
		return
	}
	if !exists {
		if len(s.order) == maxSessions {
			delete(s.sessions, s.order[0])
			s.order = s.order[1:]
		}
		s.order = append(s.order, event.SessionID)
	}
	s.sessions[event.SessionID] = event.Sequence
	if s.closed {
		s.status.Dropped++
		return
	}
	now := time.Now()
	expires := event.Time.Add(s.expiry)
	if upper := now.Add(s.expiry); expires.After(upper) {
		expires = upper
	}
	if !expires.After(now) {
		s.status.Dropped++
		s.status.Mode = "degraded"
		s.status.Error = "Notification expired before delivery"
		return
	}
	// Bound before replacement so even an unexpectedly large source error cannot
	// make the queue retain large records or include an embedded credential.
	message := truncate(event.Message, maxTextRunes)
	message = s.redact(message)
	text := formatNotification(event.Type, message, event.Time)
	item := pending{text: truncate(text, maxTextRunes), expires: expires}
	select {
	case s.queue <- item:
	default:
		s.status.Dropped++
		s.status.Mode = "degraded"
		s.status.Error = "Notification queue is full; events were dropped"
	}
}

func formatNotification(kind, message string, at time.Time) string {
	icon, title := "🔔", "Notification"
	switch kind {
	case "startup":
		icon, title = "🚀", "UPSmart started"
	case "connected":
		icon, title = "✅", "UPS connected"
	case "connection_error":
		icon, title = "⚠️", "UPS connection problem"
	case "power_changed":
		icon, title = "🔌", "Power status changed"
		if strings.Contains(message, "on_battery") {
			icon, title = "🔋", "Mains power lost - running on battery"
		} else if strings.Contains(message, "online") {
			icon, title = "⚡", "Mains power restored"
		}
	case "low_battery_changed":
		icon, title = "🪫", "UPS low-battery status changed"
	case "fault_changed":
		icon, title = "🚨", "UPS fault status changed"
	case "shutdown_pending":
		icon, title = "⏳", "Windows shutdown pending"
	case "shutdown_cancelled":
		icon, title = "✅", "Windows shutdown cancelled"
	case "shutdown_requesting", "shutdown_requested":
		icon, title = "🛑", "Windows shutdown requested"
	case "shutdown_dry_run":
		icon, title = "🧪", "Shutdown dry-run"
	case "shutdown_failed":
		icon, title = "❌", "Windows shutdown failed"
	case "shutdown_state_changed":
		icon, title = "⚙️", "PC shutdown policy changed"
	}
	return icon + " " + title + "\n🕒 " + at.UTC().Format(time.RFC3339) + "\n" + message
}

func (s *Sender) redact(text string) string {
	for _, value := range []string{s.token, s.chatID} {
		if value != "" {
			text = strings.ReplaceAll(text, value, "[redacted]")
		}
	}
	return text
}

func truncate(text string, limit int) string {
	count := 0
	for index := range text {
		if count == limit {
			return strings.ToValidUTF8(text[:index], "\uFFFD")
		}
		count++
	}
	if !utf8.ValidString(text) {
		return strings.ToValidUTF8(text, "\uFFFD")
	}
	return text
}

func (s *Sender) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.status
	if out.LastSuccess != nil {
		copy := *out.LastSuccess
		out.LastSuccess = &copy
	}
	return out
}

// Test enqueues an operator-requested message on the same serialized worker as
// live events and waits for Telegram or the relay to accept it.
func (s *Sender) Test(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("Telegram sender is stopped")
	}
	if s.status.Mode == "blocked" {
		err := errors.New(s.status.Error)
		s.mu.Unlock()
		return err
	}
	now := time.Now()
	completion := make(chan error, 1)
	item := pending{
		text: "🧪 UPSmart test notification\n🕒 " + now.UTC().Format(time.RFC3339) +
			"\nTest notification requested from desktop settings.",
		expires: now.Add(min(s.expiry, 8*time.Second)), completion: completion,
	}
	select {
	case s.queue <- item:
		s.mu.Unlock()
	default:
		s.mu.Unlock()
		return errors.New("Telegram test queue is full")
	}
	select {
	case err := <-completion:
		return err
	case <-ctx.Done():
		return errors.New("Telegram test request cancelled before completion")
	}
}

// Run serializes all sends and retries. Cancellation discards remaining memory
// events with a visible count; there is no durable outbox or shutdown drain.
func (s *Sender) Run(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.closed = true
		for len(s.queue) > 0 {
			item := <-s.queue
			s.status.Dropped++
			if item.completion != nil {
				item.completion <- errors.New("Telegram sender stopped before test completion")
			}
		}
		if s.status.Mode != "blocked" {
			s.status.Mode = "stopped"
		}
	}()

	// Also enforce these bounds for internally injected clients.
	client := *s.client
	if client.Timeout <= 0 || client.Timeout > 10*time.Second {
		client.Timeout = 10 * time.Second
	}
	client.CheckRedirect = noRedirect
	defer client.CloseIdleConnections()
	request := func(ctx context.Context, text string) (bool, bool, bool, time.Duration, string) {
		return s.request(ctx, &client, text)
	}
	if s.relayAddress != "" {
		conn, err := openRelay(s.relayAddress)
		if err != nil {
			s.mu.Lock()
			s.status.Mode, s.status.Error = "blocked", "Notification relay client could not be initialized"
			s.mu.Unlock()
			s.failQueued(errors.New("Notification relay client could not be initialized"))
			return
		}
		defer conn.Close()
		request = func(ctx context.Context, text string) (bool, bool, bool, time.Duration, string) {
			return requestRelay(ctx, conn, text)
		}
	}
	var nextAttempt time.Time
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case item := <-s.queue:
			accepted, permanent, reason := s.deliverWith(ctx, item, &nextAttempt, request)
			s.mu.Lock()
			if accepted {
				now := time.Now().UTC()
				s.status.LastSuccess = &now
				s.status.Mode, s.status.Error = "enabled", ""
			} else {
				s.status.Dropped++
				s.status.Mode, s.status.Error = "degraded", reason
				if permanent {
					s.status.Mode = "blocked"
				}
			}
			s.mu.Unlock()
			if item.completion != nil {
				if accepted {
					item.completion <- nil
				} else {
					item.completion <- errors.New(reason)
				}
			}
			if permanent {
				s.failQueued(errors.New(reason))
				return
			}
		}
	}
}

func (s *Sender) failQueued(err error) {
	s.mu.Lock()
	for len(s.queue) > 0 {
		item := <-s.queue
		s.status.Dropped++
		if item.completion != nil {
			item.completion <- err
		}
	}
	s.mu.Unlock()
}

func waitUntil(ctx context.Context, until time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	timer := time.NewTimer(time.Until(until))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (s *Sender) deliverWith(ctx context.Context, item pending, nextAttempt *time.Time, request func(context.Context, string) (bool, bool, bool, time.Duration, string)) (bool, bool, string) {
	deliveryCtx, cancel := context.WithDeadline(ctx, item.expires)
	defer cancel()
	reason := "Telegram delivery attempts exhausted"
	for attempt := 0; attempt < s.attempts; attempt++ {
		if !waitUntil(deliveryCtx, *nextAttempt) {
			return false, false, "Notification cancelled or expired before API acceptance"
		}
		accepted, permanent, retry, delay, failure := request(deliveryCtx, item.text)
		*nextAttempt = time.Now().Add(s.minInterval)
		if accepted || permanent || !retry {
			return accepted, permanent, failure
		}
		reason = failure
		if delay < s.backoff*time.Duration(1<<attempt) {
			delay = s.backoff * time.Duration(1<<attempt)
		}
		retryAt := time.Now().Add(delay)
		if retryAt.After(*nextAttempt) {
			*nextAttempt = retryAt
		}
		if !nextAttempt.Before(item.expires) {
			return false, false, "Notification retry exceeds expiry; event dropped"
		}
	}
	return false, false, reason
}

// Never propagate HTTP errors, URLs, response descriptions or response bodies:
// they can contain credentials and destination identifiers.
func (s *Sender) request(ctx context.Context, client *http.Client, text string) (accepted, permanent, retry bool, delay time.Duration, reason string) {
	body, err := json.Marshal(struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{s.chatID, text})
	if err != nil {
		return false, true, false, 0, "Notification payload could not be encoded"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return false, true, false, 0, "Telegram request could not be constructed"
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return false, false, true, 0, "Telegram network request failed; API acceptance is unknown"
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	closeErr := response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return false, true, false, 0, "Telegram redirect refused; check notification setup"
	}
	var result struct {
		OK         *bool `json:"ok"`
		ErrorCode  int   `json:"error_code"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	valid := readErr == nil && closeErr == nil && len(raw) <= maxBodyBytes && json.Unmarshal(raw, &result) == nil
	code := response.StatusCode
	if valid && result.OK != nil && !*result.OK && result.ErrorCode != 0 {
		code = result.ErrorCode
	}
	switch {
	case code == http.StatusTooManyRequests:
		// Cap arithmetic, not Telegram's instruction: an oversized wait expires
		// the event rather than retrying earlier than retry_after.
		delay = s.expiry + time.Second
		if valid && result.Parameters.RetryAfter > 0 && result.Parameters.RetryAfter <= int64(s.expiry/time.Second) {
			delay = time.Duration(result.Parameters.RetryAfter) * time.Second
		}
		return false, false, true, delay, "Telegram rate limit prevented API acceptance"
	case code == 400 || code == 401 || code == 403 || code == 404:
		return false, true, false, 0, "Telegram rejected authentication or destination; check notification setup"
	case code >= 500 || code == http.StatusRequestTimeout:
		return false, false, true, 0, "Telegram temporarily unavailable; API acceptance is unknown"
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return false, true, false, 0, "Telegram rejected the request; check notification setup"
	case !valid || result.OK == nil:
		return false, false, true, 0, "Telegram response was unreadable or invalid; API acceptance is unknown"
	case !*result.OK:
		return false, true, false, 0, "Telegram did not accept the notification; check notification setup"
	default:
		return true, false, false, 0, ""
	}
}
