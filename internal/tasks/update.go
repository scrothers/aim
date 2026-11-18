package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store/db"
)

// change is one action being applied to a loaded row.
type change struct {
	op     string
	actor  string
	caller mail.Caller
	in     UpdateInput
	now    time.Time
	row    db.Task // mutated in place
	from   Status
	note   string // event note; defaults to in.Note
	// assigned is set when the action assigned someone, for notification.
	assigned string
	// linked is the message to link, if any.
	linked string
}

func (ch *change) is(who string) bool { return who != "" && ch.actor == who }

func (ch *change) forbidden(format string, args ...any) error {
	return fmt.Errorf("tasks: %s: %w: %s", ch.op, ErrForbidden, fmt.Sprintf(format, args...))
}

func (ch *change) transition(format string, args ...any) error {
	return fmt.Errorf("tasks: %s: %w: %s", ch.op, ErrTransition, fmt.Sprintf(format, args...))
}

func (ch *change) status() Status { return Status(ch.row.Status) }

// requireOne allows the operator or any of the listed people.
func (ch *change) requireOne(what string, who ...string) error {
	if ch.caller.Operator {
		return nil
	}
	for _, w := range who {
		if ch.is(w) {
			return nil
		}
	}
	return ch.forbidden("only %s may %s task %s", what, ch.in.Action, ch.row.ID)
}

func (ch *change) setStatus(st Status) { ch.row.Status = string(st) }

// claim records the caller as the claimant of the row.
func (ch *change) claim() {
	now := sql.NullInt64{Int64: ms(ch.now), Valid: true}
	ch.row.Claimant = ch.actor
	ch.row.ClaimSession = ch.caller.SessionID
	ch.row.ClaimedAt = now
	ch.row.LastActivityAt = now
	ch.row.StaleNotified = 0
}

// clearClaim removes the claimant and lease.
func (ch *change) clearClaim() {
	ch.row.Claimant = ""
	ch.row.ClaimSession = ""
	ch.row.ClaimedAt = sql.NullInt64{}
	ch.row.LastActivityAt = sql.NullInt64{}
	ch.row.StaleNotified = 0
}

// checkClaimable enforces assignment reservation for claim and start.
func (ch *change) checkClaimable() error {
	if a := ch.row.Assignee; a != "" && !ch.is(a) && !ch.caller.Operator {
		return ch.forbidden("task %s is assigned to %s; only the assignee may claim it (ask them, or have the creator reassign it)", ch.row.ID, a)
	}
	return nil
}

func (ch *change) apply(ctx context.Context, s *Service, q *db.Queries) error {
	switch ch.in.Action {
	case ActionClaim:
		return ch.doClaim()
	case ActionStart:
		return ch.doStart()
	case ActionFinish:
		return ch.doFinish()
	case ActionRelease:
		return ch.doRelease()
	case ActionAssign:
		return ch.doAssign(ctx, s)
	case ActionUnassign:
		return ch.doUnassign()
	case ActionReopen:
		return ch.doReopen()
	case ActionComment:
		if strings.TrimSpace(ch.in.Note) == "" {
			return invalidf(ch.op, "comment needs a note with the comment text")
		}
		return nil
	case ActionLink:
		return ch.doLink(ctx, q)
	case ActionEdit:
		return ch.doEdit()
	default:
		return invalidf(ch.op, "action %q is not one of claim, start, finish, release, assign, unassign, reopen, comment, link, edit", ch.in.Action)
	}
}

func (ch *change) doClaim() error {
	switch st := ch.status(); st {
	case StatusNotStarted, StatusAbandoned:
		if err := ch.checkClaimable(); err != nil {
			return err
		}
		ch.claim()
		ch.setStatus(StatusClaimed)
		return nil
	case StatusClaimed, StatusStarted:
		if ch.is(ch.row.Claimant) {
			// Re-claiming your own task moves the lease to this session, for
			// an agent that reconnected.
			ch.claim()
			return nil
		}
		return ch.transition("task %s is %s by %s; wait for them to release it or for the lease to decay", ch.row.ID, st, ch.row.Claimant)
	default:
		return ch.transition("task %s is finished; the creator can reopen it first", ch.row.ID)
	}
}

func (ch *change) doStart() error {
	switch st := ch.status(); st {
	case StatusNotStarted, StatusAbandoned:
		if err := ch.checkClaimable(); err != nil {
			return err
		}
		ch.claim()
	case StatusClaimed:
		if !ch.is(ch.row.Claimant) && !ch.caller.Operator {
			return ch.transition("task %s is claimed by %s; only the claimant may start it", ch.row.ID, ch.row.Claimant)
		}
	case StatusStarted:
		return ch.transition("task %s is already started by %s", ch.row.ID, ch.row.Claimant)
	default:
		return ch.transition("task %s is finished; the creator can reopen it first", ch.row.ID)
	}
	ch.setStatus(StatusStarted)
	return nil
}

func (ch *change) doFinish() error {
	st := ch.status()
	if st != StatusClaimed && st != StatusStarted {
		return ch.transition("task %s is %s; only a claimed or started task can be finished (claim it first)", ch.row.ID, st)
	}
	if err := ch.requireOne("the claimant ("+ch.row.Claimant+")", ch.row.Claimant); err != nil {
		return err
	}
	ch.clearClaim()
	ch.row.FinishedAt = sql.NullInt64{Int64: ms(ch.now), Valid: true}
	ch.setStatus(StatusFinished)
	return nil
}

func (ch *change) doRelease() error {
	st := ch.status()
	if st != StatusClaimed && st != StatusStarted {
		return ch.transition("task %s is %s; only a claimed or started task can be released", ch.row.ID, st)
	}
	if err := ch.requireOne("the claimant or the creator", ch.row.Claimant, ch.row.CreatedBy); err != nil {
		return err
	}
	ch.clearClaim()
	ch.setStatus(StatusNotStarted)
	return nil
}

func (ch *change) doAssign(ctx context.Context, s *Service) error {
	if ch.status() == StatusFinished {
		return ch.transition("task %s is finished; reopen it before assigning", ch.row.ID)
	}
	if strings.TrimSpace(ch.in.Assignee) == "" {
		return invalidf(ch.op, "assign needs assignee, an identity address from roster (or operator)")
	}
	if err := ch.requireOne("the creator or the current assignee", ch.row.CreatedBy, ch.row.Assignee); err != nil {
		return err
	}
	who, err := s.resolvePerson(ctx, ch.op, ch.in.Assignee)
	if err != nil {
		return err
	}
	if who == ch.row.Assignee {
		return ch.transition("task %s is already assigned to %s", ch.row.ID, who)
	}
	ch.row.Assignee = who
	ch.assigned = who
	ch.note = joinNote(who, ch.in.Note)
	return nil
}

func (ch *change) doUnassign() error {
	if ch.row.Assignee == "" {
		return ch.transition("task %s is not assigned", ch.row.ID)
	}
	if err := ch.requireOne("the creator or the current assignee", ch.row.CreatedBy, ch.row.Assignee); err != nil {
		return err
	}
	ch.note = joinNote(ch.row.Assignee, ch.in.Note)
	ch.row.Assignee = ""
	return nil
}

func (ch *change) doReopen() error {
	st := ch.status()
	if st != StatusFinished && st != StatusAbandoned {
		return ch.transition("task %s is %s; only finished or abandoned tasks can be reopened", ch.row.ID, st)
	}
	if err := ch.requireOne("the creator ("+ch.row.CreatedBy+")", ch.row.CreatedBy); err != nil {
		return err
	}
	ch.clearClaim()
	ch.row.FinishedAt = sql.NullInt64{}
	ch.setStatus(StatusNotStarted)
	return nil
}

func (ch *change) doLink(ctx context.Context, q *db.Queries) error {
	id := strings.TrimSpace(ch.in.MessageID)
	if id == "" {
		return invalidf(ch.op, "link needs message_id, a msg-... id from list or read")
	}
	if err := checkMessage(ctx, q, ch.op, id); err != nil {
		return err
	}
	ch.linked = id
	ch.note = joinNote(id, ch.in.Note)
	return nil
}

func (ch *change) doEdit() error {
	if err := ch.requireOne("the creator or the claimant", ch.row.CreatedBy, ch.row.Claimant); err != nil {
		return err
	}
	in := ch.in
	if in.Title == nil && in.Body == nil && in.Priority == nil && in.Scope == nil && in.DueAt == nil && !in.ClearDue {
		return invalidf(ch.op, "edit needs at least one of title, body, priority, scope, due_at or clear_due")
	}
	var changed []string
	if in.Title != nil {
		t, err := validateTitle(ch.op, *in.Title)
		if err != nil {
			return err
		}
		ch.row.Title = t
		changed = append(changed, "title")
	}
	if in.Body != nil {
		if err := validateBody(ch.op, *in.Body); err != nil {
			return err
		}
		ch.row.Body = *in.Body
		changed = append(changed, "body")
	}
	if in.Priority != nil {
		p, err := validatePriority(ch.op, *in.Priority)
		if err != nil {
			return err
		}
		ch.row.Priority = string(p)
		changed = append(changed, "priority")
	}
	if in.Scope != nil {
		sc, err := normalizeScope(ch.op, *in.Scope)
		if err != nil {
			return err
		}
		ch.row.Scope = sc
		changed = append(changed, "scope")
	}
	switch {
	case in.ClearDue:
		ch.row.DueAt = sql.NullInt64{}
		changed = append(changed, "due_at")
	case in.DueAt != nil:
		ch.row.DueAt = nullMS(in.DueAt)
		changed = append(changed, "due_at")
	}
	ch.note = joinNote("changed "+strings.Join(changed, ", "), ch.in.Note)
	return nil
}

// joinNote prefixes a reason with what the action recorded.
func joinNote(what, reason string) string {
	if reason = strings.TrimSpace(reason); reason == "" {
		return what
	}
	return what + ": " + reason
}

// Update applies one action for c and returns the task afterwards. It counts
// as activity when c's session holds the task.
func (s *Service) Update(ctx context.Context, c mail.Caller, in UpdateInput) (Task, error) {
	op := "update"
	if in.Action != "" {
		op = string(in.Action)
	}
	actor, err := actorOf(op, c)
	if err != nil {
		return Task{}, err
	}
	if len(in.Note) > MaxNoteLen {
		return Task{}, invalidf(op, "note is %d bytes; the limit is %d", len(in.Note), MaxNoteLen)
	}
	var ch *change
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		row, err := q.GetTask(ctx, in.TaskID)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound(op, in.TaskID)
		}
		if err != nil {
			return fmt.Errorf("tasks: %s: load: %w", op, err)
		}
		ch = &change{op: op, actor: actor, caller: c, in: in, now: s.now(), row: row, from: Status(row.Status), note: in.Note}
		if err := ch.apply(ctx, s, q); err != nil {
			return err
		}
		return s.save(ctx, q, ch)
	})
	if err != nil {
		return Task{}, err
	}
	s.wake.broadcast()
	t := s.toTask(ch.row, s.now())
	if ch.assigned != "" && ch.assigned != actor {
		s.notifyAssigned(ctx, c, t, in.Note)
	}
	return t, nil
}

// save writes the changed row, its event, any link, and the caller's
// activity.
func (s *Service) save(ctx context.Context, q *db.Queries, ch *change) error {
	if c := ch.caller.SessionID; c != "" && c == ch.row.ClaimSession {
		ch.row.LastActivityAt = sql.NullInt64{Int64: ms(ch.now), Valid: true}
		ch.row.StaleNotified = 0
	}
	ch.row.UpdatedAt = ms(ch.now)
	r := ch.row
	if err := q.UpdateTask(ctx, db.UpdateTaskParams{
		Title: r.Title, Body: r.Body, Status: r.Status, Priority: r.Priority, Scope: r.Scope,
		Assignee: r.Assignee, Claimant: r.Claimant, ClaimSession: r.ClaimSession, StaleNotified: r.StaleNotified,
		DueAt: r.DueAt, ClaimedAt: r.ClaimedAt, LastActivityAt: r.LastActivityAt, FinishedAt: r.FinishedAt,
		UpdatedAt: r.UpdatedAt, ID: r.ID,
	}); err != nil {
		return fmt.Errorf("tasks: %s: save: %w", ch.op, err)
	}
	var from, to Status
	if ch.status() != ch.from {
		from, to = ch.from, ch.status()
	}
	if err := insertEvent(ctx, q, r.ID, ch.now, ch.actor, string(ch.in.Action), from, to, ch.note); err != nil {
		return err
	}
	if ch.linked != "" {
		if err := q.InsertTaskLink(ctx, db.InsertTaskLinkParams{TaskID: r.ID, MessageID: ch.linked, LinkedBy: ch.actor, LinkedAt: ms(ch.now)}); err != nil {
			return fmt.Errorf("tasks: %s: link: %w", ch.op, err)
		}
	}
	// Any update by an agent session is activity for everything it holds.
	if sid := ch.caller.SessionID; sid != "" && !ch.caller.Operator {
		if _, err := q.TouchTasksBySession(ctx, db.TouchTasksBySessionParams{
			Now: sql.NullInt64{Int64: ms(ch.now), Valid: true}, ClaimSession: sid,
		}); err != nil {
			return fmt.Errorf("tasks: %s: touch: %w", ch.op, err)
		}
	}
	return nil
}

// Link records that the message is about the task, on behalf of c. It is the
// link action without the caller needing to build an UpdateInput.
func (s *Service) Link(ctx context.Context, c mail.Caller, taskID, messageID string) error {
	_, err := s.Update(ctx, c, UpdateInput{TaskID: taskID, Action: ActionLink, MessageID: messageID})
	return err
}

// LinkSent links a message that was just sent to a task. A failure is
// reported as "sent but not linked" so the caller does not resend: the
// message is already delivered and the link can be retried on its own.
func (s *Service) LinkSent(ctx context.Context, c mail.Caller, taskID, messageID string) error {
	if err := s.Link(ctx, c, taskID, messageID); err != nil {
		return fmt.Errorf("message %s was sent, but linking it to %s failed (retry with task_update action link): %w", messageID, taskID, err)
	}
	return nil
}

// Exists reports whether a task with the id exists, returning an error
// wrapping ErrNotFound when it does not.
func (s *Service) Exists(ctx context.Context, id string) error {
	if _, err := s.store.Read().GetTask(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notFound("check", id)
		}
		return fmt.Errorf("tasks: check: %w", err)
	}
	return nil
}
