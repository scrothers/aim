package mail

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/render"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/store/db"
)

// ID prefixes.
const (
	prefixMailbox = "mbx-"
	prefixMessage = "msg-"
)

const maxSuggestions = 3

const maxSuggestDistance = 3

// Caller identifies the session making a call.
type Caller struct {
	SessionID string
	Identity  address.Address
	SignedOn  bool
	// Operator marks the human operator's session. Only the HTTP layer may
	// construct such a caller (see OperatorCaller); its Identity is the
	// operator address.
	Operator bool
	// Client is the MCP client name (or UI name) and Machine the host the
	// session runs on; both are recorded on sent messages.
	Client  string
	Machine string
}

// OperatorCaller returns the caller for the human operator, who sends only
// as the operator address. It must be constructed only by trusted code (the
// HTTP layer), never from agent input.
func OperatorCaller(sessionID, client, machine string) Caller {
	return Caller{SessionID: sessionID, Identity: address.Operator(), SignedOn: true, Operator: true, Client: client, Machine: machine}
}

// Option configures a Service.
type Option func(*Service)

// WithClock sets the time source (default time.Now).
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithEntropy sets the ULID entropy source (default crypto/rand).
func WithEntropy(r io.Reader) Option {
	return func(s *Service) { s.entropy = ulid.Monotonic(r, 0) }
}

// Service implements the mail rules over a store.
type Service struct {
	store    *store.Store
	presence Presence
	now      func() time.Time
	renderer *render.Renderer
	wake     notifier // broadcast on committed Send and Mark; wakes Wait
	// onOperator is the post-commit hook for operator mail (see hook.go).
	onOperator func(Message)

	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

// New returns a Service. presence may be nil, in which case rosters report
// nobody online.
func New(st *store.Store, p Presence, opts ...Option) *Service {
	s := &Service{store: st, presence: p, now: time.Now, renderer: render.New()}
	WithEntropy(rand.Reader)(s)
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Service) newID(prefix string, t time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := ulid.New(ulid.Timestamp(t), s.entropy)
	if err != nil {
		return "", fmt.Errorf("mail: new id: %w", err)
	}
	return prefix + strings.ToLower(id.String()), nil
}

func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

func nullTime(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromMS(v.Int64)
	return &t
}

func addrOf(m db.Mailbox) address.Address {
	return address.Address{Host: m.Host, Org: m.Org, Repo: m.Repo, Identity: m.Identity, Subagent: m.Subagent}
}

func parseAddr(op, s string) (address.Address, error) {
	a, err := address.Parse(s)
	if err != nil {
		return address.Address{}, invalidAddr(op, err)
	}
	return a, nil
}

func parseAll(op string, in []string) ([]address.Address, error) {
	out := make([]address.Address, 0, len(in))
	for _, s := range in {
		a, err := parseAddr(op, s)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func requireSignedOn(op string, c Caller) error {
	if c.Operator && c.Identity.IsOperator() {
		return nil
	}
	if !c.SignedOn || c.Identity.Kind() != address.KindIdentity {
		return notSignedOn(op)
	}
	return nil
}

func keyOf(a address.Address) db.GetMailboxByKeyParams {
	return db.GetMailboxByKeyParams{Host: a.Host, Org: a.Org, Repo: a.Repo, Identity: a.Identity, Subagent: a.Subagent}
}

// findIdentity returns the active registered identity mailbox for a (or the
// operator mailbox), a *MailboxNotFoundError with suggestions, or an error
// wrapping ErrRetired.
func findIdentity(ctx context.Context, q *db.Queries, a address.Address) (db.Mailbox, error) {
	m, err := findIdentityAny(ctx, q, a)
	if err != nil {
		return db.Mailbox{}, err
	}
	if m.RetiredAt.Valid {
		return db.Mailbox{}, fmt.Errorf("mail: %w: %s was retired; ask the operator to restore it or agent_register a new identity", ErrRetired, addrOf(m))
	}
	return m, nil
}

// findIdentityAny is findIdentity without the retirement check.
func findIdentityAny(ctx context.Context, q *db.Queries, a address.Address) (db.Mailbox, error) {
	a = a.IdentityAddress()
	m, err := q.GetMailboxByKey(ctx, keyOf(a))
	if errors.Is(err, sql.ErrNoRows) {
		return db.Mailbox{}, notFound(ctx, q, a)
	}
	if err != nil {
		return db.Mailbox{}, fmt.Errorf("mail: get mailbox: %w", err)
	}
	return m, nil
}

func notFound(ctx context.Context, q *db.Queries, a address.Address) error {
	e := &MailboxNotFoundError{
		Address: a.String(),
		Hint:    "Identities must be registered first (agent_register); check the address, or agent_register it.",
	}
	cands, err := store.ListIdentitiesInScope(ctx, q, store.Scope{Host: a.Host, Org: a.Org})
	if err != nil {
		return e // suggestions are best effort
	}
	e.Suggestions = suggest(e.Address, cands)
	return e
}

func suggest(target string, cands []db.Mailbox) []string {
	type scored struct {
		addr string
		d    int
	}
	var got []scored
	for _, c := range cands {
		s := addrOf(c).String()
		if d := levenshtein(target, s); d <= maxSuggestDistance {
			got = append(got, scored{s, d})
		}
	}
	slices.SortFunc(got, func(a, b scored) int {
		if a.d != b.d {
			return a.d - b.d
		}
		return strings.Compare(a.addr, b.addr)
	})
	out := make([]string, 0, maxSuggestions)
	for _, g := range got[:min(len(got), maxSuggestions)] {
		out = append(out, g.addr)
	}
	return out
}

// ensureMailbox returns the mailbox for a, creating bare namespace and
// subagent mailboxes on first use. Identities must already exist.
func (s *Service) ensureMailbox(ctx context.Context, q *db.Queries, a address.Address, now time.Time) (db.Mailbox, error) {
	if a.Kind() == address.KindIdentity {
		return findIdentity(ctx, q, a)
	}
	if a.Kind() == address.KindOperator {
		return s.ensureOperator(ctx, q, now)
	}
	if a.Kind() == address.KindSubagent {
		if _, err := findIdentity(ctx, q, a); err != nil {
			return db.Mailbox{}, err
		}
	}
	m, err := q.GetMailboxByKey(ctx, keyOf(a))
	if err == nil {
		return m, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.Mailbox{}, fmt.Errorf("mail: get mailbox: %w", err)
	}
	id, err := s.newID(prefixMailbox, now)
	if err != nil {
		return db.Mailbox{}, err
	}
	m = db.Mailbox{
		ID: id, Host: a.Host, Org: a.Org, Repo: a.Repo, Identity: a.Identity, Subagent: a.Subagent,
		Kind: a.Kind().String(), CreatedAt: now.UnixMilli(),
	}
	err = q.InsertMailbox(ctx, db.InsertMailboxParams{
		ID: m.ID, Host: m.Host, Org: m.Org, Repo: m.Repo, Identity: m.Identity, Subagent: m.Subagent,
		Kind: m.Kind, CreatedAt: m.CreatedAt,
	})
	if err != nil {
		return db.Mailbox{}, fmt.Errorf("mail: insert mailbox: %w", err)
	}
	return m, nil
}

// ensureOperator returns the operator mailbox, creating it on first use.
func (s *Service) ensureOperator(ctx context.Context, q *db.Queries, now time.Time) (db.Mailbox, error) {
	a := address.Operator()
	m, err := q.GetMailboxByKey(ctx, keyOf(a))
	if err == nil {
		return m, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.Mailbox{}, fmt.Errorf("mail: get operator mailbox: %w", err)
	}
	id, err := s.newID(prefixMailbox, now)
	if err != nil {
		return db.Mailbox{}, err
	}
	m = db.Mailbox{ID: id, Identity: a.Identity, Kind: address.KindOperator.String(), CreatedAt: now.UnixMilli()}
	err = q.InsertMailbox(ctx, db.InsertMailboxParams{ID: m.ID, Identity: m.Identity, Kind: m.Kind, CreatedAt: m.CreatedAt})
	if err != nil {
		return db.Mailbox{}, fmt.Errorf("mail: insert operator mailbox: %w", err)
	}
	return m, nil
}

// EnsureOperator creates the reserved operator mailbox if it does not exist.
// The daemon calls it at startup; it is idempotent.
func (s *Service) EnsureOperator(ctx context.Context) error {
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		_, err := s.ensureOperator(ctx, q, s.now())
		return err
	})
	if err != nil {
		return fmt.Errorf("mail: ensure operator: %w", err)
	}
	return nil
}

// callerMailbox returns the mailbox the caller owns: its identity mailbox, or
// the named subagent's mailbox beneath it.
func callerMailbox(ctx context.Context, q *db.Queries, op string, c Caller, subagent string) (db.Mailbox, error) {
	if subagent == "" {
		return findIdentity(ctx, q, c.Identity)
	}
	sub, err := c.Identity.IdentityAddress().WithSubagent(subagent)
	if err != nil {
		return db.Mailbox{}, invalidAddr(op, err)
	}
	if _, err := findIdentity(ctx, q, c.Identity); err != nil {
		return db.Mailbox{}, err
	}
	m, err := q.GetMailboxByKey(ctx, keyOf(sub))
	if errors.Is(err, sql.ErrNoRows) {
		return db.Mailbox{}, &MailboxNotFoundError{
			Address: sub.String(),
			Hint:    "Subagent mailboxes are created the first time that subagent sends or receives mail; check the subagent name.",
		}
	}
	if err != nil {
		return db.Mailbox{}, fmt.Errorf("mail: get subagent mailbox: %w", err)
	}
	return m, nil
}
