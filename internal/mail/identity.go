package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/store/db"
)

const (
	maxRoleRunes        = 100
	maxDescriptionRunes = 1000

	// maxReportsDepth bounds the reports_to chain walk of the cycle check.
	maxReportsDepth = 64

	// ReportsToOperator is the literal reports_to value for identities that
	// answer to the human operator; it is the operator address.
	ReportsToOperator = address.OperatorName
)

func reservedErr(op string) error {
	return fmt.Errorf("mail: %s: %w: operator is reserved for the human operator; agents escalate by sending to operator", op, ErrReserved)
}

func validateRole(op, role string) (string, error) {
	role = strings.TrimSpace(role)
	if role == "" || utf8.RuneCountInString(role) > maxRoleRunes {
		return "", invalidf(op, "role is required and must be at most %d characters", maxRoleRunes)
	}
	return role, nil
}

func validateDescription(op, desc string) (string, error) {
	desc = strings.TrimSpace(desc)
	if utf8.RuneCountInString(desc) > maxDescriptionRunes {
		return "", invalidf(op, "description must be at most %d characters", maxDescriptionRunes)
	}
	return desc, nil
}

// normalizeReportsTo canonicalizes a reports_to value: empty, the operator,
// or an address.
func normalizeReportsTo(op, reports string) (string, error) {
	reports = strings.TrimSpace(reports)
	if reports == "" {
		return "", nil
	}
	r, err := parseAddr(op, reports)
	if err != nil {
		return "", fmt.Errorf("%w (reports_to must be an address or %q)", err, ReportsToOperator)
	}
	return r.String(), nil
}

// checkReportsCycle walks the reports_to chain starting at reports and fails
// when it leads back to self or is deeper than maxReportsDepth.
func checkReportsCycle(ctx context.Context, q *db.Queries, op string, self address.Address, reports string) error {
	cur := reports
	for range maxReportsDepth {
		if cur == "" || cur == ReportsToOperator {
			return nil
		}
		a, perr := address.Parse(cur)
		if perr != nil || a.Kind() != address.KindIdentity {
			return nil //nolint:nilerr // unparseable or namespace targets simply end the chain
		}
		if a == self {
			return fmt.Errorf("mail: %s: %w: %w: %s would report to itself through %s", op, ErrInvalidInput, ErrCycle, self, reports)
		}
		m, err := q.GetMailboxByKey(ctx, keyOf(a))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("mail: %s: reports_to chain: %w", op, err)
		}
		cur = m.ReportsTo
	}
	return fmt.Errorf("mail: %s: %w: %w: reports_to chain from %s is deeper than %d", op, ErrInvalidInput, ErrCycle, reports, maxReportsDepth)
}

func identityOf(m db.Mailbox) Identity {
	return Identity{
		Address: addrOf(m).String(), Role: m.Role, Description: m.Description, ReportsTo: m.ReportsTo,
		CreatedAt: fromMS(m.CreatedAt), RetiredAt: nullTime(m.RetiredAt),
	}
}

// Register creates an identity mailbox. The caller need not be signed on
// (identities must exist before anyone can sign on as them).
func (s *Service) Register(ctx context.Context, _ Caller, in RegisterInput) (Identity, error) {
	const op = "register"
	a, err := parseAddr(op, in.Address)
	if err != nil {
		return Identity{}, err
	}
	if a.Kind() == address.KindOperator {
		return Identity{}, reservedErr(op)
	}
	if a.Kind() != address.KindIdentity {
		return Identity{}, invalidf(op, "%q is a %s address; agent_register needs an identity address like name@host/org[/repo]", a, a.Kind())
	}
	role, err := validateRole(op, in.Role)
	if err != nil {
		return Identity{}, err
	}
	desc, err := validateDescription(op, in.Description)
	if err != nil {
		return Identity{}, err
	}
	reports, err := normalizeReportsTo(op, in.ReportsTo)
	if err != nil {
		return Identity{}, err
	}

	now := s.now()
	var out Identity
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetMailboxByKey(ctx, keyOf(a)); err == nil {
			return fmt.Errorf("mail: %s: %w: identity %s is already registered; pick another name or sign on as it", op, ErrExists, a)
		}
		if err := checkReportsCycle(ctx, q, op, a, reports); err != nil {
			return err
		}
		id, err := s.newID(prefixMailbox, now)
		if err != nil {
			return err
		}
		err = q.InsertMailbox(ctx, db.InsertMailboxParams{
			ID: id, Host: a.Host, Org: a.Org, Repo: a.Repo, Identity: a.Identity,
			Kind: address.KindIdentity.String(), Role: role, Description: desc, ReportsTo: reports,
			CreatedAt: now.UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("mail: %s: insert: %w", op, err)
		}
		out = Identity{Address: a.String(), Role: role, Description: desc, ReportsTo: reports, CreatedAt: fromMS(now.UnixMilli())}
		return nil
	})
	if err != nil {
		return Identity{}, err
	}
	return out, nil
}

// identityTarget parses addr and requires an identity address.
func identityTarget(op, addr string) (address.Address, error) {
	a, err := parseAddr(op, addr)
	if err != nil {
		return address.Address{}, err
	}
	if a.Kind() == address.KindOperator {
		return address.Address{}, reservedErr(op)
	}
	if a.Kind() != address.KindIdentity {
		return address.Address{}, invalidf(op, "%q is a %s address; %s needs an identity address like name@host/org[/repo]", a, a.Kind(), op)
	}
	return a, nil
}

// UpdateIdentity changes the role, description or reports_to of an identity;
// nil fields are left unchanged. The reports_to chain must not loop. Retired
// identities may be edited.
func (s *Service) UpdateIdentity(ctx context.Context, addr string, in UpdateInput) (Identity, error) {
	const op = "update identity"
	a, err := identityTarget(op, addr)
	if err != nil {
		return Identity{}, err
	}
	var role, desc, reports *string
	if in.Role != nil {
		v, err := validateRole(op, *in.Role)
		if err != nil {
			return Identity{}, err
		}
		role = &v
	}
	if in.Description != nil {
		v, err := validateDescription(op, *in.Description)
		if err != nil {
			return Identity{}, err
		}
		desc = &v
	}
	if in.ReportsTo != nil {
		v, err := normalizeReportsTo(op, *in.ReportsTo)
		if err != nil {
			return Identity{}, err
		}
		reports = &v
	}
	var out Identity
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		m, err := findIdentityAny(ctx, q, a)
		if err != nil {
			return fmt.Errorf("mail: %s: %w", op, err)
		}
		p := db.UpdateIdentityParams{ID: m.ID, Role: m.Role, Description: m.Description, ReportsTo: m.ReportsTo}
		if role != nil {
			p.Role = *role
		}
		if desc != nil {
			p.Description = *desc
		}
		if reports != nil {
			p.ReportsTo = *reports
			if err := checkReportsCycle(ctx, q, op, a, p.ReportsTo); err != nil {
				return err
			}
		}
		if _, err := q.UpdateIdentity(ctx, p); err != nil {
			return fmt.Errorf("mail: %s: %w", op, err)
		}
		m.Role, m.Description, m.ReportsTo = p.Role, p.Description, p.ReportsTo
		out = identityOf(m)
		return nil
	})
	if err != nil {
		return Identity{}, err
	}
	return out, nil
}

// Retire marks an identity retired: it can no longer sign on, send, or
// receive mail, and the roster hides it unless asked. Retiring an already
// retired identity keeps its original retirement time.
func (s *Service) Retire(ctx context.Context, addr string) (Identity, error) {
	return s.setRetired(ctx, "retire", addr, true)
}

// Unretire restores a retired identity. It is a no-op for active ones.
func (s *Service) Unretire(ctx context.Context, addr string) (Identity, error) {
	return s.setRetired(ctx, "unretire", addr, false)
}

func (s *Service) setRetired(ctx context.Context, op, addr string, retire bool) (Identity, error) {
	a, err := identityTarget(op, addr)
	if err != nil {
		return Identity{}, err
	}
	var out Identity
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		m, err := findIdentityAny(ctx, q, a)
		if err != nil {
			return fmt.Errorf("mail: %s: %w", op, err)
		}
		if retire != m.RetiredAt.Valid {
			at := sql.NullInt64{}
			if retire {
				at = sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true}
			}
			if _, err := q.SetRetiredAt(ctx, db.SetRetiredAtParams{ID: m.ID, RetiredAt: at}); err != nil {
				return fmt.Errorf("mail: %s: %w", op, err)
			}
			m.RetiredAt = at
		}
		out = identityOf(m)
		return nil
	})
	if err != nil {
		return Identity{}, err
	}
	return out, nil
}

// ResolveIdentity parses addr and checks that it names a registered, active
// identity. It is used by sign_on; the operator address is reserved.
func (s *Service) ResolveIdentity(ctx context.Context, addr string) (address.Address, error) {
	const op = "resolve identity"
	a, err := parseAddr(op, addr)
	if err != nil {
		return address.Address{}, err
	}
	if a.Kind() == address.KindOperator {
		return address.Address{}, reservedErr(op)
	}
	if a.Kind() != address.KindIdentity {
		return address.Address{}, invalidf(op, "%q is a %s address; sign on with an identity address like name@host/org[/repo]", a, a.Kind())
	}
	if _, err := findIdentity(ctx, s.store.Read(), a); err != nil {
		return address.Address{}, fmt.Errorf("mail: %s: %w", op, err)
	}
	return a, nil
}

// RosterOptions tunes RosterWith.
type RosterOptions struct {
	// IncludeRetired keeps retired identities in the listing.
	IncludeRetired bool
}

// Roster lists the active identities within addr's scope, with presence and
// the subagent names that have mailboxes under each. An empty addr lists
// every identity.
func (s *Service) Roster(ctx context.Context, addr string) ([]RosterEntry, error) {
	return s.RosterWith(ctx, addr, RosterOptions{})
}

// RosterWith is Roster with options.
func (s *Service) RosterWith(ctx context.Context, addr string, opts RosterOptions) ([]RosterEntry, error) {
	const op = "roster"
	q := s.store.Read()
	var rows []db.Mailbox
	var err error
	var target address.Address
	if strings.TrimSpace(addr) == "" {
		rows, err = q.ListAllIdentities(ctx)
		if err == nil && !opts.IncludeRetired {
			rows = activeOnly(rows)
		}
	} else {
		if target, err = parseAddr(op, addr); err != nil {
			return nil, err
		}
		if target.Kind() == address.KindOperator {
			return nil, reservedErr(op)
		}
		target = target.IdentityAddress()
		sc := toStoreScope(target.Scope())
		sc.IncludeRetired = opts.IncludeRetired
		rows, err = store.ListIdentitiesInScope(ctx, q, sc)
	}
	if err != nil {
		return nil, fmt.Errorf("mail: %s: %w", op, err)
	}
	if len(rows) == 0 && target.Kind() == address.KindIdentity {
		return nil, fmt.Errorf("mail: %s: %w", op, notFound(ctx, q, target))
	}
	out := make([]RosterEntry, 0, len(rows))
	for _, r := range rows {
		ia := addrOf(r)
		subs, err := q.ListSubagentsOf(ctx, db.ListSubagentsOfParams{Host: r.Host, Org: r.Org, Repo: r.Repo, Identity: r.Identity})
		if err != nil {
			return nil, fmt.Errorf("mail: %s: subagents: %w", op, err)
		}
		e := RosterEntry{
			Address: ia.String(), Role: r.Role, Description: r.Description, ReportsTo: r.ReportsTo,
			Online: []OnlineStatus{}, Subagents: make([]string, 0, len(subs)), RetiredAt: nullTime(r.RetiredAt),
		}
		if s.presence != nil {
			if on := s.presence.Online(ia); on != nil {
				e.Online = on
			}
		}
		for _, sub := range subs {
			e.Subagents = append(e.Subagents, sub.Subagent)
		}
		out = append(out, e)
	}
	return out, nil
}

func activeOnly(rows []db.Mailbox) []db.Mailbox {
	out := make([]db.Mailbox, 0, len(rows))
	for _, r := range rows {
		if !r.RetiredAt.Valid {
			out = append(out, r)
		}
	}
	return out
}

func toStoreScope(sc address.Scope) store.Scope {
	return store.Scope{
		Host: sc.Host, Org: sc.Org, Repo: sc.Repo, Identity: sc.Identity, Subagent: sc.Subagent,
		MatchRepo: sc.MatchRepo, MatchIdentity: sc.MatchIdentity, MatchSubagent: sc.MatchSubagent,
	}
}
