package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/store/db"
)

// MuteThread mutes a thread for the operator: its mail no longer counts as
// unread, is hidden from the operator inbox unless ListInput.IncludeMuted is
// set, and does not trigger notifications. Muting twice is a no-op.
func (s *Service) MuteThread(ctx context.Context, threadID string) error {
	const op = "mute"
	if threadID == "" {
		return invalidf(op, "thread_id is required")
	}
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		ok, err := q.ThreadExists(ctx, threadID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: no thread %q", ErrMessageNotFound, threadID)
		}
		return q.MuteThread(ctx, db.MuteThreadParams{ThreadID: threadID, MutedAt: s.now().UnixMilli()})
	})
	if err != nil {
		return fmt.Errorf("mail: %s: %w", op, err)
	}
	return nil
}

// UnmuteThread reverses MuteThread; unmuting an unmuted thread is a no-op.
func (s *Service) UnmuteThread(ctx context.Context, threadID string) error {
	if threadID == "" {
		return invalidf("unmute", "thread_id is required")
	}
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		return q.UnmuteThread(ctx, threadID)
	})
	if err != nil {
		return fmt.Errorf("mail: unmute: %w", err)
	}
	return nil
}

// MutedThreads lists muted threads, most recently muted first.
func (s *Service) MutedThreads(ctx context.Context) ([]MutedThread, error) {
	rows, err := s.store.Read().ListMutedThreads(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail: muted threads: %w", err)
	}
	out := make([]MutedThread, len(rows))
	for i, r := range rows {
		out[i] = MutedThread{ThreadID: r.ThreadID, MutedAt: fromMS(r.MutedAt)}
	}
	return out, nil
}

// ThreadMuted reports whether a thread is muted.
func (s *Service) ThreadMuted(ctx context.Context, threadID string) (bool, error) {
	ok, err := s.store.Read().IsThreadMuted(ctx, threadID)
	if err != nil {
		return false, fmt.Errorf("mail: thread muted: %w", err)
	}
	return ok, nil
}

func (s *Service) mutedSet(ctx context.Context, q *db.Queries) (map[string]bool, error) {
	rows, err := q.ListMutedThreads(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail: muted threads: %w", err)
	}
	set := make(map[string]bool, len(rows))
	for _, r := range rows {
		set[r.ThreadID] = true
	}
	return set, nil
}

// operatorUnreadMuted counts unread operator deliveries in muted threads.
func operatorUnreadMuted(ctx context.Context, q *db.Queries, mailboxID string) (int, error) {
	n, err := q.CountUnreadInMutedThreads(ctx, mailboxID)
	if err != nil {
		return 0, fmt.Errorf("mail: views: muted unread: %w", err)
	}
	return int(n), nil
}

// isOperatorScope reports whether a list address is the operator mailbox.
func isOperatorScope(addr string) bool {
	a, err := address.Parse(addr)
	return err == nil && a.IsOperator()
}

// listRows fetches up to limit+1 inbox rows, skipping muted threads when
// muted is non-nil. It pages through the store until it has enough rows or
// runs out.
func listRows(ctx context.Context, q *db.Queries, p store.InboxParams, limit int, muted map[string]bool) ([]store.InboxRow, error) {
	p.PageLimit = int64(limit) + 1
	if len(muted) == 0 {
		return store.ListInbox(ctx, q, p)
	}
	var kept []store.InboxRow
	for {
		rows, err := store.ListInbox(ctx, q, p)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if !muted[r.ThreadID] {
				kept = append(kept, r)
			}
		}
		if len(kept) > limit || len(rows) < int(p.PageLimit) {
			return kept[:min(len(kept), limit+1)], nil
		}
		last := rows[len(rows)-1]
		p.BeforeMessage = sql.NullString{String: last.MessageID, Valid: true}
		p.BeforeMailbox = sql.NullString{String: last.MailboxID, Valid: true}
	}
}

// threadMuted reports whether a thread is muted on the given queries.
func threadMuted(ctx context.Context, q *db.Queries, threadID string) (bool, error) {
	ok, err := q.IsThreadMuted(ctx, threadID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("mail: thread muted: %w", err)
	}
	return ok, nil
}
