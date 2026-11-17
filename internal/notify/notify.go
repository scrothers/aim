package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/render"
)

// Defaults.
const (
	DefaultCoalesce  = 30 * time.Second
	DefaultBackoff   = time.Second
	defaultQueue     = 256
	maxAttempts      = 4
	maxRetryAfter    = time.Minute
	snippetRunes     = 140
	maxErrorRunes    = 300
	httpTimeout      = 15 * time.Second
	maxPending       = 100
	maxResponseBytes = 4096
)

// Settings is the part of the mail service the notifier uses.
type Settings interface {
	Notify(ctx context.Context) (mail.NotifySettings, error)
	SetNotify(ctx context.Context, n mail.NotifySettings) (mail.NotifySettings, error)
	UpdateNotify(ctx context.Context, fn func(*mail.NotifySettings)) error
	NotifyConfigured(ctx context.Context) (bool, error)
	ThreadMuted(ctx context.Context, threadID string) (bool, error)
}

// Option configures a Manager.
type Option func(*Manager)

// WithClient sets the HTTP client (default: 15s timeout).
func WithClient(c *http.Client) Option { return func(m *Manager) { m.client = c } }

// WithClock sets the time source (default time.Now).
func WithClock(now func() time.Time) Option { return func(m *Manager) { m.now = now } }

// WithCoalesce sets the minimum gap between posts to one webhook.
func WithCoalesce(d time.Duration) Option { return func(m *Manager) { m.coalesce = d } }

// WithBackoff sets the base retry delay (doubled per attempt).
func WithBackoff(d time.Duration) Option { return func(m *Manager) { m.backoff = d } }

// WithHostCheck replaces the webhook host allowlist; tests use it to point
// at httptest servers.
func WithHostCheck(c HostCheck) Option { return func(m *Manager) { m.check = c } }

// WithDefaultPublicURL sets the console URL used when the settings have none.
func WithDefaultPublicURL(u string) Option { return func(m *Manager) { m.defaultURL = u } }

// Manager delivers operator mail to the configured webhooks.
type Manager struct {
	set      Settings
	log      *slog.Logger
	client   *http.Client
	now      func() time.Time
	coalesce time.Duration
	backoff  time.Duration
	check    HostCheck
	renderer *render.Renderer

	in chan mail.Message

	mu         sync.Mutex
	defaultURL string
	workers    map[string]*worker
}

// New returns a Manager. Run must be called for deliveries to be posted.
func New(set Settings, log *slog.Logger, opts ...Option) *Manager {
	m := &Manager{
		set: set, log: log, client: &http.Client{Timeout: httpTimeout}, now: time.Now,
		coalesce: DefaultCoalesce, backoff: DefaultBackoff, check: defaultKind,
		renderer: render.New(), in: make(chan mail.Message, defaultQueue), workers: map[string]*worker{},
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// SetDefaultPublicURL sets the console URL used when settings have none; the
// daemon calls it once the HTTP listener is bound.
func (m *Manager) SetDefaultPublicURL(u string) {
	m.mu.Lock()
	m.defaultURL = u
	m.mu.Unlock()
}

// DefaultPublicURL returns the fallback console URL.
func (m *Manager) DefaultPublicURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.defaultURL
}

// DefaultPublicURLFor builds http://<host>:<port> from an HTTP listen
// address such as ":7172" or "0.0.0.0:7172"; it returns "" for an address
// without a port.
func DefaultPublicURLFor(host, listen string) string {
	i := strings.LastIndex(listen, ":")
	if i < 0 || i == len(listen)-1 {
		return ""
	}
	return "http://" + host + ":" + listen[i+1:]
}

// Handle queues a committed operator delivery for notification. It never
// blocks: when the queue is full the message is dropped and logged. It is
// meant to be installed as mail's OnOperatorDelivery hook.
func (m *Manager) Handle(msg mail.Message) {
	if msg.From == address.OperatorName {
		return
	}
	select {
	case m.in <- msg:
	default:
		m.log.Warn("notification queue full; dropping", slog.String("message", msg.ID))
	}
}

// Run dispatches queued deliveries until ctx is canceled and all webhook
// workers have stopped.
func (m *Manager) Run(ctx context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-m.in:
			m.dispatch(ctx, &wg, msg)
		}
	}
}

func (m *Manager) dispatch(ctx context.Context, wg *sync.WaitGroup, msg mail.Message) {
	muted, err := m.set.ThreadMuted(ctx, msg.ThreadID)
	if err != nil {
		m.log.Warn("notify: mute check failed", slog.String("error", err.Error()))
	}
	if muted {
		return
	}
	cfg, err := m.set.Notify(ctx)
	if err != nil {
		m.log.Warn("notify: load settings failed", slog.String("error", err.Error()))
		return
	}
	for _, w := range cfg.Webhooks {
		if !w.Enabled {
			continue
		}
		m.worker(ctx, wg, w.ID).add(msg)
	}
}

func (m *Manager) worker(ctx context.Context, wg *sync.WaitGroup, id string) *worker {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workers[id]
	if !ok {
		w = &worker{m: m, id: id, wake: make(chan struct{}, 1)}
		m.workers[id] = w
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.run(ctx)
		}()
	}
	return w
}

func (m *Manager) publicURL(cfg mail.NotifySettings) string {
	if cfg.PublicURL != "" {
		return cfg.PublicURL
	}
	return m.DefaultPublicURL()
}

func (m *Manager) link(base, threadID string) string {
	if base == "" {
		return ""
	}
	if threadID == "" {
		return base + "/mail?view=operator"
	}
	return base + "/mail?view=operator&thread=" + url.QueryEscape(threadID)
}

// cardFor renders the notification for a batch of messages.
func (m *Manager) cardFor(base string, batch []mail.Message) card {
	if len(batch) == 1 {
		msg := batch[0]
		meta := msg.From
		if msg.FromClient != "" {
			meta += " via " + msg.FromClient
		}
		if msg.FromMachine != "" {
			meta += " on " + msg.FromMachine
		}
		return card{
			Title: msg.Subject, Body: m.renderer.Snippet(msg.Body, snippetRunes),
			Link: m.link(base, msg.ThreadID), Meta: meta, At: msg.CreatedAt,
		}
	}
	var b strings.Builder
	for i, msg := range batch {
		if i == maxListed {
			fmt.Fprintf(&b, "…and %d more", len(batch)-maxListed)
			break
		}
		fmt.Fprintf(&b, "• %s: %s\n", msg.From, truncate(msg.Subject, 100))
	}
	return card{
		Title: fmt.Sprintf("%d new messages for the operator", len(batch)),
		Body:  strings.TrimRight(b.String(), "\n"), Link: m.link(base, ""),
		Meta: "AIM", At: batch[len(batch)-1].CreatedAt,
	}
}

// SendTest posts a test card to one webhook immediately (no coalescing, no
// retries) and records the outcome like a real delivery. It returns the
// outcome even when the post failed; the error is non-nil only when the
// webhook does not exist or is not usable.
func (m *Manager) SendTest(ctx context.Context, webhookID string) (Result, error) {
	cfg, err := m.set.Notify(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("notify: %w", err)
	}
	var wh *mail.Webhook
	for i := range cfg.Webhooks {
		if cfg.Webhooks[i].ID == webhookID {
			wh = &cfg.Webhooks[i]
		}
	}
	if wh == nil {
		return Result{}, fmt.Errorf("notify: %w: no webhook %q", mail.ErrNotFound, webhookID)
	}
	kind, err := m.classify(wh.URL)
	if err != nil {
		return Result{}, fmt.Errorf("notify: %w: %w", mail.ErrInvalidInput, err)
	}
	now := m.now()
	body, err := card{
		Title: "AIM test notification", Body: "If you can read this, operator mail notifications are working.",
		Link: m.link(m.publicURL(cfg), ""), Meta: "AIM", At: now,
	}.payload(kind)
	if err != nil {
		return Result{}, err
	}
	res := m.post(ctx, wh.URL, body, 1)
	m.record(ctx, wh.ID, res)
	return res, nil
}

func (m *Manager) classify(raw string) (Kind, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("webhook url is not a valid URL")
	}
	return m.check(u)
}

// Result is the outcome of one delivery.
type Result struct {
	// Status is the last HTTP status, 0 when no response was received.
	Status int `json:"status"`
	// Error is empty on success.
	Error string `json:"error,omitempty"`
}

// OK reports whether the delivery succeeded.
func (r Result) OK() bool { return r.Error == "" }

// record stores a delivery outcome on the webhook in settings.
func (m *Manager) record(ctx context.Context, id string, res Result) {
	now := m.now().UTC()
	err := m.set.UpdateNotify(context.WithoutCancel(ctx), func(n *mail.NotifySettings) {
		for i := range n.Webhooks {
			if n.Webhooks[i].ID == id {
				n.Webhooks[i].LastStatus, n.Webhooks[i].LastError, n.Webhooks[i].LastSentAt = res.Status, res.Error, &now
			}
		}
	})
	if err != nil {
		m.log.Warn("notify: record status failed", slog.String("error", err.Error()))
	}
}
