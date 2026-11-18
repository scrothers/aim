package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store/db"
)

// noticePrefix marks automatic mail AIM sends on an identity's behalf.
const noticePrefix = "[AIM] "

// health derives the lease stage of a held row; empty for other statuses.
func (s *Service) health(r db.Task, now time.Time) Health {
	st := Status(r.Status)
	if st != StatusClaimed && st != StatusStarted {
		return ""
	}
	if !r.LastActivityAt.Valid {
		return HealthActive
	}
	return s.stage(now.Sub(fromMS(r.LastActivityAt.Int64)), r.ClaimSession)
}

// stage maps time since activity to a health stage.
func (s *Service) stage(idle time.Duration, session string) Health {
	d := s.decay
	switch {
	case idle >= d.MaxHold:
		return HealthAbandoned
	case idle >= d.AbandonAfter && !s.sessions.Connected(session):
		return HealthAbandoned
	case idle >= d.StaleAfter:
		return HealthStale
	case idle >= d.IdleAfter:
		return HealthIdle
	default:
		return HealthActive
	}
}

// Touch records activity by an agent session, resetting the decay of every
// task it holds. It is cheap enough to call on every tool call and hook: it
// writes only when a held task's activity is older than the touch
// granularity (a second, or a tenth of IdleAfter when that is shorter).
func (s *Service) Touch(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	now := s.now()
	every := min(touchGranularity, s.decay.IdleAfter/10)
	n, err := s.store.Read().CountTouchableTasks(ctx, db.CountTouchableTasksParams{
		ClaimSession: sessionID, Before: sql.NullInt64{Int64: ms(now.Add(-every)), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("tasks: touch: %w", err)
	}
	if n == 0 {
		return nil
	}
	var rows int64
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		rows, err = q.TouchTasksBySession(ctx, db.TouchTasksBySessionParams{
			Now: sql.NullInt64{Int64: ms(now), Valid: true}, ClaimSession: sessionID,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("tasks: touch: %w", err)
	}
	if rows > 0 {
		s.wake.broadcast()
	}
	return nil
}

// decayed is a decay outcome to mail after commit.
type decayed struct {
	row       db.Task // as it was before the change
	abandoned bool
	idle      time.Duration
}

// Sweep sends stale notices and stores abandonment for held tasks whose
// lease decayed, returning how many tasks it abandoned. The daemon calls it
// every minute.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	now := s.now()
	cands, err := s.store.Read().ListHeldTasks(ctx, sql.NullInt64{Int64: ms(now.Add(-s.decay.StaleAfter)), Valid: true})
	if err != nil {
		return 0, fmt.Errorf("tasks: sweep: %w", err)
	}
	var (
		abandoned int
		errs      []error
	)
	for _, c := range cands {
		out, ok, err := s.decayOne(ctx, c.ID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !ok {
			continue
		}
		if out.abandoned {
			abandoned++
		}
		if err := s.notifyDecay(ctx, out); err != nil {
			errs = append(errs, err)
		}
	}
	return abandoned, errors.Join(errs...)
}

// decayOne re-reads one task in a transaction and applies its stale notice
// or abandonment, reporting whether anything changed.
func (s *Service) decayOne(ctx context.Context, id string) (decayed, bool, error) {
	var out decayed
	changed := false
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.GetTask(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tasks: sweep: load: %w", err)
		}
		now := s.now()
		h := s.health(row, now)
		if h != HealthStale && h != HealthAbandoned {
			return nil
		}
		if h == HealthStale && row.StaleNotified != 0 {
			return nil
		}
		out = decayed{row: row, idle: now.Sub(fromMS(row.LastActivityAt.Int64)).Truncate(time.Second)}
		next := row
		next.UpdatedAt = ms(now)
		var ev db.InsertTaskEventParams
		if h == HealthAbandoned {
			out.abandoned = true
			next.Status = string(StatusAbandoned)
			next.Claimant, next.ClaimSession = "", ""
			next.ClaimedAt, next.LastActivityAt = sql.NullInt64{}, sql.NullInt64{}
			next.StaleNotified = 0
			note := fmt.Sprintf("lease expired after %s without activity; previous claimant %s", out.idle, row.Claimant)
			// Abandoned means anyone may claim, so a reservation lapses too.
			if row.Assignee != "" {
				note += "; reservation for " + row.Assignee + " released"
				next.Assignee = ""
			}
			ev = db.InsertTaskEventParams{Kind: EventDecayed, FromStatus: row.Status, ToStatus: string(StatusAbandoned), Note: note}
		} else {
			next.StaleNotified = 1
			ev = db.InsertTaskEventParams{Kind: EventStale,
				Note: fmt.Sprintf("no activity for %s", out.idle)}
		}
		if err := q.UpdateTask(ctx, db.UpdateTaskParams{
			Title: next.Title, Body: next.Body, Status: next.Status, Priority: next.Priority, Scope: next.Scope,
			Assignee: next.Assignee, Claimant: next.Claimant, ClaimSession: next.ClaimSession, StaleNotified: next.StaleNotified,
			DueAt: next.DueAt, ClaimedAt: next.ClaimedAt, LastActivityAt: next.LastActivityAt, FinishedAt: next.FinishedAt,
			UpdatedAt: next.UpdatedAt, ID: next.ID,
		}); err != nil {
			return fmt.Errorf("tasks: sweep: save: %w", err)
		}
		ev.TaskID, ev.At, ev.Actor = id, ms(now), row.Claimant
		if err := q.InsertTaskEvent(ctx, ev); err != nil {
			return fmt.Errorf("tasks: sweep: event: %w", err)
		}
		changed = true
		return nil
	})
	if err != nil {
		return decayed{}, false, err
	}
	if changed {
		s.wake.broadcast()
	}
	return out, changed, nil
}

// callerFor returns a caller that sends as person, on AIM's behalf.
func callerFor(person, session string) (mail.Caller, error) {
	if person == Operator {
		return mail.OperatorCaller(session, "aim", ""), nil
	}
	a, err := address.Parse(person)
	if err != nil {
		return mail.Caller{}, fmt.Errorf("tasks: notify: sender %q: %w", person, err)
	}
	return mail.Caller{SessionID: session, Identity: a, SignedOn: true, Client: "aim"}, nil
}

// notifyDecay mails the claimant and creator about a stale or abandoned
// lease, from the claimant's identity.
func (s *Service) notifyDecay(ctx context.Context, d decayed) error {
	r := d.row
	from, err := callerFor(r.Claimant, r.ClaimSession)
	if err != nil {
		return err
	}
	to := []string{r.Claimant}
	if r.CreatedBy != r.Claimant {
		to = append(to, r.CreatedBy)
	}
	var subject, what string
	if d.abandoned {
		subject = noticePrefix + "Task abandoned: " + r.Title
		what = fmt.Sprintf("The claim on task %s (%q) lapsed after %s without activity from %s's session, so the task is now abandoned and anyone may claim it again.",
			r.ID, r.Title, d.idle, r.Claimant)
	} else {
		subject = noticePrefix + "Task going stale: " + r.Title
		what = fmt.Sprintf("Task %s (%q), held by %s, has had no activity for %s. Its claim will be abandoned if the session stays inactive; any tool call or task_update from the claimant's session renews it, or release it with task_update action=release if the work stopped.",
			r.ID, r.Title, r.Claimant, d.idle)
	}
	body := what + "\n\nThis message was sent automatically by AIM on " + r.Claimant + "'s behalf; nobody typed it."
	if _, err := s.mail.Send(ctx, from, mail.SendInput{To: to, Subject: truncSubject(subject), Body: body}); err != nil {
		return fmt.Errorf("tasks: notify %s: %w", r.ID, err)
	}
	return nil
}

// notifyAssigned mails the assignee from the assigning caller. Failures are
// logged: the assignment itself is committed.
func (s *Service) notifyAssigned(ctx context.Context, c mail.Caller, t Task, note string) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s assigned you task %s: %s (priority %s).\n\n", actorName(c), t.ID, t.Title, t.Priority)
	if t.Body != "" {
		b.WriteString(t.Body)
		b.WriteString("\n\n")
	}
	if note = strings.TrimSpace(note); note != "" {
		fmt.Fprintf(&b, "Note: %s\n\n", note)
	}
	fmt.Fprintf(&b, "Claim it with task_update (task_id %s, action claim) or start it directly with action start; read aim://task/%s for details.", t.ID, t.ID)
	m, err := s.mail.Send(ctx, c, mail.SendInput{To: []string{t.Assignee}, Subject: truncSubject("Task assigned: " + t.Title), Body: b.String()})
	if err != nil {
		s.log.Warn("tasks: assignment notice failed", "task", t.ID, "assignee", t.Assignee, "err", err)
		return
	}
	// Link the notice so the task history and its thread show it; the
	// assignment stands even if this fails.
	if err := s.Link(ctx, c, t.ID, m.ID); err != nil {
		s.log.Warn("tasks: linking assignment notice failed", "task", t.ID, "message", m.ID, "err", err)
	}
}

func actorName(c mail.Caller) string {
	if c.Operator {
		return Operator
	}
	return c.Identity.IdentityAddress().String()
}

// truncSubject keeps a notice subject within the mail limit.
func truncSubject(s string) string {
	r := []rune(s)
	if len(r) <= mail.MaxSubjectRunes {
		return s
	}
	return string(r[:mail.MaxSubjectRunes-3]) + "..."
}
