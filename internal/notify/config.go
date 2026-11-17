package notify

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scrothers/aim/internal/mail"
)

// WebhookView is a webhook as shown to API clients: the URL is masked.
type WebhookView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	URL        string     `json:"url"`
	Kind       Kind       `json:"kind"`
	Enabled    bool       `json:"enabled"`
	LastStatus int        `json:"last_status,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	LastSentAt *time.Time `json:"last_sent_at,omitempty"`
}

// ConfigView is the notification settings as shown to API clients.
type ConfigView struct {
	// PublicURL is the stored console URL; empty means DefaultPublicURL.
	PublicURL        string        `json:"public_url"`
	DefaultPublicURL string        `json:"default_public_url"`
	Webhooks         []WebhookView `json:"webhooks"`
}

func (m *Manager) view(n mail.NotifySettings) ConfigView {
	v := ConfigView{PublicURL: n.PublicURL, DefaultPublicURL: m.DefaultPublicURL(), Webhooks: make([]WebhookView, len(n.Webhooks))}
	for i, w := range n.Webhooks {
		kind, _ := m.classify(w.URL)
		v.Webhooks[i] = WebhookView{
			ID: w.ID, Name: w.Name, URL: MaskURL(w.URL), Kind: kind, Enabled: w.Enabled,
			LastStatus: w.LastStatus, LastError: w.LastError, LastSentAt: w.LastSentAt,
		}
	}
	return v
}

// Config returns the notification settings with webhook URLs masked.
func (m *Manager) Config(ctx context.Context) (ConfigView, error) {
	n, err := m.set.Notify(ctx)
	if err != nil {
		return ConfigView{}, fmt.Errorf("notify: %w", err)
	}
	return m.view(n), nil
}

// UpdateConfig replaces the notification settings. A webhook with an empty
// URL keeps its stored one (so clients can edit without ever seeing the
// secret); a non-empty URL must be on an allowed host. It returns the masked
// result.
func (m *Manager) UpdateConfig(ctx context.Context, in mail.NotifySettings) (ConfigView, error) {
	for _, w := range in.Webhooks {
		if w.URL == "" {
			continue
		}
		if _, err := m.classify(w.URL); err != nil {
			return ConfigView{}, fmt.Errorf("notify: %w: webhook %q: %w", mail.ErrInvalidInput, w.Name, err)
		}
	}
	n, err := m.set.SetNotify(ctx, in)
	if err != nil {
		return ConfigView{}, err
	}
	return m.view(n), nil
}

// SeedWebhooks stores urls as the initial webhooks, on the first start only:
// if notification settings were ever stored, it does nothing. It always
// stores settings on first start (possibly empty), so a later start with the
// variable set does not re-seed. Unsupported URLs are skipped and logged
// without their value. It reports whether it stored anything.
func (m *Manager) SeedWebhooks(ctx context.Context, urls []string) (bool, error) {
	done, err := m.set.NotifyConfigured(ctx)
	if err != nil {
		return false, fmt.Errorf("notify: %w", err)
	}
	if done {
		return false, nil
	}
	seed := mail.NotifySettings{Webhooks: []mail.Webhook{}}
	for i, raw := range urls {
		if _, perr := m.classify(raw); perr != nil {
			m.log.Warn("AIM_NOTIFY_WEBHOOKS entry skipped", slog.Int("index", i), slog.String("reason", "not a supported webhook URL"))
			continue
		}
		seed.Webhooks = append(seed.Webhooks, mail.Webhook{Name: fmt.Sprintf("env-%d", i+1), URL: raw, Enabled: true})
	}
	if _, err := m.set.SetNotify(ctx, seed); err != nil {
		return false, fmt.Errorf("notify: seed: %w", err)
	}
	return len(seed.Webhooks) > 0, nil
}
