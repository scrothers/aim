package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/store/db"
)

// rawMessage unifies the sqlc row types for messages.
type rawMessage struct {
	id, threadID, inReplyTo, subject, body string
	from                                   address.Address
	createdAt                              int64
}

func (r rawMessage) message(to, cc []string) Message {
	return Message{
		ID: r.id, ThreadID: r.threadID, InReplyTo: r.inReplyTo, From: r.from.String(),
		Subject: r.subject, Body: r.body, CreatedAt: fromMS(r.createdAt), To: to, Cc: cc,
	}
}

// recipients returns the to and cc address lists of a message.
func recipients(ctx context.Context, q *db.Queries, id string) (to, cc []string, err error) {
	rows, err := q.ListDeliveriesForMessage(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("mail: recipients: %w", err)
	}
	to, cc = []string{}, []string{}
	for _, r := range rows {
		a := rowAddr(r.Host, r.Org, r.Repo, r.Identity, r.Subagent).String()
		if r.Kind == kindTo {
			to = append(to, a)
		} else {
			cc = append(cc, a)
		}
	}
	return to, cc, nil
}

func loadMessage(ctx context.Context, q *db.Queries, id string) (Message, error) {
	m, err := q.GetMessageByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, fmt.Errorf("mail: read: %w: no message %q; ids come from list, search or thread", ErrMessageNotFound, id)
	}
	if err != nil {
		return Message{}, fmt.Errorf("mail: read: %w", err)
	}
	to, cc, err := recipients(ctx, q, id)
	if err != nil {
		return Message{}, err
	}
	raw := rawMessage{
		id: m.ID, threadID: m.ThreadID, inReplyTo: m.InReplyTo.String, subject: m.Subject, body: m.Body,
		createdAt: m.CreatedAt, from: rowAddr(m.FromHost, m.FromOrg, m.FromRepo, m.FromIdentity, m.FromSubagent),
	}
	return raw.message(to, cc), nil
}

// Read returns a message. Without Peek it also marks the caller's own
// delivery read (the caller must be signed on); with Peek nothing changes and
// no sign-on is needed.
func (s *Service) Read(ctx context.Context, c Caller, in ReadInput) (Message, error) {
	const op = "read"
	if in.MessageID == "" {
		return Message{}, invalidf(op, "message_id is required")
	}
	if in.Peek {
		return loadMessage(ctx, s.store.Read(), in.MessageID)
	}
	if err := requireSignedOn(op, c); err != nil {
		return Message{}, fmt.Errorf("%w (or pass peek=true to read without marking)", err)
	}
	now := s.now().UnixMilli()
	var out Message
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		if out, err = loadMessage(ctx, q, in.MessageID); err != nil {
			return err
		}
		mb, err := callerMailbox(ctx, q, op, c, in.Subagent)
		if err != nil {
			return err
		}
		d, err := q.GetDelivery(ctx, db.GetDeliveryParams{MessageID: in.MessageID, MailboxID: mb.ID})
		if errors.Is(err, sql.ErrNoRows) || (err == nil && d.ReadAt.Valid) {
			return nil // not ours, or already read
		}
		if err != nil {
			return fmt.Errorf("mail: %s: delivery: %w", op, err)
		}
		_, err = q.SetReadAt(ctx, db.SetReadAtParams{Now: sql.NullInt64{Int64: now, Valid: true}, MessageID: in.MessageID, MailboxID: mb.ID})
		if err != nil {
			return fmt.Errorf("mail: %s: mark read: %w", op, err)
		}
		return nil
	})
	if err != nil {
		return Message{}, err
	}
	return out, nil
}

// Thread returns every message in a thread, oldest first.
func (s *Service) Thread(ctx context.Context, threadID string) ([]Message, error) {
	if threadID == "" {
		return nil, invalidf("thread", "thread_id is required")
	}
	q := s.store.Read()
	rows, err := q.ListThread(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("mail: thread: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("mail: thread: %w: no thread %q; thread ids appear on every message", ErrMessageNotFound, threadID)
	}
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		to, cc, err := recipients(ctx, q, r.ID)
		if err != nil {
			return nil, err
		}
		raw := rawMessage{
			id: r.ID, threadID: r.ThreadID, inReplyTo: r.InReplyTo.String, subject: r.Subject, body: r.Body,
			createdAt: r.CreatedAt, from: rowAddr(r.FromHost, r.FromOrg, r.FromRepo, r.FromIdentity, r.FromSubagent),
		}
		out = append(out, raw.message(to, cc))
	}
	return out, nil
}

func ptrMS(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

func (s *Service) listParams(ctx context.Context, in ListInput) (db.ListInboxParams, bool, error) {
	const op = "list"
	a, err := parseAddr(op, in.Address)
	if err != nil {
		return db.ListInboxParams{}, false, err
	}
	state := in.State
	if state == "" {
		state = StateUnread
	}
	if !slices.Contains([]string{StateUnread, StateRead, StateArchived, StateAll}, state) {
		return db.ListInboxParams{}, false, invalidf(op, "state %q is not one of unread, read, archived, all", state)
	}
	if state == StateArchived {
		if in.Since == nil || in.Until == nil {
			return db.ListInboxParams{}, false, invalidf(op, "state=archived requires both since and until to bound the window")
		}
	}
	if in.Since != nil && in.Until != nil && in.Until.Before(*in.Since) {
		return db.ListInboxParams{}, false, invalidf(op, "until must not be before since")
	}
	if in.Limit < 0 {
		return db.ListInboxParams{}, false, invalidf(op, "limit must not be negative")
	}
	if in.Cursor != "" && !strings.HasPrefix(in.Cursor, prefixMessage) {
		return db.ListInboxParams{}, false, invalidf(op, "cursor %q is not valid; pass next_cursor from the previous page", in.Cursor)
	}
	sc := a.Scope()
	p := db.ListInboxParams{
		Host: sc.Host, Org: sc.Org, MatchRepo: b2i(sc.MatchRepo), Repo: sc.Repo,
		MatchIdentity: b2i(sc.MatchIdentity), Identity: sc.Identity,
		MatchSubagent: b2i(sc.MatchSubagent), Subagent: sc.Subagent,
		Since: ptrMS(in.Since), Until: ptrMS(in.Until), State: state,
		BeforeID: sql.NullString{String: in.Cursor, Valid: in.Cursor != ""},
	}
	if in.From != "" {
		fa, err := parseAddr(op, in.From)
		if err != nil {
			return db.ListInboxParams{}, false, err
		}
		m, err := s.store.Read().GetMailboxByKey(ctx, keyOf(fa))
		if errors.Is(err, sql.ErrNoRows) {
			return p, true, nil // nobody by that address has sent anything
		}
		if err != nil {
			return db.ListInboxParams{}, false, fmt.Errorf("mail: %s: from: %w", op, err)
		}
		p.FromMailbox = sql.NullString{String: m.ID, Valid: true}
	}
	return p, false, nil
}

// List returns delivery headers for the subtree at in.Address, newest first.
func (s *Service) List(ctx context.Context, in ListInput) (ListResult, error) {
	const op = "list"
	p, empty, err := s.listParams(ctx, in)
	if err != nil {
		return ListResult{}, err
	}
	if empty {
		return ListResult{Items: []Header{}}, nil
	}
	limit := in.Limit
	if limit == 0 {
		limit = DefaultListLimit
	}
	limit = min(limit, MaxListLimit)
	p.PageLimit = int64(limit) + 1
	q := s.store.Read()
	rows, err := q.ListInbox(ctx, p)
	if err != nil {
		return ListResult{}, fmt.Errorf("mail: %s: %w", op, err)
	}
	next := ""
	if len(rows) > limit {
		extra := rows[limit].MessageID
		rows = rows[:limit]
		// Cursor is a message id, so a message must not straddle pages: drop
		// the trailing message's rows when more of them follow on the next page.
		if extra == rows[limit-1].MessageID && trailingRun(rows, extra) < len(rows) {
			rows = rows[:len(rows)-trailingRun(rows, extra)]
		}
		next = rows[len(rows)-1].MessageID
	}
	items, err := s.headers(ctx, q, rows)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, NextCursor: next}, nil
}

func trailingRun(rows []db.ListInboxRow, id string) int {
	n := 0
	for i := len(rows) - 1; i >= 0 && rows[i].MessageID == id; i-- {
		n++
	}
	return n
}

func mailboxIndex(ctx context.Context, q *db.Queries, ids []string) (map[string]address.Address, error) {
	idx := map[string]address.Address{}
	ids = slices.Compact(slices.Sorted(slices.Values(ids)))
	if len(ids) == 0 {
		return idx, nil
	}
	mbs, err := q.ListMailboxesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("mail: mailboxes: %w", err)
	}
	for _, m := range mbs {
		idx[m.ID] = addrOf(m)
	}
	return idx, nil
}

func (s *Service) headers(ctx context.Context, q *db.Queries, rows []db.ListInboxRow) ([]Header, error) {
	froms := make([]string, len(rows))
	for i, r := range rows {
		froms[i] = r.FromMailbox
	}
	idx, err := mailboxIndex(ctx, q, froms)
	if err != nil {
		return nil, err
	}
	type pair struct{ to, cc []string }
	recips := map[string]pair{}
	items := make([]Header, 0, len(rows))
	for _, r := range rows {
		p, ok := recips[r.MessageID]
		if !ok {
			if p.to, p.cc, err = recipients(ctx, q, r.MessageID); err != nil {
				return nil, err
			}
			recips[r.MessageID] = p
		}
		items = append(items, Header{
			ID: r.MessageID, ThreadID: r.ThreadID, InReplyTo: r.InReplyTo.String,
			From: idx[r.FromMailbox].String(), Subject: r.Subject, CreatedAt: fromMS(r.CreatedAt),
			To: p.to, Cc: p.cc,
			Mailbox: rowAddr(r.Host, r.Org, r.Repo, r.Identity, r.Subagent).String(), Kind: r.Kind,
			ReadAt: nullTime(r.ReadAt), ArchivedAt: nullTime(r.ArchivedAt),
		})
	}
	return items, nil
}

// Mark changes read or archive state on deliveries the caller owns and
// returns how many actually changed. Ids the caller has no delivery for are
// ignored.
func (s *Service) Mark(ctx context.Context, c Caller, in MarkInput) (int, error) {
	const op = "mark"
	if err := requireSignedOn(op, c); err != nil {
		return 0, err
	}
	if !slices.Contains([]string{MarkRead, MarkUnread, MarkArchived, MarkUnarchived}, in.State) {
		return 0, invalidf(op, "state %q is not one of read, unread, archived, unarchived", in.State)
	}
	if len(in.MessageIDs) == 0 || len(in.MessageIDs) > MaxListLimit {
		return 0, invalidf(op, "message_ids must list 1-%d ids", MaxListLimit)
	}
	now := sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true}
	changed := 0
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		changed = 0
		mb, err := callerMailbox(ctx, q, op, c, in.Subagent)
		if err != nil {
			return err
		}
		for _, id := range slices.Compact(slices.Sorted(slices.Values(in.MessageIDs))) {
			d, err := q.GetDelivery(ctx, db.GetDeliveryParams{MessageID: id, MailboxID: mb.ID})
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("mail: %s: delivery: %w", op, err)
			}
			n, err := applyMark(ctx, q, in.State, d, now)
			if err != nil {
				return fmt.Errorf("mail: %s: %w", op, err)
			}
			changed += n
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

func applyMark(ctx context.Context, q *db.Queries, state string, d db.Delivery, now sql.NullInt64) (int, error) {
	var n int64
	var err error
	switch {
	case state == MarkRead && !d.ReadAt.Valid:
		n, err = q.SetReadAt(ctx, db.SetReadAtParams{Now: now, MessageID: d.MessageID, MailboxID: d.MailboxID})
	case state == MarkUnread && d.ReadAt.Valid:
		n, err = q.ClearReadAt(ctx, db.ClearReadAtParams{MessageID: d.MessageID, MailboxID: d.MailboxID})
	case state == MarkArchived && !d.ArchivedAt.Valid:
		n, err = q.SetArchivedAt(ctx, db.SetArchivedAtParams{Now: now, MessageID: d.MessageID, MailboxID: d.MailboxID})
	case state == MarkUnarchived && d.ArchivedAt.Valid:
		n, err = q.ClearArchivedAt(ctx, db.ClearArchivedAtParams{MessageID: d.MessageID, MailboxID: d.MailboxID})
	}
	return int(n), err
}

// Search runs a full-text query over messages delivered inside in.Address.
func (s *Service) Search(ctx context.Context, in SearchInput) (SearchResult, error) {
	const op = "search"
	a, err := parseAddr(op, in.Address)
	if err != nil {
		return SearchResult{}, err
	}
	if in.Limit < 0 {
		return SearchResult{}, invalidf(op, "limit must not be negative")
	}
	limit := in.Limit
	if limit == 0 {
		limit = DefaultSearchLimit
	}
	limit = min(limit, MaxSearchLimit)
	offset := 0
	if in.Cursor != "" {
		if offset, err = strconv.Atoi(in.Cursor); err != nil || offset < 0 {
			return SearchResult{}, invalidf(op, "cursor %q is not valid; pass next_cursor from the previous page", in.Cursor)
		}
	}
	sc := a.Scope()
	hits, err := s.store.Search(ctx, store.SearchParams{
		Host: sc.Host, Org: sc.Org, Repo: sc.Repo, Identity: sc.Identity, Subagent: sc.Subagent,
		MatchRepo: sc.MatchRepo, MatchIdentity: sc.MatchIdentity, MatchSubagent: sc.MatchSubagent,
		Query: in.Query, Limit: limit + 1, Offset: offset,
	})
	if errors.Is(err, store.ErrBadQuery) {
		return SearchResult{}, fmt.Errorf("mail: %s: %w (use plain words or \"quoted phrases\"): %w", op, ErrBadQuery, err)
	}
	if err != nil {
		return SearchResult{}, fmt.Errorf("mail: %s: %w", op, err)
	}
	next := ""
	if len(hits) > limit {
		hits = hits[:limit]
		next = strconv.Itoa(offset + limit)
	}
	ids := make([]string, 0, 2*len(hits))
	for _, h := range hits {
		ids = append(ids, h.FromMailboxID, h.MailboxID)
	}
	idx, err := mailboxIndex(ctx, s.store.Read(), ids)
	if err != nil {
		return SearchResult{}, err
	}
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, Hit{
			MessageID: h.MessageID, ThreadID: h.ThreadID, Subject: h.Subject, Snippet: h.Snippet,
			From: idx[h.FromMailboxID].String(), Mailbox: idx[h.MailboxID].String(),
			CreatedAt: fromMS(h.CreatedAt), Rank: h.Rank,
		})
	}
	return SearchResult{Hits: out, NextCursor: next}, nil
}
