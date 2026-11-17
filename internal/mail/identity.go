package mail

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/store/db"
)

const (
	maxRoleRunes        = 100
	maxDescriptionRunes = 1000

	// ReportsToOperator is the literal reports_to value for identities that
	// answer to the human operator.
	ReportsToOperator = "operator"
)

// Register creates an identity mailbox. The caller need not be signed on
// (identities must exist before anyone can sign on as them).
func (s *Service) Register(ctx context.Context, _ Caller, in RegisterInput) (Identity, error) {
	const op = "register"
	a, err := parseAddr(op, in.Address)
	if err != nil {
		return Identity{}, err
	}
	if a.Kind() != address.KindIdentity {
		return Identity{}, invalidf(op, "%q is a %s address; register needs an identity address like name@host/org[/repo]", a, a.Kind())
	}
	role := strings.TrimSpace(in.Role)
	if role == "" || utf8.RuneCountInString(role) > maxRoleRunes {
		return Identity{}, invalidf(op, "role is required and must be at most %d characters", maxRoleRunes)
	}
	desc := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(desc) > maxDescriptionRunes {
		return Identity{}, invalidf(op, "description must be at most %d characters", maxDescriptionRunes)
	}
	reports := strings.TrimSpace(in.ReportsTo)
	if reports != "" && reports != ReportsToOperator {
		r, err := parseAddr(op, reports)
		if err != nil {
			return Identity{}, fmt.Errorf("%w (reports_to must be an address or %q)", err, ReportsToOperator)
		}
		reports = r.String()
	}

	now := s.now()
	var out Identity
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetMailboxByKey(ctx, keyOf(a)); err == nil {
			return fmt.Errorf("mail: %s: %w: identity %s is already registered; pick another name or sign on as it", op, ErrExists, a)
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

// ResolveIdentity parses addr and checks that it names a registered identity.
// It is used by sign_on.
func (s *Service) ResolveIdentity(ctx context.Context, addr string) (address.Address, error) {
	const op = "resolve identity"
	a, err := parseAddr(op, addr)
	if err != nil {
		return address.Address{}, err
	}
	if a.Kind() != address.KindIdentity {
		return address.Address{}, invalidf(op, "%q is a %s address; sign on with an identity address like name@host/org[/repo]", a, a.Kind())
	}
	if _, err := findIdentity(ctx, s.store.Read(), a); err != nil {
		return address.Address{}, fmt.Errorf("mail: %s: %w", op, err)
	}
	return a, nil
}

// Roster lists the identities within addr's scope, with presence and the
// subagent names that have mailboxes under each.
func (s *Service) Roster(ctx context.Context, addr string) ([]RosterEntry, error) {
	const op = "roster"
	a, err := parseAddr(op, addr)
	if err != nil {
		return nil, err
	}
	a = a.IdentityAddress()
	sc := a.Scope()
	q := s.store.Read()
	rows, err := q.ListIdentitiesInScope(ctx, db.ListIdentitiesInScopeParams{
		Host: sc.Host, Org: sc.Org,
		MatchRepo: b2i(sc.MatchRepo), Repo: sc.Repo,
		MatchIdentity: b2i(sc.MatchIdentity), Identity: sc.Identity,
	})
	if err != nil {
		return nil, fmt.Errorf("mail: %s: %w", op, err)
	}
	if len(rows) == 0 && a.Kind() == address.KindIdentity {
		return nil, fmt.Errorf("mail: %s: %w", op, notFound(ctx, q, a))
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
			Online: []OnlineStatus{}, Subagents: make([]string, 0, len(subs)),
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

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
