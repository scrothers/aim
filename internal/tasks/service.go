package tasks

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/store/db"
)

// Default decay thresholds, measured from a held task's last activity.
const (
	DefaultIdleAfter    = 15 * time.Minute
	DefaultStaleAfter   = 30 * time.Minute
	DefaultAbandonAfter = 60 * time.Minute
	DefaultMaxHold      = 4 * time.Hour
)

// Content limits.
const (
	MaxTitleLen = 200
	MaxBodyLen  = 32 << 10
	MaxNoteLen  = 8 << 10
)

// touchGranularity is how stale a held task's activity must be before Touch
// writes; it keeps Touch to at most one write per second per session.
const touchGranularity = time.Second

const idPrefix = "task-"

// Decay holds the lease thresholds. A held task is idle after IdleAfter,
// stale after StaleAfter, and abandoned after AbandonAfter when its claimant
// session is disconnected, or after MaxHold regardless.
type Decay struct {
	IdleAfter    time.Duration
	StaleAfter   time.Duration
	AbandonAfter time.Duration
	MaxHold      time.Duration
}

// withDefaults fills zero thresholds with the defaults.
func (d Decay) withDefaults() Decay {
	if d.IdleAfter <= 0 {
		d.IdleAfter = DefaultIdleAfter
	}
	if d.StaleAfter <= 0 {
		d.StaleAfter = DefaultStaleAfter
	}
	if d.AbandonAfter <= 0 {
		d.AbandonAfter = DefaultAbandonAfter
	}
	if d.MaxHold <= 0 {
		d.MaxHold = DefaultMaxHold
	}
	return d
}

// Sessions reports whether an agent session has a live connection.
type Sessions interface {
	Connected(sessionID string) bool
}

type noSessions struct{}

func (noSessions) Connected(string) bool { return false }

// Option configures a Service.
type Option func(*Service)

// WithClock sets the time source. The default is time.Now.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithDecay sets the lease thresholds; zero fields keep their defaults.
func WithDecay(d Decay) Option { return func(s *Service) { s.decay = d } }

// WithSessions sets the live-connection check used by abandonment. Without
// it every session counts as disconnected.
func WithSessions(sc Sessions) Option { return func(s *Service) { s.sessions = sc } }

// WithEntropy sets the ULID entropy source (default crypto/rand).
func WithEntropy(r io.Reader) Option {
	return func(s *Service) { s.entropy = ulid.Monotonic(r, 0) }
}

// WithLogger sets the logger for best-effort failures such as undeliverable
// notifications. The default discards.
func WithLogger(l *slog.Logger) Option { return func(s *Service) { s.log = l } }

// Service is the task board. Use New; it is safe for concurrent use.
type Service struct {
	store    *store.Store
	mail     *mail.Service
	sessions Sessions
	now      func() time.Time
	decay    Decay
	log      *slog.Logger
	wake     notifier

	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

// New returns a Service over the store, using the mail service to resolve
// identities, check linked messages and send notifications.
func New(st *store.Store, m *mail.Service, opts ...Option) *Service {
	s := &Service{store: st, mail: m, now: time.Now, sessions: noSessions{}, log: slog.New(slog.DiscardHandler)}
	WithEntropy(rand.Reader)(s)
	for _, o := range opts {
		o(s)
	}
	if s.sessions == nil {
		s.sessions = noSessions{}
	}
	s.decay = s.decay.withDefaults()
	return s
}

// Decay returns the effective lease thresholds.
func (s *Service) Decay() Decay { return s.decay }

func (s *Service) newID(t time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := ulid.New(ulid.Timestamp(t), s.entropy)
	if err != nil {
		return "", fmt.Errorf("tasks: new id: %w", err)
	}
	return idPrefix + strings.ToLower(id.String()), nil
}

// actorOf returns the person c acts as: "operator" for the operator, the
// identity address for a signed-on agent, or an error for anyone else.
func actorOf(op string, c mail.Caller) (string, error) {
	if c.Operator {
		return Operator, nil
	}
	if !c.SignedOn || c.Identity.Kind() != address.KindIdentity {
		return "", fmt.Errorf("tasks: %s: %w: call agent_sign_on with a registered identity first; unsigned sessions may only read tasks", op, mail.ErrNotSignedOn)
	}
	return c.Identity.IdentityAddress().String(), nil
}

func invalidf(op, format string, args ...any) error {
	return fmt.Errorf("tasks: %s: %w: %s", op, mail.ErrInvalidInput, fmt.Sprintf(format, args...))
}

func notFound(op, id string) error {
	return fmt.Errorf("tasks: %s: %w: no task %q; use task_list to find task ids", op, ErrNotFound, id)
}

// resolvePerson validates an identity address (or "operator") and returns
// its canonical form.
func (s *Service) resolvePerson(ctx context.Context, op, addr string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(addr), Operator) {
		return Operator, nil
	}
	a, err := s.mail.ResolveIdentity(ctx, addr)
	if err != nil {
		return "", fmt.Errorf("tasks: %s: %w", op, err)
	}
	return a.String(), nil
}

// normalizePerson canonicalizes a filter address without requiring it to be
// registered.
func normalizePerson(op, field, addr string) (string, error) {
	if addr == "" {
		return "", nil
	}
	a, err := address.Parse(addr)
	if err != nil {
		return "", invalidf(op, "%s: %v", field, err)
	}
	return a.String(), nil
}

// normalizeScope validates an org or repo address.
func normalizeScope(op, scope string) (string, error) {
	if strings.TrimSpace(scope) == "" {
		return "", nil
	}
	a, err := address.Parse(scope)
	if err != nil {
		return "", invalidf(op, "scope: %v", err)
	}
	if a.Kind() != address.KindNamespace {
		return "", invalidf(op, "scope %q is not an org or repo address (it names a %s); use one like github.com/acme or github.com/acme/api", scope, a.Kind())
	}
	return a.String(), nil
}

func validateTitle(op, title string) (string, error) {
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return "", invalidf(op, "title is required; give a one-line summary of the work")
	case len(title) > MaxTitleLen:
		return "", invalidf(op, "title is %d bytes; keep it to %d and put details in body", len(title), MaxTitleLen)
	case strings.ContainsAny(title, "\r\n"):
		return "", invalidf(op, "title must be one line; put details in body")
	}
	return title, nil
}

func validateBody(op, body string) error {
	if len(body) > MaxBodyLen {
		return invalidf(op, "body is %d bytes; the limit is %d", len(body), MaxBodyLen)
	}
	return nil
}

func validatePriority(op string, p Priority) (Priority, error) {
	if p == "" {
		return PriorityNormal, nil
	}
	for _, v := range Priorities {
		if p == v {
			return p, nil
		}
	}
	return "", invalidf(op, "priority %q is not one of low, normal, high, urgent", p)
}

func validateStatus(op string, st Status) error {
	if st == "" {
		return nil
	}
	for _, v := range Statuses {
		if st == v {
			return nil
		}
	}
	return invalidf(op, "status %q is not one of not_started, claimed, started, finished, abandoned", st)
}

// checkMessage returns an error unless id names an existing message.
func checkMessage(ctx context.Context, q *db.Queries, op, id string) error {
	if _, err := q.GetMessageByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("tasks: %s: %w: message %q does not exist; use a message id from list or read", op, mail.ErrMessageNotFound, id)
		}
		return fmt.Errorf("tasks: %s: message: %w", op, err)
	}
	return nil
}

func ms(t time.Time) int64 { return t.UnixMilli() }

func nullMS(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

func timePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromMS(v.Int64)
	return &t
}

// toTask converts a row and fills in Health.
func (s *Service) toTask(r db.Task, now time.Time) Task {
	return Task{
		ID: r.ID, Title: r.Title, Body: r.Body, Status: Status(r.Status), Priority: Priority(r.Priority),
		Scope: r.Scope, CreatedBy: r.CreatedBy, Assignee: r.Assignee, Claimant: r.Claimant,
		Health: s.health(r, now), DueAt: timePtr(r.DueAt), ClaimedAt: timePtr(r.ClaimedAt),
		LastActivityAt: timePtr(r.LastActivityAt), FinishedAt: timePtr(r.FinishedAt),
		CreatedAt: fromMS(r.CreatedAt), UpdatedAt: fromMS(r.UpdatedAt),
	}
}

func insertEvent(ctx context.Context, q *db.Queries, id string, now time.Time, actor, kind string, from, to Status, note string) error {
	if err := q.InsertTaskEvent(ctx, db.InsertTaskEventParams{
		TaskID: id, At: ms(now), Actor: actor, Kind: kind, FromStatus: string(from), ToStatus: string(to), Note: note,
	}); err != nil {
		return fmt.Errorf("tasks: insert event: %w", err)
	}
	return nil
}

// Create adds a task created by c and returns it. Assigning at creation
// mails the assignee.
func (s *Service) Create(ctx context.Context, c mail.Caller, in CreateInput) (Task, error) {
	const op = "create"
	actor, err := actorOf(op, c)
	if err != nil {
		return Task{}, err
	}
	title, err := validateTitle(op, in.Title)
	if err != nil {
		return Task{}, err
	}
	if err := validateBody(op, in.Body); err != nil {
		return Task{}, err
	}
	prio, err := validatePriority(op, in.Priority)
	if err != nil {
		return Task{}, err
	}
	scope, err := normalizeScope(op, in.Scope)
	if err != nil {
		return Task{}, err
	}
	var assignee string
	if strings.TrimSpace(in.Assignee) != "" {
		if assignee, err = s.resolvePerson(ctx, op, in.Assignee); err != nil {
			return Task{}, err
		}
	}
	var row db.Task
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		now := s.now()
		id, err := s.newID(now)
		if err != nil {
			return err
		}
		if in.LinkMessageID != "" {
			if err := checkMessage(ctx, q, op, in.LinkMessageID); err != nil {
				return err
			}
		}
		if err := q.InsertTask(ctx, db.InsertTaskParams{
			ID: id, Title: title, Body: in.Body, Status: string(StatusNotStarted), Priority: string(prio),
			Scope: scope, CreatedBy: actor, Assignee: assignee, DueAt: nullMS(in.DueAt),
			CreatedAt: ms(now), UpdatedAt: ms(now),
		}); err != nil {
			return fmt.Errorf("tasks: %s: insert: %w", op, err)
		}
		if err := insertEvent(ctx, q, id, now, actor, EventCreated, "", StatusNotStarted, ""); err != nil {
			return err
		}
		if assignee != "" {
			if err := insertEvent(ctx, q, id, now, actor, string(ActionAssign), "", "", assignee); err != nil {
				return err
			}
		}
		if in.LinkMessageID != "" {
			if err := q.InsertTaskLink(ctx, db.InsertTaskLinkParams{TaskID: id, MessageID: in.LinkMessageID, LinkedBy: actor, LinkedAt: ms(now)}); err != nil {
				return fmt.Errorf("tasks: %s: link: %w", op, err)
			}
			if err := insertEvent(ctx, q, id, now, actor, string(ActionLink), "", "", in.LinkMessageID); err != nil {
				return err
			}
		}
		row, err = q.GetTask(ctx, id)
		if err != nil {
			return fmt.Errorf("tasks: %s: reload: %w", op, err)
		}
		return nil
	})
	if err != nil {
		return Task{}, err
	}
	s.wake.broadcast()
	t := s.toTask(row, s.now())
	if assignee != "" && assignee != actor {
		s.notifyAssigned(ctx, c, t, "")
	}
	return t, nil
}

// cursor encodes the position after r.
func cursor(r db.Task) string { return strconv.FormatInt(r.UpdatedAt, 10) + "_" + r.ID }

func parseCursor(op, c string) (int64, string, error) {
	ts, id, ok := strings.Cut(c, "_")
	n, err := strconv.ParseInt(ts, 10, 64)
	if !ok || err != nil || n <= 0 || !strings.HasPrefix(id, idPrefix) {
		return 0, "", invalidf(op, "cursor %q is not valid; pass next_cursor from the previous page unchanged", c)
	}
	return n, id, nil
}

// List returns a page of tasks matching in, with Health filled in.
func (s *Service) List(ctx context.Context, in ListInput) (ListResult, error) {
	const op = "list"
	if err := validateStatus(op, in.Status); err != nil {
		return ListResult{}, err
	}
	p := db.ListTasksParams{Status: string(in.Status)}
	var err error
	if p.Assignee, err = normalizePerson(op, "assignee", in.Assignee); err != nil {
		return ListResult{}, err
	}
	if p.Claimant, err = normalizePerson(op, "claimant", in.Claimant); err != nil {
		return ListResult{}, err
	}
	if p.Involves, err = normalizePerson(op, "involves", in.Involves); err != nil {
		return ListResult{}, err
	}
	if p.Scope, err = normalizeScope(op, in.Scope); err != nil {
		return ListResult{}, err
	}
	if p.Scope != "" {
		p.ScopePrefix = p.Scope + "/%"
	}
	if in.OpenOnly {
		p.OpenOnly = 1
	}
	if in.Cursor != "" {
		if p.BeforeUpdated, p.BeforeID, err = parseCursor(op, in.Cursor); err != nil {
			return ListResult{}, err
		}
	}
	limit := in.Limit
	switch {
	case limit <= 0:
		limit = DefaultListLimit
	case limit > MaxListLimit:
		limit = MaxListLimit
	}
	p.Limit = int64(limit + 1)
	rows, err := s.store.Read().ListTasks(ctx, p)
	if err != nil {
		return ListResult{}, fmt.Errorf("tasks: %s: %w", op, err)
	}
	var res ListResult
	if len(rows) > limit {
		rows = rows[:limit]
		res.NextCursor = cursor(rows[len(rows)-1])
	}
	now := s.now()
	res.Items = make([]Task, 0, len(rows))
	for _, r := range rows {
		// LIKE treats _ as a wildcard; confirm the scope prefix exactly.
		if p.Scope != "" && r.Scope != p.Scope && !strings.HasPrefix(r.Scope, p.Scope+"/") {
			continue
		}
		res.Items = append(res.Items, s.toTask(r, now))
	}
	return res, nil
}

// Get returns a task with its history and links.
func (s *Service) Get(ctx context.Context, id string) (Detail, error) {
	const op = "get"
	q := s.store.Read()
	row, err := q.GetTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, notFound(op, id)
	}
	if err != nil {
		return Detail{}, fmt.Errorf("tasks: %s: %w", op, err)
	}
	evs, err := q.ListTaskEvents(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("tasks: %s: events: %w", op, err)
	}
	links, err := q.ListTaskLinks(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("tasks: %s: links: %w", op, err)
	}
	d := Detail{Task: s.toTask(row, s.now()), Events: make([]Event, len(evs)), Links: make([]Link, len(links))}
	for i, e := range evs {
		d.Events[i] = Event{At: fromMS(e.At), Actor: e.Actor, Kind: e.Kind, FromStatus: Status(e.FromStatus), ToStatus: Status(e.ToStatus), Note: e.Note}
	}
	for i, l := range links {
		d.Links[i] = Link{MessageID: l.MessageID, ThreadID: l.ThreadID, Subject: l.Subject, LinkedBy: l.LinkedBy, LinkedAt: fromMS(l.LinkedAt)}
	}
	return d, nil
}

// Counts returns an identity's open work.
func (s *Service) Counts(ctx context.Context, identity string) (Counts, error) {
	const op = "counts"
	who, err := normalizePerson(op, "identity", identity)
	if err != nil || who == "" {
		return Counts{}, err
	}
	q := s.store.Read()
	row, err := q.CountTasksForIdentity(ctx, who)
	if err != nil {
		return Counts{}, fmt.Errorf("tasks: %s: %w", op, err)
	}
	c := Counts{Held: int(row.Held), Assigned: int(row.Assigned)}
	if c.Held == 0 {
		return c, nil
	}
	held, err := q.ListTasks(ctx, db.ListTasksParams{Claimant: who, OpenOnly: 1, Limit: MaxListLimit})
	if err != nil {
		return Counts{}, fmt.Errorf("tasks: %s: held: %w", op, err)
	}
	now := s.now()
	for _, r := range held {
		if h := s.health(r, now); h == HealthStale || h == HealthAbandoned {
			c.Stale++
		}
	}
	return c, nil
}

// ForThreads returns the tasks linked to messages in the given threads.
func (s *Service) ForThreads(ctx context.Context, threadIDs []string) ([]ThreadTask, error) {
	if len(threadIDs) == 0 {
		return []ThreadTask{}, nil
	}
	rows, err := s.store.Read().ListTasksForThreads(ctx, threadIDs)
	if err != nil {
		return nil, fmt.Errorf("tasks: for threads: %w", err)
	}
	out := make([]ThreadTask, len(rows))
	for i, r := range rows {
		out[i] = ThreadTask{ThreadID: r.ThreadID, TaskID: r.ID, Title: r.Title, Status: Status(r.Status)}
	}
	return out, nil
}

// Changed returns a channel closed at the next task change.
func (s *Service) Changed() <-chan struct{} { return s.wake.wait() }

// notifier is an in-process broadcast: waiters grab the current channel and
// each broadcast closes it and installs a fresh one. The zero value is ready.
type notifier struct {
	mu sync.Mutex
	ch chan struct{}
}

func (n *notifier) wait() <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ch == nil {
		n.ch = make(chan struct{})
	}
	return n.ch
}

func (n *notifier) broadcast() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ch != nil {
		close(n.ch)
		n.ch = nil
	}
}
