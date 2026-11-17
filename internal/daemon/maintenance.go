package daemon

import (
	"context"

	"github.com/scrothers/aim/internal/maintenance"
	"github.com/scrothers/aim/internal/store"
)

// startMaintenance runs the housekeeping loop (retention prune, scheduled
// backups) until ctx is canceled; the returned channel closes when it has
// stopped, which must happen before the store closes.
func (d *daemon) startMaintenance(ctx context.Context, st *store.Store) <-chan struct{} {
	opts := []maintenance.Option{maintenance.WithDefaultDir(d.cfg.BackupDir)}
	if d.cfg.MaintenanceInterval > 0 {
		opts = append(opts, maintenance.WithInterval(d.cfg.MaintenanceInterval))
	}
	d.maint = maintenance.New(st, d.svc, d.logger, opts...)
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.maint.Run(ctx)
	}()
	return done
}
