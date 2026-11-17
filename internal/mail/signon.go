package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/store/db"
)

// SignonTTL is how long a persisted sign-on stays restorable. A session that
// reconnects within this window of its last sign-on activity is signed back
// on automatically.
const SignonTTL = 24 * time.Hour

// Signon is a persisted sign-on of one session.
type Signon struct {
	SessionID string
	Identity  address.Address
	Away      string
	Client    string
	Machine   string
}

// SaveSignon persists a session's sign-on so it survives a daemon restart. It
// also opportunistically prunes rows older than SignonTTL. An empty session
// id is ignored: anonymous connections cannot be restored.
func (s *Service) SaveSignon(ctx context.Context, so Signon) error {
	if so.SessionID == "" {
		return nil
	}
	now := s.now()
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.UpsertSessionSignon(ctx, db.UpsertSessionSignonParams{
			SessionID: so.SessionID, Identity: so.Identity.String(), Away: so.Away,
			Client: so.Client, Machine: so.Machine, UpdatedAt: now.UnixMilli(),
		}); err != nil {
			return err
		}
		_, err := q.PruneSessionSignons(ctx, now.Add(-SignonTTL).UnixMilli())
		return err
	})
	if err != nil {
		return fmt.Errorf("mail: save signon: %w", err)
	}
	return nil
}

// ClearSignon forgets a session's persisted sign-on (sign-off).
func (s *Service) ClearSignon(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		return q.DeleteSessionSignon(ctx, sessionID)
	})
	if err != nil {
		return fmt.Errorf("mail: clear signon: %w", err)
	}
	return nil
}

// RestoreSignon returns the persisted sign-on for sessionID when one exists
// within SignonTTL and its identity still exists and is not retired. The
// boolean is false when there is nothing to restore.
func (s *Service) RestoreSignon(ctx context.Context, sessionID string) (Signon, bool, error) {
	if sessionID == "" {
		return Signon{}, false, nil
	}
	q := s.store.Read()
	row, err := q.GetSessionSignon(ctx, db.GetSessionSignonParams{
		SessionID: sessionID, MinUpdatedAt: s.now().Add(-SignonTTL).UnixMilli(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Signon{}, false, nil
	}
	if err != nil {
		return Signon{}, false, fmt.Errorf("mail: restore signon: %w", err)
	}
	a, perr := address.Parse(row.Identity)
	if usable := perr == nil && a.Kind() == address.KindIdentity; !usable {
		return Signon{}, false, nil
	}
	// A gone or retired identity simply cannot be restored.
	if _, err := findIdentity(ctx, q, a); err != nil {
		var nf *MailboxNotFoundError
		if errors.Is(err, ErrRetired) || errors.As(err, &nf) {
			return Signon{}, false, nil
		}
		return Signon{}, false, fmt.Errorf("mail: restore signon: %w", err)
	}
	return Signon{SessionID: row.SessionID, Identity: a, Away: row.Away, Client: row.Client, Machine: row.Machine}, true, nil
}
