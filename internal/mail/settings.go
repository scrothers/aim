package mail

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/scrothers/aim/internal/store/db"
)

// ErrNotFound marks an unknown settings object such as a saved filter.
var ErrNotFound = errors.New("not found")

// Settings keys.
const (
	keyRetention    = "retention"
	keyBackups      = "backups"
	keyBackupStatus = "backup_status"
	keyNotify       = "notify"
	keyFilters      = "saved_filters"
)

// Settings defaults and limits.
const (
	DefaultBackupEveryHours = 24
	DefaultBackupKeep       = 7
	maxRetentionDays        = 36500
	maxBackupEveryHours     = 24 * 365
	maxBackupKeep           = 365
	MaxWebhooks             = 10
	MaxSavedFilters         = 50
	maxNameRunes            = 60
	maxFilterParams         = 20
	maxParamKeyRunes        = 40
	maxParamValueRunes      = 500
	prefixWebhook           = "whk-"
	prefixFilter            = "flt-"
)

// RetentionSettings configures pruning of old archived mail.
type RetentionSettings struct {
	// PruneArchivedAfterDays deletes messages older than this many days whose
	// every delivery is archived; 0 keeps everything.
	PruneArchivedAfterDays int `json:"prune_archived_after_days"`
}

// BackupSettings configures scheduled database backups.
type BackupSettings struct {
	EveryHours int `json:"every_hours"`
	Keep       int `json:"keep"`
	// Dir is the backup directory; empty selects the default
	// ($XDG_STATE_HOME/aim/backups).
	Dir string `json:"dir"`
}

// BackupStatus records the outcome of the most recent backup attempt.
type BackupStatus struct {
	// LastAt is the time of the last attempt, successful or not.
	LastAt *time.Time `json:"last_at,omitempty"`
	// LastSuccessAt is the time of the last successful backup.
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastPath      string     `json:"last_path,omitempty"`
	LastSize      int64      `json:"last_size,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

// Webhook is one notification endpoint. URL is a secret: API layers must
// mask it (see the notify package).
type Webhook struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	URL        string     `json:"url"`
	Enabled    bool       `json:"enabled"`
	LastStatus int        `json:"last_status,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	LastSentAt *time.Time `json:"last_sent_at,omitempty"`
}

// NotifySettings configures operator-mail notifications.
type NotifySettings struct {
	// PublicURL is the console base URL used in notification links.
	PublicURL string    `json:"public_url"`
	Webhooks  []Webhook `json:"webhooks"`
}

// SavedFilter is a named set of console search parameters.
type SavedFilter struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
}

// MutedThread is a thread muted for the operator.
type MutedThread struct {
	ThreadID string    `json:"thread_id"`
	MutedAt  time.Time `json:"muted_at"`
}

func getSetting[T any](ctx context.Context, q *db.Queries, key string, def T) (T, error) {
	row, err := q.GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return def, fmt.Errorf("mail: setting %s: %w", key, err)
	}
	out := def
	if err := json.Unmarshal([]byte(row.Value), &out); err != nil {
		return def, fmt.Errorf("mail: setting %s: decode: %w", key, err)
	}
	return out, nil
}

func putSetting(ctx context.Context, q *db.Queries, key string, v any, now time.Time) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("mail: setting %s: encode: %w", key, err)
	}
	if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: key, Value: string(b), UpdatedAt: now.UnixMilli()}); err != nil {
		return fmt.Errorf("mail: setting %s: %w", key, err)
	}
	return nil
}

// updateSetting runs a read-modify-write of one setting in a transaction.
func updateSetting[T any](ctx context.Context, s *Service, key string, def T, fn func(*T) error) (T, error) {
	var out T
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		v, err := getSetting(ctx, q, key, def)
		if err != nil {
			return err
		}
		if err := fn(&v); err != nil {
			return err
		}
		out = v
		return putSetting(ctx, q, key, v, s.now())
	})
	return out, err
}

// Retention returns the retention settings (pruning off by default).
func (s *Service) Retention(ctx context.Context) (RetentionSettings, error) {
	return getSetting(ctx, s.store.Read(), keyRetention, RetentionSettings{})
}

// SetRetention validates and stores the retention settings.
func (s *Service) SetRetention(ctx context.Context, r RetentionSettings) error {
	if r.PruneArchivedAfterDays < 0 || r.PruneArchivedAfterDays > maxRetentionDays {
		return invalidf("settings", "prune_archived_after_days must be 0-%d", maxRetentionDays)
	}
	_, err := updateSetting(ctx, s, keyRetention, RetentionSettings{}, func(v *RetentionSettings) error {
		*v = r
		return nil
	})
	return err
}

// Backups returns the backup settings with defaults applied.
func (s *Service) Backups(ctx context.Context) (BackupSettings, error) {
	b, err := getSetting(ctx, s.store.Read(), keyBackups, BackupSettings{})
	if err != nil {
		return BackupSettings{}, err
	}
	return b.withDefaults(), nil
}

func (b BackupSettings) withDefaults() BackupSettings {
	if b.EveryHours == 0 {
		b.EveryHours = DefaultBackupEveryHours
	}
	if b.Keep == 0 {
		b.Keep = DefaultBackupKeep
	}
	return b
}

// SetBackups validates and stores the backup settings. Zero EveryHours and
// Keep select the defaults.
func (s *Service) SetBackups(ctx context.Context, b BackupSettings) error {
	b = b.withDefaults()
	if b.EveryHours < 1 || b.EveryHours > maxBackupEveryHours {
		return invalidf("settings", "every_hours must be 1-%d", maxBackupEveryHours)
	}
	if b.Keep < 1 || b.Keep > maxBackupKeep {
		return invalidf("settings", "keep must be 1-%d", maxBackupKeep)
	}
	if b.Dir != "" && !filepath.IsAbs(b.Dir) {
		return invalidf("settings", "dir must be an absolute path (or empty for the default)")
	}
	b.Dir = filepath.Clean(b.Dir)
	if b.Dir == "." {
		b.Dir = ""
	}
	_, err := updateSetting(ctx, s, keyBackups, BackupSettings{}, func(v *BackupSettings) error {
		*v = b
		return nil
	})
	return err
}

// BackupStatus returns the last backup outcome.
func (s *Service) BackupStatus(ctx context.Context) (BackupStatus, error) {
	return getSetting(ctx, s.store.Read(), keyBackupStatus, BackupStatus{})
}

// SetBackupStatus records the outcome of a backup attempt.
func (s *Service) SetBackupStatus(ctx context.Context, st BackupStatus) error {
	_, err := updateSetting(ctx, s, keyBackupStatus, BackupStatus{}, func(v *BackupStatus) error {
		*v = st
		return nil
	})
	return err
}

// Notify returns the notification settings. Webhook URLs are unmasked.
func (s *Service) Notify(ctx context.Context) (NotifySettings, error) {
	n, err := getSetting(ctx, s.store.Read(), keyNotify, NotifySettings{})
	if n.Webhooks == nil {
		n.Webhooks = []Webhook{}
	}
	return n, err
}

// NotifyConfigured reports whether notification settings have ever been
// stored, so callers can seed defaults on first start only.
func (s *Service) NotifyConfigured(ctx context.Context) (bool, error) {
	_, err := s.store.Read().GetSetting(ctx, keyNotify)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("mail: setting %s: %w", keyNotify, err)
	}
	return true, nil
}

// SetNotify validates and replaces the notification settings. New webhooks
// (empty ID) get an id; the delivery status fields of existing webhooks are
// preserved, and an existing webhook submitted with an empty URL keeps its
// stored URL.
func (s *Service) SetNotify(ctx context.Context, n NotifySettings) (NotifySettings, error) {
	return updateSetting(ctx, s, keyNotify, NotifySettings{}, func(cur *NotifySettings) error {
		next, err := s.mergeNotify(*cur, n)
		if err != nil {
			return err
		}
		*cur = next
		return nil
	})
}

// UpdateNotify applies fn to the stored notification settings atomically. It
// is for internal bookkeeping (delivery status); fn's result is not
// validated.
func (s *Service) UpdateNotify(ctx context.Context, fn func(*NotifySettings)) error {
	_, err := updateSetting(ctx, s, keyNotify, NotifySettings{}, func(v *NotifySettings) error {
		fn(v)
		return nil
	})
	return err
}

func (s *Service) mergeNotify(cur, in NotifySettings) (NotifySettings, error) {
	const op = "settings"
	if in.PublicURL != "" {
		u, err := url.Parse(in.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return NotifySettings{}, invalidf(op, "public_url must be an http or https URL")
		}
		in.PublicURL = strings.TrimRight(in.PublicURL, "/")
	}
	if len(in.Webhooks) > MaxWebhooks {
		return NotifySettings{}, invalidf(op, "at most %d webhooks", MaxWebhooks)
	}
	old := map[string]Webhook{}
	for _, w := range cur.Webhooks {
		old[w.ID] = w
	}
	seen := map[string]bool{}
	out := NotifySettings{PublicURL: in.PublicURL, Webhooks: make([]Webhook, 0, len(in.Webhooks))}
	for _, w := range in.Webhooks {
		w.Name = strings.TrimSpace(w.Name)
		if n := utf8.RuneCountInString(w.Name); n < 1 || n > maxNameRunes {
			return NotifySettings{}, invalidf(op, "webhook name must be 1-%d characters", maxNameRunes)
		}
		prev, existing := old[w.ID]
		if w.ID != "" && !existing {
			return NotifySettings{}, invalidf(op, "unknown webhook id %q", w.ID)
		}
		if w.ID != "" && seen[w.ID] {
			return NotifySettings{}, invalidf(op, "duplicate webhook id %q", w.ID)
		}
		if existing {
			if w.URL == "" {
				w.URL = prev.URL
			}
			w.LastStatus, w.LastError, w.LastSentAt = prev.LastStatus, prev.LastError, prev.LastSentAt
			if w.URL != prev.URL { // a new endpoint has no history
				w.LastStatus, w.LastError, w.LastSentAt = 0, "", nil
			}
		}
		u, err := url.Parse(w.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return NotifySettings{}, invalidf(op, "webhook %q needs an http or https url", w.Name)
		}
		if w.ID == "" {
			id, err := s.newID(prefixWebhook, s.now())
			if err != nil {
				return NotifySettings{}, err
			}
			w.ID = id
		}
		seen[w.ID] = true
		out.Webhooks = append(out.Webhooks, w)
	}
	return out, nil
}

// SavedFilters lists the saved console filters in creation order.
func (s *Service) SavedFilters(ctx context.Context) ([]SavedFilter, error) {
	f, err := getSetting(ctx, s.store.Read(), keyFilters, []SavedFilter{})
	if f == nil {
		f = []SavedFilter{}
	}
	return f, err
}

func validateFilter(name string, params map[string]string) (string, error) {
	const op = "saved filter"
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameRunes {
		return "", invalidf(op, "name must be 1-%d characters", maxNameRunes)
	}
	if len(params) == 0 || len(params) > maxFilterParams {
		return "", invalidf(op, "params must have 1-%d entries", maxFilterParams)
	}
	for k, v := range params {
		if n := utf8.RuneCountInString(k); n < 1 || n > maxParamKeyRunes {
			return "", invalidf(op, "param names must be 1-%d characters", maxParamKeyRunes)
		}
		if utf8.RuneCountInString(v) > maxParamValueRunes {
			return "", invalidf(op, "param values must be at most %d characters", maxParamValueRunes)
		}
	}
	return name, nil
}

// CreateSavedFilter stores a new filter.
func (s *Service) CreateSavedFilter(ctx context.Context, name string, params map[string]string) (SavedFilter, error) {
	name, err := validateFilter(name, params)
	if err != nil {
		return SavedFilter{}, err
	}
	var out SavedFilter
	_, err = updateSetting(ctx, s, keyFilters, []SavedFilter{}, func(list *[]SavedFilter) error {
		if len(*list) >= MaxSavedFilters {
			return invalidf("saved filter", "at most %d saved filters", MaxSavedFilters)
		}
		id, err := s.newID(prefixFilter, s.now())
		if err != nil {
			return err
		}
		out = SavedFilter{ID: id, Name: name, Params: maps.Clone(params)}
		*list = append(*list, out)
		return nil
	})
	return out, err
}

// UpdateSavedFilter renames a filter and/or replaces its params; nil leaves a
// field unchanged.
func (s *Service) UpdateSavedFilter(ctx context.Context, id string, name *string, params map[string]string) (SavedFilter, error) {
	var out SavedFilter
	_, err := updateSetting(ctx, s, keyFilters, []SavedFilter{}, func(list *[]SavedFilter) error {
		i := slices.IndexFunc(*list, func(f SavedFilter) bool { return f.ID == id })
		if i < 0 {
			return fmt.Errorf("mail: saved filter: %w: no filter %q", ErrNotFound, id)
		}
		f := (*list)[i]
		if name != nil {
			f.Name = *name
		}
		if params != nil {
			f.Params = params
		}
		n, err := validateFilter(f.Name, f.Params)
		if err != nil {
			return err
		}
		f.Name, f.Params = n, maps.Clone(f.Params)
		(*list)[i] = f
		out = f
		return nil
	})
	return out, err
}

// DeleteSavedFilter removes a filter.
func (s *Service) DeleteSavedFilter(ctx context.Context, id string) error {
	_, err := updateSetting(ctx, s, keyFilters, []SavedFilter{}, func(list *[]SavedFilter) error {
		i := slices.IndexFunc(*list, func(f SavedFilter) bool { return f.ID == id })
		if i < 0 {
			return fmt.Errorf("mail: saved filter: %w: no filter %q", ErrNotFound, id)
		}
		*list = slices.Delete(*list, i, i+1)
		return nil
	})
	return err
}
