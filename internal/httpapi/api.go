package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/render"
	"github.com/scrothers/aim/internal/tasks"
)

type handlers struct {
	mail      *mail.Service
	presence  *presence.Registry
	render    *render.Renderer
	version   string
	caller    mail.Caller
	maint     Maintenance
	notify    Notifier
	approvals Approver
	tasks     *tasks.Service
}

// operatorCaller is the only actor of the HTTP API: the human operator.
func operatorCaller() mail.Caller {
	machine, _ := os.Hostname()
	return mail.OperatorCaller("http-operator", "aim-web", machine)
}

// Input and output envelopes.
type (
	identityQuery struct {
		Address string `query:"address" required:"true" doc:"Identity address."`
	}
	updateInput struct {
		Address string `query:"address" required:"true" doc:"Identity address."`
		Body    UpdateRequest
	}
	idPath struct {
		ID string `path:"id" doc:"Message id."`
	}
	viewsInput struct {
		TodaySince time.Time `query:"today_since" doc:"Start of the operator's day, RFC 3339. Defaults to 00:00 UTC."`
	}
	listInput struct {
		Address string    `query:"address" doc:"Mailbox address; empty lists every mailbox."`
		State   string    `query:"state" enum:"unread,read,archived,inbox,all" doc:"Delivery state filter (default unread). inbox is read or unread, not archived."`
		From    string    `query:"from" doc:"Only messages from this address."`
		Since   time.Time `query:"since" doc:"Only messages created after this time, RFC 3339."`
		Until   time.Time `query:"until" doc:"Only messages created before this time, RFC 3339."`
		Cursor  string    `query:"cursor" doc:"next_cursor from the previous page."`
		Limit   int       `query:"limit" minimum:"1" maximum:"200" doc:"Page size (default 50)."`
		// IncludeMuted lists operator mail in muted threads, hidden by default.
		IncludeMuted bool `query:"include_muted" doc:"Include operator mail in muted threads."`
	}
	searchInput struct {
		Address string    `query:"address" doc:"Mailbox address; empty searches everything."`
		Query   string    `query:"q" required:"true" doc:"FTS5 query."`
		From    string    `query:"from" doc:"Only messages from this address."`
		Since   time.Time `query:"since" doc:"Only messages created at or after this time, RFC 3339."`
		Until   time.Time `query:"until" doc:"Only messages created at or before this time, RFC 3339."`
		State   string    `query:"state" enum:"unread,read,archived,inbox,all" doc:"Delivery state filter (default all)."`
		Cursor  string    `query:"cursor" doc:"next_cursor from the previous page."`
		Limit   int       `query:"limit" minimum:"1" maximum:"100" doc:"Page size (default 20)."`
	}
	rosterInput struct {
		Address        string `query:"address" doc:"Scope address; empty lists every identity."`
		IncludeRetired bool   `query:"include_retired" doc:"Include retired identities."`
	}
	out[T any] struct{ Body T }
)

func registerOperations(api huma.API, h *handlers) {
	get := func(id, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: http.MethodGet, Path: path, Summary: summary, Tags: []string{"aim"}}
	}
	post := func(id, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: http.MethodPost, Path: path, Summary: summary, Tags: []string{"aim"}}
	}

	huma.Register(api, get("getViews", "/api/views", "Navigation badge counts"), h.views)
	huma.Register(api, get("getTree", "/api/tree", "Mailbox tree with unread counts and activity"), h.tree)
	huma.Register(api, get("getPresence", "/api/presence", "Buddy list and recent presence events"), h.presenceList)
	huma.Register(api, get("getStats", "/api/stats", "Daemon totals"), h.stats)
	huma.Register(api, get("listMessages", "/api/messages", "List message headers"), h.list)
	huma.Register(api, get("getMessage", "/api/messages/{id}", "Read a message without marking it read"), h.message)
	huma.Register(api, post("readMessage", "/api/messages/{id}/read", "Read a message and mark the operator's delivery read"), h.readMessage)
	huma.Register(api, get("getThread", "/api/threads/{id}", "Every message in a thread, oldest first"), h.thread)
	huma.Register(api, func() huma.Operation {
		o := post("sendMessage", "/api/messages", "Send a message as the operator")
		o.DefaultStatus = http.StatusCreated
		return o
	}(), h.send)
	huma.Register(api, post("markMessages", "/api/marks", "Mark operator deliveries read, unread or archived"), h.mark)
	huma.Register(api, get("searchMessages", "/api/search", "Full-text search"), h.search)
	huma.Register(api, get("getRoster", "/api/roster", "Identities with presence"), h.roster)
	huma.Register(api, huma.Operation{
		OperationID: "updateIdentity", Method: http.MethodPatch, Path: "/api/identities",
		Summary: "Update an identity's role, description or reports_to", Tags: []string{"aim"},
	}, h.updateIdentity)
	huma.Register(api, post("retireIdentity", "/api/identities/retire", "Retire an identity"), h.retire)
	huma.Register(api, post("unretireIdentity", "/api/identities/unretire", "Unretire an identity"), h.unretire)
	huma.Register(api, get("getOrgChart", "/api/orgchart", "Reporting graph with presence"), h.orgChart)
	huma.Register(api, post("renderMarkdown", "/api/render", "Preview Markdown as sanitized HTML"), h.renderPreview)
	registerSettings(api, h)
	registerApprovals(api, h)
	registerTasks(api, h)
}

// apiError maps a mail error to an HTTP error.
func apiError(err error) error {
	var nf *mail.MailboxNotFoundError
	switch {
	case errors.As(err, &nf):
		var details []error
		if len(nf.Suggestions) > 0 {
			details = append(details, &huma.ErrorDetail{Message: "did you mean one of these addresses?", Location: "suggestions", Value: nf.Suggestions})
		}
		return huma.NewError(http.StatusNotFound, nf.Error(), details...)
	case errors.Is(err, mail.ErrMessageNotFound), errors.Is(err, mail.ErrNotFound):
		return huma.NewError(http.StatusNotFound, err.Error())
	case errors.Is(err, mail.ErrReserved), errors.Is(err, mail.ErrCycle), errors.Is(err, mail.ErrRetired):
		// Checked before invalid input: ErrCycle also wraps ErrInvalidInput.
		return huma.NewError(http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, mail.ErrInvalidInput), errors.Is(err, mail.ErrBadQuery):
		return huma.NewError(http.StatusBadRequest, err.Error())
	case errors.Is(err, mail.ErrExists):
		return huma.NewError(http.StatusConflict, err.Error())
	default:
		return huma.NewError(http.StatusInternalServerError, "internal error", err)
	}
}

func (h *handlers) view(m mail.Message) (MessageView, error) {
	html, err := h.render.HTML(m.Body)
	if err != nil {
		return MessageView{}, huma.NewError(http.StatusInternalServerError, "render failed", err)
	}
	return MessageView{Message: m, BodyHTML: html, Snippet: h.render.Snippet(m.Body, snippetRunes)}, nil
}

func (h *handlers) views(ctx context.Context, in *viewsInput) (*out[mail.ViewCounts], error) {
	since := in.TodaySince
	if since.IsZero() {
		now := time.Now().UTC()
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	v, err := h.mail.Views(ctx, since)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[mail.ViewCounts]{Body: v}, nil
}

func (h *handlers) tree(ctx context.Context, _ *struct{}) (*out[mail.Tree], error) {
	t, err := h.mail.Tree(ctx)
	if err != nil {
		return nil, apiError(err)
	}
	if t.Orgs == nil {
		t.Orgs = []mail.OrgNode{}
	}
	return &out[mail.Tree]{Body: t}, nil
}

func (h *handlers) stats(ctx context.Context, _ *struct{}) (*out[StatsBody], error) {
	s, err := h.mail.Stats(ctx)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[StatsBody]{Body: StatsBody{Stats: s, Version: h.version}}, nil
}

func (h *handlers) list(ctx context.Context, in *listInput) (*out[mail.ListResult], error) {
	li := mail.ListInput{Address: in.Address, State: in.State, From: in.From, Limit: in.Limit, Cursor: in.Cursor, IncludeMuted: in.IncludeMuted}
	if !in.Since.IsZero() {
		li.Since = &in.Since
	}
	if !in.Until.IsZero() {
		li.Until = &in.Until
	}
	r, err := h.mail.List(ctx, li)
	if err != nil {
		return nil, apiError(err)
	}
	if r.Items == nil {
		r.Items = []mail.Header{}
	}
	return &out[mail.ListResult]{Body: r}, nil
}

func (h *handlers) message(ctx context.Context, in *idPath) (*out[MessageView], error) {
	m, err := h.mail.Read(ctx, h.caller, mail.ReadInput{MessageID: in.ID, Peek: true})
	if err != nil {
		return nil, apiError(err)
	}
	v, err := h.view(m)
	if err != nil {
		return nil, err
	}
	return &out[MessageView]{Body: v}, nil
}

func (h *handlers) readMessage(ctx context.Context, in *idPath) (*out[MessageView], error) {
	m, err := h.mail.Read(ctx, h.caller, mail.ReadInput{MessageID: in.ID})
	if err != nil {
		return nil, apiError(err)
	}
	v, err := h.view(m)
	if err != nil {
		return nil, err
	}
	return &out[MessageView]{Body: v}, nil
}

func (h *handlers) thread(ctx context.Context, in *idPath) (*out[ThreadBody], error) {
	msgs, err := h.mail.Thread(ctx, in.ID)
	if err != nil {
		return nil, apiError(err)
	}
	body := ThreadBody{Messages: make([]MessageView, 0, len(msgs))}
	for _, m := range msgs {
		v, err := h.view(m)
		if err != nil {
			return nil, err
		}
		body.Messages = append(body.Messages, v)
	}
	return &out[ThreadBody]{Body: body}, nil
}

func (h *handlers) send(ctx context.Context, in *struct{ Body SendRequest }) (*out[MessageView], error) {
	b := in.Body
	m, err := h.mail.Send(ctx, h.caller, mail.SendInput{
		To: b.To, Cc: b.Cc, Subject: b.Subject, Body: b.Body, InReplyTo: b.InReplyTo, ReplyAll: b.ReplyAll,
	})
	if err != nil {
		return nil, apiError(err)
	}
	v, err := h.view(m)
	if err != nil {
		return nil, err
	}
	return &out[MessageView]{Body: v}, nil
}

func (h *handlers) mark(ctx context.Context, in *struct{ Body MarkRequest }) (*out[MarkBody], error) {
	n, err := h.mail.Mark(ctx, h.caller, mail.MarkInput{MessageIDs: in.Body.MessageIDs, State: in.Body.State})
	if err != nil {
		return nil, apiError(err)
	}
	return &out[MarkBody]{Body: MarkBody{Updated: n}}, nil
}

func (h *handlers) search(ctx context.Context, in *searchInput) (*out[SearchBody], error) {
	si := mail.SearchInput{
		Address: in.Address, Query: in.Query, From: in.From, State: in.State, Limit: in.Limit, Cursor: in.Cursor,
	}
	if !in.Since.IsZero() {
		si.Since = &in.Since
	}
	if !in.Until.IsZero() {
		si.Until = &in.Until
	}
	r, err := h.mail.Search(ctx, si)
	if err != nil {
		return nil, apiError(err)
	}
	body := SearchBody{Hits: make([]SearchHitView, len(r.Hits)), NextCursor: r.NextCursor}
	for i, hit := range r.Hits {
		body.Hits[i] = SearchHitView{Hit: hit, Segments: segments(hit.Marked)}
	}
	return &out[SearchBody]{Body: body}, nil
}

func (h *handlers) roster(ctx context.Context, in *rosterInput) (*out[RosterBody], error) {
	items, err := h.mail.RosterWith(ctx, in.Address, mail.RosterOptions{IncludeRetired: in.IncludeRetired})
	if err != nil {
		return nil, apiError(err)
	}
	return &out[RosterBody]{Body: RosterBody{Items: items}}, nil
}

func (h *handlers) updateIdentity(ctx context.Context, in *updateInput) (*out[mail.Identity], error) {
	id, err := h.mail.UpdateIdentity(ctx, in.Address, mail.UpdateInput{
		Role: in.Body.Role, Description: in.Body.Description, ReportsTo: in.Body.ReportsTo,
	})
	if err != nil {
		return nil, apiError(err)
	}
	return &out[mail.Identity]{Body: id}, nil
}

func (h *handlers) retire(ctx context.Context, in *identityQuery) (*out[mail.Identity], error) {
	id, err := h.mail.Retire(ctx, in.Address)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[mail.Identity]{Body: id}, nil
}

func (h *handlers) unretire(ctx context.Context, in *identityQuery) (*out[mail.Identity], error) {
	id, err := h.mail.Unretire(ctx, in.Address)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[mail.Identity]{Body: id}, nil
}

func (h *handlers) orgChart(ctx context.Context, _ *struct{}) (*out[mail.OrgChart], error) {
	c, err := h.mail.OrgChart(ctx)
	if err != nil {
		return nil, apiError(err)
	}
	if c.Nodes == nil {
		c.Nodes = []mail.OrgChartNode{}
	}
	if c.Edges == nil {
		c.Edges = []mail.OrgChartEdge{}
	}
	return &out[mail.OrgChart]{Body: c}, nil
}

func (h *handlers) renderPreview(_ context.Context, in *struct{ Body RenderRequest }) (*out[RenderBody], error) {
	html, err := h.render.HTML(in.Body.Markdown)
	if err != nil {
		return nil, huma.NewError(http.StatusInternalServerError, "render failed", err)
	}
	return &out[RenderBody]{Body: RenderBody{HTML: html}}, nil
}

func (h *handlers) presenceList(ctx context.Context, _ *struct{}) (*out[PresenceBody], error) {
	roster, err := h.mail.RosterWith(ctx, "", mail.RosterOptions{})
	if err != nil {
		return nil, apiError(err)
	}
	return &out[PresenceBody]{Body: buildPresence(roster, h.presence)}, nil
}
