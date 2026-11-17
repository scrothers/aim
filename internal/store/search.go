package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrBadQuery is returned when the full-text query is empty or is not valid
// FTS5 syntax.
var ErrBadQuery = errors.New("store: invalid search query")

// SearchParams selects messages by full-text query within an address scope.
// The Match* flags mirror the address scope depth: host and org always apply.
type SearchParams struct {
	Host, Org, Repo, Identity, Subagent     string
	MatchRepo, MatchIdentity, MatchSubagent bool
	Query                                   string
	Limit, Offset                           int
	Since, Until                            *int64
}

// SearchHit is one ranked search result. Rank is the bm25 score (lower is
// better). A message delivered to several in-scope mailboxes yields one hit,
// reported against the lowest mailbox ID.
type SearchHit struct {
	MessageID     string
	ThreadID      string
	Subject       string
	Snippet       string
	FromMailboxID string
	CreatedAt     int64
	Rank          float64
	MailboxID     string
}

const scopeClause = `mb.host = @host AND mb.org = @org
  AND (@match_repo = 0 OR mb.repo = @repo)
  AND (@match_identity = 0 OR mb.identity = @identity)
  AND (@match_subagent = 0 OR mb.subagent = @subagent)`

const searchSQL = `
SELECT m.id, m.thread_id, m.subject,
       snippet(messages_fts, 1, '[', ']', '...', 12),
       m.from_mailbox, m.created_at, bm25(messages_fts), d.mailbox_id
FROM messages_fts
JOIN messages m ON m.rowid = messages_fts.rowid
JOIN deliveries d ON d.message_id = m.id
JOIN mailboxes mb ON mb.id = d.mailbox_id
WHERE messages_fts MATCH @query
  AND ` + scopeClause + `
  AND (@since IS NULL OR m.created_at >= @since)
  AND (@until IS NULL OR m.created_at <= @until)
  AND NOT EXISTS (
    SELECT 1 FROM deliveries d2 JOIN mailboxes mb2 ON mb2.id = d2.mailbox_id
    WHERE d2.message_id = d.message_id AND d2.mailbox_id < d.mailbox_id
      AND mb2.host = @host AND mb2.org = @org
      AND (@match_repo = 0 OR mb2.repo = @repo)
      AND (@match_identity = 0 OR mb2.identity = @identity)
      AND (@match_subagent = 0 OR mb2.subagent = @subagent))
ORDER BY bm25(messages_fts), m.id DESC
LIMIT @limit OFFSET @offset`

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullInt(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

// isFTSSyntax reports whether err came from FTS5 rejecting the query text.
func isFTSSyntax(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "fts5") ||
		strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "unterminated string") ||
		strings.Contains(msg, "malformed MATCH")
}

// Search runs a bm25-ranked FTS5 query over messages delivered inside the
// scope. Query text is passed as a bound parameter, so the only failure agent
// input can cause is ErrBadQuery.
func (s *Store) Search(ctx context.Context, p SearchParams) ([]SearchHit, error) {
	if strings.TrimSpace(p.Query) == "" {
		return nil, fmt.Errorf("store: search: empty query: %w", ErrBadQuery)
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.reader.QueryContext(ctx, searchSQL,
		sql.Named("query", p.Query),
		sql.Named("host", p.Host), sql.Named("org", p.Org),
		sql.Named("repo", p.Repo), sql.Named("identity", p.Identity), sql.Named("subagent", p.Subagent),
		sql.Named("match_repo", b2i(p.MatchRepo)),
		sql.Named("match_identity", b2i(p.MatchIdentity)),
		sql.Named("match_subagent", b2i(p.MatchSubagent)),
		sql.Named("since", nullInt(p.Since)), sql.Named("until", nullInt(p.Until)),
		sql.Named("limit", limit), sql.Named("offset", max(p.Offset, 0)),
	)
	if err != nil {
		return nil, searchErr(err)
	}
	defer func() { _ = rows.Close() }()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.MessageID, &h.ThreadID, &h.Subject, &h.Snippet,
			&h.FromMailboxID, &h.CreatedAt, &h.Rank, &h.MailboxID); err != nil {
			return nil, fmt.Errorf("store: search: scan: %w", err)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, searchErr(err)
	}
	return hits, nil
}

func searchErr(err error) error {
	if isFTSSyntax(err) {
		return fmt.Errorf("store: search: %w: %w", ErrBadQuery, err)
	}
	return fmt.Errorf("store: search: %w", err)
}
