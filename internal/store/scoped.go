package store

import (
	"context"
	"database/sql"

	"github.com/scrothers/aim/internal/store/db"
)

// Scope selects mailboxes by address depth. Host and org always apply; each
// Match flag adds the next level's equality, and implies the ones before it.
type Scope struct {
	Host, Org, Repo, Identity, Subagent     string
	MatchRepo, MatchIdentity, MatchSubagent bool
}

// InboxParams filters ListInbox. Pagination is a keyset on (message, mailbox):
// set both BeforeMessage and BeforeMailbox to resume after a delivery.
type InboxParams struct {
	Scope
	FromMailbox   sql.NullString
	Since, Until  sql.NullInt64
	BeforeMessage sql.NullString
	BeforeMailbox sql.NullString
	State         string
	PageLimit     int64
}

// InboxRow is one delivery with its message header and mailbox address.
type InboxRow = db.ListInboxSubagentRow

// ListInbox lists deliveries in scope, newest first, using the per-depth
// query so SQLite can use the full mailbox UNIQUE index.
func ListInbox(ctx context.Context, q *db.Queries, p InboxParams) ([]InboxRow, error) {
	switch {
	case p.MatchSubagent:
		return q.ListInboxSubagent(ctx, db.ListInboxSubagentParams{
			Host: p.Host, Org: p.Org, Repo: p.Repo, Identity: p.Identity, Subagent: p.Subagent,
			FromMailbox: p.FromMailbox, Since: p.Since, Until: p.Until,
			BeforeMessage: p.BeforeMessage, BeforeMailbox: p.BeforeMailbox,
			State: p.State, PageLimit: p.PageLimit,
		})
	case p.MatchIdentity:
		rows, err := q.ListInboxIdentity(ctx, db.ListInboxIdentityParams{
			Host: p.Host, Org: p.Org, Repo: p.Repo, Identity: p.Identity,
			FromMailbox: p.FromMailbox, Since: p.Since, Until: p.Until,
			BeforeMessage: p.BeforeMessage, BeforeMailbox: p.BeforeMailbox,
			State: p.State, PageLimit: p.PageLimit,
		})
		return convertRows[db.ListInboxIdentityRow](rows, err)
	case p.MatchRepo:
		rows, err := q.ListInboxRepo(ctx, db.ListInboxRepoParams{
			Host: p.Host, Org: p.Org, Repo: p.Repo,
			FromMailbox: p.FromMailbox, Since: p.Since, Until: p.Until,
			BeforeMessage: p.BeforeMessage, BeforeMailbox: p.BeforeMailbox,
			State: p.State, PageLimit: p.PageLimit,
		})
		return convertRows[db.ListInboxRepoRow](rows, err)
	default:
		rows, err := q.ListInboxOrg(ctx, db.ListInboxOrgParams{
			Host: p.Host, Org: p.Org,
			FromMailbox: p.FromMailbox, Since: p.Since, Until: p.Until,
			BeforeMessage: p.BeforeMessage, BeforeMailbox: p.BeforeMailbox,
			State: p.State, PageLimit: p.PageLimit,
		})
		return convertRows[db.ListInboxOrgRow](rows, err)
	}
}

// convertRows maps the per-depth row types, which share one field layout,
// onto InboxRow.
func convertRows[T db.ListInboxOrgRow | db.ListInboxRepoRow | db.ListInboxIdentityRow](rows []T, err error) ([]InboxRow, error) {
	if err != nil {
		return nil, err
	}
	out := make([]InboxRow, len(rows))
	for i, r := range rows {
		out[i] = InboxRow(r)
	}
	return out, nil
}

// ListIdentitiesInScope lists identity mailboxes in scope (the subagent flag
// is ignored), ordered by repo then identity.
func ListIdentitiesInScope(ctx context.Context, q *db.Queries, s Scope) ([]db.Mailbox, error) {
	switch {
	case s.MatchIdentity:
		return q.ListIdentitiesInIdentity(ctx, db.ListIdentitiesInIdentityParams{
			Host: s.Host, Org: s.Org, Repo: s.Repo, Identity: s.Identity})
	case s.MatchRepo:
		return q.ListIdentitiesInRepo(ctx, db.ListIdentitiesInRepoParams{Host: s.Host, Org: s.Org, Repo: s.Repo})
	default:
		return q.ListIdentitiesInOrg(ctx, db.ListIdentitiesInOrgParams{Host: s.Host, Org: s.Org})
	}
}
