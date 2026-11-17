package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/notify"
	"github.com/scrothers/aim/internal/store"
)

// Maintenance runs backups and pruning; *maintenance.Runner implements it.
type Maintenance interface {
	BackupNow(ctx context.Context) (mail.BackupStatus, error)
	Prune(ctx context.Context, dryRun bool) (mail.PruneResult, error)
	Dir(ctx context.Context) (string, error)
}

// Notifier manages webhook notifications; *notify.Manager implements it.
type Notifier interface {
	Config(ctx context.Context) (notify.ConfigView, error)
	UpdateConfig(ctx context.Context, in mail.NotifySettings) (notify.ConfigView, error)
	SendTest(ctx context.Context, webhookID string) (notify.Result, error)
}

// BackupsBody is the backup schedule and where backups go.
type BackupsBody struct {
	EveryHours   int    `json:"every_hours" doc:"Hours between scheduled backups."`
	Keep         int    `json:"keep" doc:"Number of backups to keep."`
	Dir          string `json:"dir" doc:"Configured backup directory; empty selects the daemon default."`
	EffectiveDir string `json:"effective_dir" doc:"Directory backups are written to: dir, or the daemon default when dir is empty."`
}

// BackupsRequest replaces the backup schedule. Zero values select defaults.
type BackupsRequest struct {
	EveryHours int    `json:"every_hours,omitempty" doc:"Hours between scheduled backups (default 24)."`
	Keep       int    `json:"keep,omitempty" doc:"Number of backups to keep (default 7)."`
	Dir        string `json:"dir,omitempty" doc:"Absolute backup directory; empty selects the daemon default."`
}

// SettingsBody is every operator-editable setting. Webhook URLs are masked.
type SettingsBody struct {
	Retention    mail.RetentionSettings `json:"retention"`
	Backups      BackupsBody            `json:"backups"`
	BackupStatus mail.BackupStatus      `json:"backup_status"`
	Notify       notify.ConfigView      `json:"notify"`
}

// WebhookInput is one webhook in an update. An empty id adds a webhook; an
// empty url on an existing webhook keeps the stored URL.
type WebhookInput struct {
	ID      string `json:"id,omitempty" doc:"Existing webhook id; empty adds a new webhook."`
	Name    string `json:"name" doc:"Display name."`
	URL     string `json:"url,omitempty" doc:"Discord or Slack webhook URL; write-only. Empty keeps the stored URL of an existing webhook."`
	Enabled bool   `json:"enabled"`
}

// NotifyRequest replaces the notification settings. Webhooks omitted from
// the list are deleted.
type NotifyRequest struct {
	PublicURL string         `json:"public_url,omitempty" doc:"Console base URL used in notification links; empty selects the default."`
	Webhooks  []WebhookInput `json:"webhooks" doc:"The complete webhook list."`
}

// MutedBody lists muted threads.
type MutedBody struct {
	Items []mail.MutedThread `json:"items"`
}

// MuteBody reports a thread's mute state after a change.
type MuteBody struct {
	ThreadID string `json:"thread_id"`
	Muted    bool   `json:"muted"`
}

// FiltersBody lists saved filters.
type FiltersBody struct {
	Items []mail.SavedFilter `json:"items"`
}

// FilterRequest is the body of POST /api/filters.
type FilterRequest struct {
	Name   string            `json:"name" doc:"Display name."`
	Params map[string]string `json:"params" doc:"Console URL search params that reproduce the view."`
}

// FilterPatch is the body of PATCH /api/filters. Omitted fields are unchanged.
type FilterPatch struct {
	Name   *string           `json:"name,omitempty" doc:"New name."`
	Params map[string]string `json:"params,omitempty" doc:"Replacement params."`
}

type (
	webhookQuery struct {
		ID string `query:"id" required:"true" doc:"Webhook id."`
	}
	pruneInput struct {
		DryRun bool `query:"dry_run" doc:"Only count what would be deleted."`
	}
	threadQuery struct {
		ThreadID string `query:"thread_id" required:"true" doc:"Thread id."`
	}
	filterQuery struct {
		ID string `query:"id" required:"true" doc:"Saved filter id."`
	}
	filterUpdateInput struct {
		ID   string `query:"id" required:"true" doc:"Saved filter id."`
		Body FilterPatch
	}
)

func registerSettings(api huma.API, h *handlers) {
	op := func(method, id, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: []string{"aim"}}
	}
	huma.Register(api, op(http.MethodGet, "getSettings", "/api/settings", "Retention, backups and notification settings"), h.settings)
	huma.Register(api, op(http.MethodPut, "updateRetention", "/api/settings/retention", "Set the retention policy"), h.updateRetention)
	huma.Register(api, op(http.MethodPut, "updateBackups", "/api/settings/backups", "Set the backup schedule"), h.updateBackups)
	huma.Register(api, op(http.MethodPut, "updateNotify", "/api/settings/notify", "Replace webhooks and the public URL"), h.updateNotify)
	huma.Register(api, op(http.MethodPost, "testWebhook", "/api/settings/webhooks/test", "Send a test notification to one webhook"), h.testWebhook)
	huma.Register(api, op(http.MethodPost, "backupNow", "/api/settings/backup", "Write a database backup now"), h.backupNow)
	huma.Register(api, op(http.MethodPost, "pruneNow", "/api/settings/prune", "Apply the retention policy now"), h.pruneNow)

	huma.Register(api, op(http.MethodPost, "muteThread", "/api/threads/mute", "Mute a thread for the operator"), h.muteThread)
	huma.Register(api, op(http.MethodPost, "unmuteThread", "/api/threads/unmute", "Unmute a thread"), h.unmuteThread)
	huma.Register(api, op(http.MethodGet, "listMutedThreads", "/api/muted-threads", "Muted threads, newest first"), h.mutedThreads)

	huma.Register(api, op(http.MethodGet, "listSavedFilters", "/api/filters", "Saved console filters"), h.listFilters)
	huma.Register(api, func() huma.Operation {
		o := op(http.MethodPost, "createSavedFilter", "/api/filters", "Save a console filter")
		o.DefaultStatus = http.StatusCreated
		return o
	}(), h.createFilter)
	huma.Register(api, op(http.MethodPatch, "updateSavedFilter", "/api/filters", "Rename or replace a saved filter"), h.updateFilter)
	huma.Register(api, func() huma.Operation {
		o := op(http.MethodDelete, "deleteSavedFilter", "/api/filters", "Delete a saved filter")
		o.DefaultStatus = http.StatusNoContent
		return o
	}(), h.deleteFilter)
}

func unavailable(what string) error {
	return huma.NewError(http.StatusServiceUnavailable, what+" is not available")
}

func (h *handlers) settings(ctx context.Context, _ *struct{}) (*out[SettingsBody], error) {
	if h.maint == nil || h.notify == nil {
		return nil, unavailable("settings")
	}
	var b SettingsBody
	var err error
	if b.Retention, err = h.mail.Retention(ctx); err != nil {
		return nil, apiError(err)
	}
	if b.Backups, err = h.backupsBody(ctx); err != nil {
		return nil, apiError(err)
	}
	if b.BackupStatus, err = h.mail.BackupStatus(ctx); err != nil {
		return nil, apiError(err)
	}
	if b.Notify, err = h.notify.Config(ctx); err != nil {
		return nil, apiError(err)
	}
	return &out[SettingsBody]{Body: b}, nil
}

func (h *handlers) backupsBody(ctx context.Context) (BackupsBody, error) {
	s, err := h.mail.Backups(ctx)
	if err != nil {
		return BackupsBody{}, err
	}
	dir, err := h.maint.Dir(ctx)
	if err != nil {
		return BackupsBody{}, err
	}
	return BackupsBody{EveryHours: s.EveryHours, Keep: s.Keep, Dir: s.Dir, EffectiveDir: dir}, nil
}

func (h *handlers) updateRetention(ctx context.Context, in *struct{ Body mail.RetentionSettings }) (*out[mail.RetentionSettings], error) {
	if err := h.mail.SetRetention(ctx, in.Body); err != nil {
		return nil, apiError(err)
	}
	return &out[mail.RetentionSettings]{Body: in.Body}, nil
}

func (h *handlers) updateBackups(ctx context.Context, in *struct{ Body BackupsRequest }) (*out[BackupsBody], error) {
	if h.maint == nil {
		return nil, unavailable("backups")
	}
	req := in.Body
	if err := h.mail.SetBackups(ctx, mail.BackupSettings{EveryHours: req.EveryHours, Keep: req.Keep, Dir: req.Dir}); err != nil {
		return nil, apiError(err)
	}
	b, err := h.backupsBody(ctx)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[BackupsBody]{Body: b}, nil
}

func (h *handlers) updateNotify(ctx context.Context, in *struct{ Body NotifyRequest }) (*out[notify.ConfigView], error) {
	if h.notify == nil {
		return nil, unavailable("notifications")
	}
	n := mail.NotifySettings{PublicURL: in.Body.PublicURL, Webhooks: make([]mail.Webhook, len(in.Body.Webhooks))}
	for i, w := range in.Body.Webhooks {
		n.Webhooks[i] = mail.Webhook{ID: w.ID, Name: w.Name, URL: w.URL, Enabled: w.Enabled}
	}
	v, err := h.notify.UpdateConfig(ctx, n)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[notify.ConfigView]{Body: v}, nil
}

func (h *handlers) testWebhook(ctx context.Context, in *webhookQuery) (*out[notify.Result], error) {
	if h.notify == nil {
		return nil, unavailable("notifications")
	}
	r, err := h.notify.SendTest(ctx, in.ID)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[notify.Result]{Body: r}, nil
}

func (h *handlers) backupNow(ctx context.Context, _ *struct{}) (*out[mail.BackupStatus], error) {
	if h.maint == nil {
		return nil, unavailable("backups")
	}
	st, err := h.maint.BackupNow(ctx)
	// A failed backup is recorded in the status (last_error) and reported to
	// the console in the body rather than as an HTTP error.
	if err != nil && st.LastAt == nil {
		return nil, apiError(err)
	}
	return &out[mail.BackupStatus]{Body: st}, nil
}

func (h *handlers) pruneNow(ctx context.Context, in *pruneInput) (*out[mail.PruneResult], error) {
	if h.maint == nil {
		return nil, unavailable("maintenance")
	}
	r, err := h.maint.Prune(ctx, in.DryRun)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[mail.PruneResult]{Body: r}, nil
}

func (h *handlers) muteThread(ctx context.Context, in *threadQuery) (*out[MuteBody], error) {
	if err := h.mail.MuteThread(ctx, in.ThreadID); err != nil {
		return nil, apiError(err)
	}
	return &out[MuteBody]{Body: MuteBody{ThreadID: in.ThreadID, Muted: true}}, nil
}

func (h *handlers) unmuteThread(ctx context.Context, in *threadQuery) (*out[MuteBody], error) {
	if err := h.mail.UnmuteThread(ctx, in.ThreadID); err != nil {
		return nil, apiError(err)
	}
	return &out[MuteBody]{Body: MuteBody{ThreadID: in.ThreadID}}, nil
}

func (h *handlers) mutedThreads(ctx context.Context, _ *struct{}) (*out[MutedBody], error) {
	items, err := h.mail.MutedThreads(ctx)
	if err != nil {
		return nil, apiError(err)
	}
	if items == nil {
		items = []mail.MutedThread{}
	}
	return &out[MutedBody]{Body: MutedBody{Items: items}}, nil
}

func (h *handlers) listFilters(ctx context.Context, _ *struct{}) (*out[FiltersBody], error) {
	items, err := h.mail.SavedFilters(ctx)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[FiltersBody]{Body: FiltersBody{Items: items}}, nil
}

func (h *handlers) createFilter(ctx context.Context, in *struct{ Body FilterRequest }) (*out[mail.SavedFilter], error) {
	f, err := h.mail.CreateSavedFilter(ctx, in.Body.Name, in.Body.Params)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[mail.SavedFilter]{Body: f}, nil
}

func (h *handlers) updateFilter(ctx context.Context, in *filterUpdateInput) (*out[mail.SavedFilter], error) {
	f, err := h.mail.UpdateSavedFilter(ctx, in.ID, in.Body.Name, in.Body.Params)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[mail.SavedFilter]{Body: f}, nil
}

func (h *handlers) deleteFilter(ctx context.Context, in *filterQuery) (*struct{}, error) {
	if err := h.mail.DeleteSavedFilter(ctx, in.ID); err != nil {
		return nil, apiError(err)
	}
	return &struct{}{}, nil
}

// Segment is a run of snippet text, flagged when it matched the query.
type Segment struct {
	Text  string `json:"text" doc:"Plain text; render as text, never as HTML."`
	Match bool   `json:"match" doc:"True when this run matched the search query."`
}

// SearchHitView is a search hit with its snippet split into highlight
// segments.
type SearchHitView struct {
	mail.Hit
	Segments []Segment `json:"segments" doc:"The snippet as text runs; concatenated they equal snippet without the [brackets]."`
}

// SearchBody is one page of search hits.
type SearchBody struct {
	Hits       []SearchHitView `json:"hits"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

// segments splits a snippet carrying store.MarkOpen and store.MarkClose
// markers into text runs. Unbalanced or nested markers are tolerated.
func segments(marked string) []Segment {
	out := []Segment{}
	match := false
	add := func(text string) {
		if text != "" {
			out = append(out, Segment{Text: text, Match: match})
		}
	}
	rest := marked
	for rest != "" {
		i := strings.IndexAny(rest, store.MarkOpen+store.MarkClose)
		if i < 0 {
			add(rest)
			break
		}
		add(rest[:i])
		match = rest[i:i+1] == store.MarkOpen
		rest = rest[i+1:]
	}
	return out
}
