package daemon

import (
	"context"
	"log/slog"
	"os"

	"github.com/scrothers/aim/internal/notify"
)

// startNotify seeds webhooks from the configuration on first start, hooks
// operator mail into the notifier and runs it until ctx is canceled. The
// returned channel closes when it has stopped.
func (d *daemon) startNotify(ctx context.Context) <-chan struct{} {
	d.notify = notify.New(d.svc, d.logger)
	if _, err := d.notify.SeedWebhooks(ctx, d.cfg.NotifyWebhooks); err != nil {
		d.logger.LogAttrs(ctx, slog.LevelWarn, "seeding notification webhooks failed", slog.String("error", err.Error()))
	}
	d.svc.SetOnOperatorDelivery(d.notify.Handle)
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.notify.Run(ctx)
	}()
	return done
}

// setPublicURL derives the default console link base from the bound HTTP
// address and this machine's hostname.
func (d *daemon) setPublicURL(httpAddr string) {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "localhost"
	}
	if u := notify.DefaultPublicURLFor(host, httpAddr); u != "" {
		d.notify.SetDefaultPublicURL(u)
	}
}
