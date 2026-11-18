package devseed

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	tsk "github.com/scrothers/aim/internal/tasks"
)

// maxLiveBoard caps the open tasks the simulated agents keep creating.
const maxLiveBoard = 24

// liveTask is a task the simulated agents created and still track.
type liveTask struct {
	id, title string
	assignee  *agent // nil when anyone may claim it
	claimant  *agent
	status    tsk.Status
}

var (
	progressNotes = []string{
		"Draft PR is up; waiting on CI.", "Found the root cause; writing the fix.", "Blocked on a review, pinging the reviewer.",
		"Tests pass locally; running the full suite.", "Split a follow-up out for the migration.",
	}
	finishNotes  = []string{"Merged.", "Shipped behind a flag.", "Done; follow-up filed."}
	releaseNotes = []string{"Reprioritized; someone else can pick this up.", "Blocked on an upstream change."}
	titleVerbs   = []string{"", "", "Follow-up: ", "Spike: ", "Tech debt: "}
	priorities   = []tsk.Priority{tsk.PriorityLow, tsk.PriorityNormal, tsk.PriorityNormal, tsk.PriorityNormal, tsk.PriorityHigh, tsk.PriorityUrgent}
)

// callInto calls a tool and decodes its structured result into out.
func (a *agent) callInto(ctx context.Context, tool string, args, out any) error {
	res, err := a.callRes(ctx, tool, args)
	if err != nil {
		return err
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil || res.StructuredContent == nil {
		b = []byte(textOf(res))
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("devseed: %s %s: decode result: %w", a.p, tool, err)
	}
	return nil
}

func (a *agent) taskUpdate(ctx context.Context, id string, action tsk.Action, note string) error {
	args := map[string]any{"task_id": id, "action": string(action)}
	if note != "" {
		args["note"] = note
	}
	return a.call(ctx, "task_update", args)
}

// taskTick has one online agent move the task board along: advance a task it
// holds, take an open one, or create a new one.
func (l *liveLoop) taskTick(ctx context.Context) error {
	on := l.online()
	if len(on) == 0 {
		return nil
	}
	a := pick(l.rng, on)
	var mine, avail []*liveTask
	open := 0
	for _, t := range l.board {
		open++
		switch {
		case t.claimant == a:
			mine = append(mine, t)
		case t.status == tsk.StatusNotStarted && (t.assignee == nil || t.assignee == a):
			avail = append(avail, t)
		}
	}
	switch r := l.rng.Float64(); {
	case len(mine) > 0 && r < 0.6:
		return l.advance(ctx, a, pick(l.rng, mine))
	case len(avail) > 0 && r < 0.8:
		return l.take(ctx, a, pick(l.rng, avail))
	case open >= maxLiveBoard:
		return nil
	default:
		return l.createTask(ctx, a)
	}
}

// drop forgets a task whose state the loop can no longer be sure of.
func (l *liveLoop) drop(t *liveTask) {
	l.board = slices.DeleteFunc(l.board, func(x *liveTask) bool { return x == t })
}

func (l *liveLoop) createTask(ctx context.Context, a *agent) error {
	scope := a.p.scope()
	fl, ok := flavors()[scope]
	if !ok {
		scope = acme + "/api"
		fl = flavors()[scope]
	}
	title := pick(l.rng, titleVerbs) + pick(l.rng, fl.titles)
	if slices.ContainsFunc(l.board, func(t *liveTask) bool { return t.title == title }) {
		title = fmt.Sprintf("%s (%d)", title, l.n)
	}
	file := pick(l.rng, fl.files)
	args := map[string]any{
		"title": title, "scope": scope, "priority": string(pick(l.rng, priorities)),
		"body": acceptance(fmt.Sprintf("Needs work in ¦%s¦.", file), "Change covered by "+"¦"+pick(l.rng, fl.tests)+"¦", "No behavior change for existing callers"),
	}
	var assignee *agent
	if peers := l.peersOf(a); len(peers) > 0 && l.rng.Float64() < 0.4 {
		assignee = pick(l.rng, peers)
		args["assignee"] = assignee.p.String()
	}
	if l.rng.Float64() < 0.25 {
		args["due_at"] = time.Now().UTC().Add(time.Duration(4+l.rng.IntN(68)) * time.Hour).Format(time.RFC3339)
	}
	var t struct {
		ID string `json:"id"`
	}
	if err := a.callInto(ctx, "task_create", args, &t); err != nil {
		return err
	}
	lt := &liveTask{id: t.ID, title: title, assignee: assignee, status: tsk.StatusNotStarted}
	l.board = append(l.board, lt)
	if assignee != nil {
		say(l.out, "%s creates task %s for %s: %s\n", a.p, t.ID, assignee.p, title)
	} else {
		say(l.out, "%s creates task %s: %s\n", a.p, t.ID, title)
	}

	// Tell someone about it, linking the message to the task.
	var to *persona
	switch {
	case assignee != nil:
		to = assignee.p
	case l.rng.Float64() < 0.4:
		to = l.byAddr(a.p.reports)
	}
	if to == nil || to == a.p {
		return nil
	}
	say(l.out, "%s -> %s: Task: %s (linked to %s)\n", a.p, to, title, t.ID)
	return a.call(ctx, "send", map[string]any{
		"to": []string{to.String()}, "subject": "Task: " + title, "task_id": t.ID,
		"body": md(fmt.Sprintf("Filed %s for ¦%s¦. %s", t.ID, file, pick(l.rng, progressNotes))),
	})
}

// peersOf returns the other online agents of a's org.
func (l *liveLoop) peersOf(a *agent) []*agent {
	var out []*agent
	for _, b := range l.online() {
		if b != a && b.p.org() == a.p.org() {
			out = append(out, b)
		}
	}
	return out
}

// take claims or starts an open task.
func (l *liveLoop) take(ctx context.Context, a *agent, t *liveTask) error {
	action, status, verb := tsk.ActionClaim, tsk.StatusClaimed, "claims"
	if l.rng.Float64() < 0.4 {
		action, status, verb = tsk.ActionStart, tsk.StatusStarted, "starts"
	}
	if err := a.taskUpdate(ctx, t.id, action, ""); err != nil {
		l.drop(t)
		return err
	}
	t.claimant, t.status = a, status
	say(l.out, "%s %s task %s: %s\n", a.p, verb, t.id, t.title)
	return nil
}

// advance moves a held task forward: start it, or comment, finish or release.
func (l *liveLoop) advance(ctx context.Context, a *agent, t *liveTask) error {
	if t.status == tsk.StatusClaimed {
		if err := a.taskUpdate(ctx, t.id, tsk.ActionStart, ""); err != nil {
			l.drop(t)
			return err
		}
		t.status = tsk.StatusStarted
		say(l.out, "%s starts task %s: %s\n", a.p, t.id, t.title)
		return nil
	}
	var (
		action tsk.Action
		note   string
		verb   string
	)
	switch r := l.rng.Float64(); {
	case r < 0.45:
		action, note, verb = tsk.ActionComment, pick(l.rng, progressNotes), "comments on"
	case r < 0.85:
		action, note, verb = tsk.ActionFinish, pick(l.rng, finishNotes), "finishes"
	default:
		action, note, verb = tsk.ActionRelease, pick(l.rng, releaseNotes), "releases"
	}
	if err := a.taskUpdate(ctx, t.id, action, note); err != nil {
		l.drop(t)
		return err
	}
	switch action {
	case tsk.ActionFinish:
		l.drop(t)
	case tsk.ActionRelease:
		t.claimant, t.status = nil, tsk.StatusNotStarted
	}
	say(l.out, "%s %s task %s: %s\n", a.p, verb, t.id, strings.TrimSpace(t.title))
	return nil
}

// startQuiet has one agent create, claim and start a task, then marks it so
// it takes no further part; goQuiet then disconnects it. It returns nil when
// that failed.
func (l *liveLoop) startQuiet(ctx context.Context) *agent {
	var cands []*agent
	for _, a := range l.agents {
		if a.signedOn && a.p.role != roleCoordinator {
			cands = append(cands, a)
		}
	}
	if len(cands) == 0 {
		return nil
	}
	a := pick(l.rng, cands)
	scope := a.p.scope()
	fl, ok := flavors()[scope]
	if !ok {
		scope = acme + "/api"
		fl = flavors()[scope]
	}
	title := "Hotfix: " + pick(l.rng, fl.titles)
	var t struct {
		ID string `json:"id"`
	}
	err := a.callInto(ctx, "task_create", map[string]any{
		"title": title, "scope": scope, "priority": string(tsk.PriorityHigh),
		"body": acceptance("Production is affected; fix and ship today.", "Fix verified in staging", "Rollback plan noted"),
	}, &t)
	if err == nil {
		if err = a.taskUpdate(ctx, t.ID, tsk.ActionStart, ""); err == nil {
			say(l.out, "%s starts task %s: %s\n", a.p, t.ID, title)
			a.quiet = true
			l.quiet, l.quietTask = a, t.ID
			return a
		}
	}
	say(l.out, "quiet agent: %v\n", err)
	return nil
}

// goQuiet disconnects the quiet agent for good; the task it holds then decays
// at the daemon's thresholds: idle, stale, then abandoned.
func (l *liveLoop) goQuiet() {
	if l.quiet == nil {
		return
	}
	say(l.out, "%s goes quiet holding task %s; its lease will decay\n", l.quiet.p, l.quietTask)
	l.quiet.disconnect()
}
