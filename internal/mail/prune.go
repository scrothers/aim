package mail

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/scrothers/aim/internal/store/db"
)

const pruneBatch = 500

// PruneResult reports what a prune removed (or would remove).
type PruneResult struct {
	// Enabled is false when retention is off and no override was given.
	Enabled bool `json:"enabled"`
	DryRun  bool `json:"dry_run"`
	// Messages is the number of messages deleted, or that would be.
	Messages int `json:"messages"`
	// Cutoff is the age threshold: messages created before it are eligible.
	Cutoff time.Time `json:"cutoff"`
}

// Prune applies the configured retention: it deletes messages older than
// prune_archived_after_days whose every delivery is archived. With dryRun it
// only counts. It does nothing when retention is off.
func (s *Service) Prune(ctx context.Context, now time.Time, dryRun bool) (PruneResult, error) {
	r, err := s.Retention(ctx)
	if err != nil {
		return PruneResult{}, err
	}
	return s.PruneAfter(ctx, now, r.PruneArchivedAfterDays, dryRun)
}

// PruneAfter is Prune with an explicit age in days instead of the stored
// setting; days <= 0 does nothing.
func (s *Service) PruneAfter(ctx context.Context, now time.Time, days int, dryRun bool) (PruneResult, error) {
	if days <= 0 {
		return PruneResult{DryRun: dryRun}, nil
	}
	res := PruneResult{Enabled: true, DryRun: dryRun, Cutoff: now.AddDate(0, 0, -days).UTC()}
	before := res.Cutoff.UnixMilli()
	if dryRun {
		ids, err := s.store.Read().ListPrunableMessages(ctx, db.ListPrunableMessagesParams{BeforeMs: before, RowLimit: math.MaxInt32})
		if err != nil {
			return PruneResult{}, fmt.Errorf("mail: prune: %w", err)
		}
		res.Messages = len(ids)
		return res, nil
	}
	for {
		ids, err := s.store.Read().ListPrunableMessages(ctx, db.ListPrunableMessagesParams{BeforeMs: before, RowLimit: pruneBatch})
		if err != nil {
			return res, fmt.Errorf("mail: prune: %w", err)
		}
		if len(ids) == 0 {
			return res, nil
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return res, fmt.Errorf("mail: prune: %w", err)
			}
			if err := s.pruneOne(ctx, id); err != nil {
				return res, err
			}
			res.Messages++
		}
	}
}

// pruneOne deletes one message; replies to it lose their in_reply_to.
func (s *Service) pruneOne(ctx context.Context, id string) error {
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.NullRepliesTo(ctx, sql.NullString{String: id, Valid: true}); err != nil {
			return err
		}
		if err := q.DeleteMessageDeliveries(ctx, id); err != nil {
			return err
		}
		return q.DeleteMessage(ctx, id)
	})
	if err != nil {
		return fmt.Errorf("mail: prune %s: %w", id, err)
	}
	return nil
}
