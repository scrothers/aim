package completions

import (
	"context"
	"slices"
	"strings"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
)

// recentLimit bounds the mail scanned for id completions.
const recentLimit = mail.MaxListLimit

// scope is the address whose roster feeds suggestions: the caller's org when
// its namespace is known, otherwise everything.
func (cp completer) scope() string {
	parts := strings.Split(cp.c.Entry.Preamble().Namespace, "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[:2], "/")
}

// roster lists identities in scope, online ones first.
func (cp completer) roster(ctx context.Context) ([]mail.RosterEntry, error) {
	es, err := cp.c.Svc.Roster(ctx, cp.scope())
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(es, func(a, b mail.RosterEntry) int {
		return onlineRank(a) - onlineRank(b)
	})
	return es, nil
}

func onlineRank(e mail.RosterEntry) int {
	if len(e.Online) > 0 {
		return 0
	}
	return 1
}

// identities suggests identity addresses, optionally with operator.
func (cp completer) identities(ctx context.Context, withOperator bool) ([]string, error) {
	es, err := cp.roster(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(es)+1)
	for _, e := range es {
		out = append(out, e.Address)
	}
	if withOperator {
		out = append(out, address.Operator().String())
	}
	return out, nil
}

// addresses suggests scopes (the caller's namespace, orgs, repos) and
// identities for address arguments.
func (cp completer) addresses(ctx context.Context, withOperator bool) ([]string, error) {
	ids, err := cp.identities(ctx, withOperator)
	if err != nil {
		return nil, err
	}
	var scopes []string
	add := func(s string) {
		if s != "" && !slices.Contains(scopes, s) {
			scopes = append(scopes, s)
		}
	}
	add(cp.c.Entry.Preamble().Namespace)
	add(cp.scope())
	for _, id := range ids {
		if _, scope, ok := strings.Cut(id, "@"); ok {
			add(scope)
			if i := strings.LastIndex(scope, "/"); i > strings.Index(scope, "/") {
				add(scope[:i])
			}
		}
	}
	return append(scopes, ids...), nil
}

// recentIDs extracts values from the caller's recent non-archived mail (their
// identity when signed on, else their namespace), newest first, deduplicated.
func (cp completer) recentIDs(ctx context.Context, pick func(mail.Header) string) ([]string, error) {
	addr := cp.c.Entry.Preamble().Namespace
	if id, ok := cp.c.Entry.Identity(); ok {
		addr = id.String()
	}
	if addr == "" {
		return nil, nil
	}
	res, err := cp.c.Svc.List(ctx, mail.ListInput{Address: addr, State: mail.StateInbox, Limit: recentLimit})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range res.Items {
		if v := pick(m); !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out, nil
}
