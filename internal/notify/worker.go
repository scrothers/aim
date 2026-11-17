package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/scrothers/aim/internal/mail"
)

// worker delivers to one webhook, coalescing bursts.
type worker struct {
	m    *Manager
	id   string
	wake chan struct{}

	mu      sync.Mutex
	pending []mail.Message
}

func (w *worker) add(msg mail.Message) {
	w.mu.Lock()
	if len(w.pending) >= maxPending {
		w.pending = w.pending[1:] // drop the oldest; the summary still counts what remains
	}
	w.pending = append(w.pending, msg)
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *worker) take() []mail.Message {
	w.mu.Lock()
	defer w.mu.Unlock()
	b := w.pending
	w.pending = nil
	return b
}

func (w *worker) run(ctx context.Context) {
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		}
		// Hold the batch open until the coalescing window since the last post
		// has passed, so a burst arrives as one card.
		if wait := w.m.coalesce - w.m.now().Sub(last); !last.IsZero() && wait > 0 {
			if !sleep(ctx, wait) {
				return
			}
		}
		batch := w.take()
		if len(batch) == 0 {
			continue
		}
		w.deliver(ctx, batch)
		last = w.m.now()
	}
}

func (w *worker) deliver(ctx context.Context, batch []mail.Message) {
	cfg, err := w.m.set.Notify(ctx)
	if err != nil {
		w.m.log.Warn("notify: load settings failed", slog.String("error", err.Error()))
		return
	}
	var wh *mail.Webhook
	for i := range cfg.Webhooks {
		if cfg.Webhooks[i].ID == w.id {
			wh = &cfg.Webhooks[i]
		}
	}
	if wh == nil || !wh.Enabled {
		return // removed or disabled while queued
	}
	kind, err := w.m.classify(wh.URL)
	if err != nil {
		w.m.record(ctx, w.id, Result{Error: err.Error()})
		return
	}
	body, err := w.m.cardFor(w.m.publicURL(cfg), batch).payload(kind)
	if err != nil {
		w.m.record(ctx, w.id, Result{Error: err.Error()})
		return
	}
	res := w.m.post(ctx, wh.URL, body, maxAttempts)
	if ctx.Err() != nil {
		return
	}
	if !res.OK() {
		w.m.log.Warn("notify: delivery failed", slog.String("webhook", w.id), slog.Int("status", res.Status), slog.String("error", res.Error))
	}
	w.m.record(ctx, w.id, res)
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// post sends body to raw, retrying server errors, network errors and 429s
// up to attempts times with exponential backoff.
func (m *Manager) post(ctx context.Context, raw string, body []byte, attempts int) Result {
	var res Result
	for i := range attempts {
		var retryAfter time.Duration
		res, retryAfter = m.postOnce(ctx, raw, body)
		if res.OK() || !retryable(res) || i == attempts-1 {
			return res
		}
		delay := m.backoff << i
		if retryAfter > 0 {
			delay = min(retryAfter, maxRetryAfter)
		}
		if !sleep(ctx, delay) {
			return res
		}
	}
	return res
}

func retryable(r Result) bool {
	return r.Status == 0 || r.Status == http.StatusTooManyRequests || r.Status >= http.StatusInternalServerError
}

func (m *Manager) postOnce(ctx context.Context, raw string, body []byte) (Result, time.Duration) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, bytes.NewReader(body))
	if err != nil {
		return Result{Error: "invalid webhook url"}, 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "aim-notify")
	resp, err := m.client.Do(req)
	if err != nil {
		// url.Error embeds the full URL, which carries the secret token.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return Result{Error: clip("request failed: " + err.Error())}, 0
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Result{Status: resp.StatusCode}, 0
	}
	msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if t := strings.TrimSpace(string(data)); t != "" {
		msg += ": " + t
	}
	var retry time.Duration
	if resp.StatusCode == http.StatusTooManyRequests {
		retry = retryAfter(resp.Header.Get("Retry-After"), data)
	}
	return Result{Status: resp.StatusCode, Error: clip(msg)}, retry
}

// retryAfter reads the wait from Slack's Retry-After header (seconds) or
// Discord's JSON retry_after (seconds, fractional).
func retryAfter(header string, body []byte) time.Duration {
	var secs float64
	if f, err := strconv.ParseFloat(strings.TrimSpace(header), 64); err == nil {
		secs = f
	}
	var v struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if json.Unmarshal(body, &v) == nil && v.RetryAfter > 0 {
		secs = v.RetryAfter
	}
	return time.Duration(secs * float64(time.Second))
}

func clip(s string) string {
	s = strings.ToValidUTF8(s, "")
	if utf8.RuneCountInString(s) > maxErrorRunes {
		return string([]rune(s)[:maxErrorRunes]) + "…"
	}
	return s
}
