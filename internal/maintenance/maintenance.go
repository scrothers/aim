package maintenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
)

// DefaultInterval is how often Run checks whether work is due.
const DefaultInterval = time.Hour

const backupTimeLayout = "20060102T150405Z"

// backupName matches exactly the files this package creates and may delete.
var backupName = regexp.MustCompile(`^aim-\d{8}T\d{6}Z\.db$`)

// ErrNoBackupDir is returned when neither the settings nor the daemon
// configuration name a backup directory.
var ErrNoBackupDir = errors.New("no backup directory configured")

// DefaultDir returns the default backup directory: $XDG_STATE_HOME/aim/backups,
// or ~/.local/state/aim/backups.
func DefaultDir(getenv func(string) string, home string) string {
	if x := getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "aim", "backups")
	}
	return filepath.Join(home, ".local", "state", "aim", "backups")
}

// Option configures a Runner.
type Option func(*Runner)

// WithClock sets the time source (default time.Now).
func WithClock(now func() time.Time) Option { return func(r *Runner) { r.now = now } }

// WithInterval sets how often Run checks for due work.
func WithInterval(d time.Duration) Option { return func(r *Runner) { r.interval = d } }

// WithDefaultDir sets the backup directory used when the settings leave it
// empty. Without it (and without a settings dir) scheduled backups are off.
func WithDefaultDir(dir string) Option { return func(r *Runner) { r.defaultDir = dir } }

// Runner performs maintenance against one store.
type Runner struct {
	st         *store.Store
	svc        *mail.Service
	log        *slog.Logger
	now        func() time.Time
	interval   time.Duration
	defaultDir string

	mu sync.Mutex // serializes backups
}

// New returns a Runner.
func New(st *store.Store, svc *mail.Service, log *slog.Logger, opts ...Option) *Runner {
	r := &Runner{st: st, svc: svc, log: log, now: time.Now, interval: DefaultInterval}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Dir returns the effective backup directory ("" when none is configured).
func (r *Runner) Dir(ctx context.Context) (string, error) {
	b, err := r.svc.Backups(ctx)
	if err != nil {
		return "", fmt.Errorf("maintenance: %w", err)
	}
	if b.Dir != "" {
		return b.Dir, nil
	}
	return r.defaultDir, nil
}

// BackupNow writes a backup into the backup directory, prunes old backups
// beyond the keep count and records the outcome in the settings. The status
// is returned even when the backup failed, alongside the error.
func (r *Runner) BackupNow(ctx context.Context) (mail.BackupStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().UTC()
	prev, _ := r.svc.BackupStatus(ctx)
	st := mail.BackupStatus{LastAt: &now, LastSuccessAt: prev.LastSuccessAt, LastPath: prev.LastPath, LastSize: prev.LastSize}
	path, size, err := r.backup(ctx, now)
	if err != nil {
		st.LastError = err.Error()
	} else {
		st.LastSuccessAt, st.LastPath, st.LastSize = &now, path, size
	}
	if serr := r.svc.SetBackupStatus(ctx, st); serr != nil {
		err = errors.Join(err, serr)
	}
	return st, err
}

func (r *Runner) backup(ctx context.Context, now time.Time) (string, int64, error) {
	dir, err := r.Dir(ctx)
	if err != nil {
		return "", 0, err
	}
	if dir == "" {
		return "", 0, fmt.Errorf("maintenance: backup: %w", ErrNoBackupDir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", 0, fmt.Errorf("maintenance: backup: %w", err)
	}
	path := filepath.Join(dir, "aim-"+now.Format(backupTimeLayout)+".db")
	if _, err := os.Lstat(path); err == nil {
		return "", 0, fmt.Errorf("maintenance: backup: %s already exists (one backup per second)", path)
	}
	if err := r.st.Backup(ctx, path); err != nil {
		_ = os.Remove(path) // path did not exist, so any file is a partial one of ours
		return "", 0, fmt.Errorf("maintenance: %w", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", 0, fmt.Errorf("maintenance: backup: %w", err)
	}
	cfg, err := r.svc.Backups(ctx)
	if err != nil {
		return path, fi.Size(), fmt.Errorf("maintenance: %w", err)
	}
	if err := Rotate(dir, cfg.Keep); err != nil {
		return path, fi.Size(), err
	}
	return path, fi.Size(), nil
}

// Rotate deletes all but the newest keep backups in dir. Only regular files
// named exactly aim-YYYYMMDDTHHMMSSZ.db are considered; anything else in dir
// is left alone.
func Rotate(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("maintenance: rotate: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && backupName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names) // timestamps sort chronologically
	if len(names) <= keep {
		return nil
	}
	var errs []error
	for _, n := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("maintenance: rotate: %w", err)
	}
	return nil
}

// BackupDue reports whether a scheduled backup should run now.
func (r *Runner) BackupDue(ctx context.Context) (bool, error) {
	dir, err := r.Dir(ctx)
	if err != nil || dir == "" {
		return false, err
	}
	cfg, err := r.svc.Backups(ctx)
	if err != nil {
		return false, fmt.Errorf("maintenance: %w", err)
	}
	st, err := r.svc.BackupStatus(ctx)
	if err != nil {
		return false, fmt.Errorf("maintenance: %w", err)
	}
	if st.LastSuccessAt == nil {
		return true, nil
	}
	return !r.now().Before(st.LastSuccessAt.Add(time.Duration(cfg.EveryHours) * time.Hour)), nil
}

// Prune applies the retention setting now.
func (r *Runner) Prune(ctx context.Context, dryRun bool) (mail.PruneResult, error) {
	return r.svc.Prune(ctx, r.now(), dryRun)
}

// Tick runs one maintenance pass: prune when retention is on, then back up
// when one is due. Failures are logged and do not stop later steps.
func (r *Runner) Tick(ctx context.Context) {
	if res, err := r.Prune(ctx, false); err != nil {
		r.log.LogAttrs(ctx, slog.LevelError, "retention prune failed", slog.String("error", err.Error()))
	} else if res.Messages > 0 {
		r.log.LogAttrs(ctx, slog.LevelInfo, "pruned archived mail", slog.Int("messages", res.Messages))
	}
	due, err := r.BackupDue(ctx)
	if err != nil {
		r.log.LogAttrs(ctx, slog.LevelError, "backup schedule check failed", slog.String("error", err.Error()))
		return
	}
	if !due {
		return
	}
	st, err := r.BackupNow(ctx)
	if err != nil {
		r.log.LogAttrs(ctx, slog.LevelError, "scheduled backup failed", slog.String("error", err.Error()))
		return
	}
	r.log.LogAttrs(ctx, slog.LevelInfo, "backup written", slog.String("path", st.LastPath), slog.Int64("bytes", st.LastSize))
}

// Run calls Tick immediately and then every interval until ctx is done.
func (r *Runner) Run(ctx context.Context) {
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		r.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
