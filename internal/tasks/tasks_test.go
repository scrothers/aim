package tasks_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/tasks"
)

const (
	alice = "alice@github.com/acme/web"
	bob   = "bob@github.com/acme/web"
	carol = "carol@github.com/acme/api"
)

var epoch = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

type clock struct{ ns atomic.Int64 }

func (c *clock) now() time.Time          { return epoch.Add(time.Duration(c.ns.Load())) }
func (c *clock) advance(d time.Duration) { c.ns.Add(int64(d)) }
func (c *clock) set(d time.Duration)     { c.ns.Store(int64(d)) }

type sessions struct {
	mu   sync.Mutex
	live map[string]bool
}

func (s *sessions) Connected(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live[id]
}

func (s *sessions) setLive(id string, v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[id] = v
}

type env struct {
	t    *testing.T
	st   *store.Store
	mail *mail.Service
	svc  *tasks.Service
	clk  *clock
	sess *sessions
}

var testDecay = tasks.Decay{IdleAfter: 10 * time.Minute, StaleAfter: 20 * time.Minute, AbandonAfter: 30 * time.Minute, MaxHold: 2 * time.Hour}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "aim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &env{t: t, st: st, clk: &clock{}, sess: &sessions{live: map[string]bool{}}}
	e.mail = mail.New(st, nil, mail.WithClock(e.clk.now))
	if err := e.mail.EnsureOperator(ctx); err != nil {
		t.Fatal(err)
	}
	e.svc = tasks.New(st, e.mail, tasks.WithClock(e.clk.now), tasks.WithDecay(testDecay), tasks.WithSessions(e.sess))
	for _, a := range []string{alice, bob, carol} {
		if _, err := e.mail.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: a, Role: "dev", Description: "d"}); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func agent(a string) mail.Caller {
	return mail.Caller{SessionID: "sess-" + a, Identity: address.MustParse(a), SignedOn: true}
}

func operator() mail.Caller { return mail.OperatorCaller("op-sess", "web", "") }

func (e *env) create(c mail.Caller, in tasks.CreateInput) tasks.Task {
	e.t.Helper()
	if in.Title == "" {
		in.Title = "do the thing"
	}
	tk, err := e.svc.Create(e.t.Context(), c, in)
	if err != nil {
		e.t.Fatalf("create: %v", err)
	}
	return tk
}

func (e *env) update(c mail.Caller, in tasks.UpdateInput) tasks.Task {
	e.t.Helper()
	tk, err := e.svc.Update(e.t.Context(), c, in)
	if err != nil {
		e.t.Fatalf("update %s: %v", in.Action, err)
	}
	return tk
}

func (e *env) act(c mail.Caller, id string, a tasks.Action) (tasks.Task, error) {
	return e.svc.Update(e.t.Context(), c, tasks.UpdateInput{TaskID: id, Action: a})
}

func (e *env) get(id string) tasks.Detail {
	e.t.Helper()
	d, err := e.svc.Get(e.t.Context(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

// inbox returns the subjects delivered to addr.
func (e *env) inbox(addr string) []string {
	e.t.Helper()
	r, err := e.mail.List(e.t.Context(), mail.ListInput{Address: addr, State: mail.StateAll, Limit: 100})
	if err != nil {
		e.t.Fatal(err)
	}
	out := make([]string, len(r.Items))
	for i, h := range r.Items {
		out[i] = h.Subject
	}
	return out
}

func kinds(d tasks.Detail) []string {
	out := make([]string, len(d.Events))
	for i, ev := range d.Events {
		out[i] = ev.Kind
	}
	return out
}

func TestCreate(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	due := epoch.Add(48 * time.Hour)
	tk := e.create(agent(alice), tasks.CreateInput{Title: "  Fix login  ", Body: "details", Priority: tasks.PriorityHigh, Scope: "GitHub.com/acme", DueAt: &due})
	if !strings.HasPrefix(tk.ID, "task-") || tk.ID != strings.ToLower(tk.ID) || len(tk.ID) != len("task-")+26 {
		t.Errorf("id = %q", tk.ID)
	}
	if tk.Title != "Fix login" || tk.Status != tasks.StatusNotStarted || tk.Priority != tasks.PriorityHigh ||
		tk.Scope != "github.com/acme" || tk.CreatedBy != alice || tk.Health != "" || !tk.DueAt.Equal(due) {
		t.Errorf("task = %+v", tk)
	}
	d := e.get(tk.ID)
	if got := kinds(d); len(got) != 1 || got[0] != tasks.EventCreated || d.Events[0].ToStatus != tasks.StatusNotStarted {
		t.Errorf("events = %+v", d.Events)
	}
	if d2 := e.create(operator(), tasks.CreateInput{}); d2.CreatedBy != tasks.Operator || d2.Priority != tasks.PriorityNormal {
		t.Errorf("operator task = %+v", d2)
	}
}

func TestCreateInvalid(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cases := map[string]tasks.CreateInput{
		"no title":       {Title: "  "},
		"long title":     {Title: strings.Repeat("x", tasks.MaxTitleLen+1)},
		"multiline":      {Title: "a\nb"},
		"long body":      {Title: "x", Body: strings.Repeat("x", tasks.MaxBodyLen+1)},
		"priority":       {Title: "x", Priority: "meh"},
		"scope identity": {Title: "x", Scope: alice},
		"scope bad":      {Title: "x", Scope: "nope"},
		"assignee bad":   {Title: "x", Assignee: "github.com/acme"},
	}
	for name, in := range cases {
		if _, err := e.svc.Create(e.t.Context(), agent(alice), in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	_, err := e.svc.Create(e.t.Context(), agent(alice), tasks.CreateInput{Title: "x", Assignee: "bobb@github.com/acme/web"})
	var nf *mail.MailboxNotFoundError
	if !errors.As(err, &nf) || len(nf.Suggestions) == 0 || nf.Suggestions[0] != bob {
		t.Errorf("unknown assignee err = %v", err)
	}
	if _, err := e.svc.Create(e.t.Context(), agent(alice), tasks.CreateInput{Title: "x", LinkMessageID: "msg-nope"}); !errors.Is(err, mail.ErrMessageNotFound) {
		t.Errorf("bad link err = %v", err)
	}
	if _, err := e.svc.Create(e.t.Context(), mail.Caller{SessionID: "anon"}, tasks.CreateInput{Title: "x"}); !errors.Is(err, mail.ErrNotSignedOn) {
		t.Errorf("unsigned err = %v", err)
	}
}

func TestAssignmentReservesAndMails(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	msg, err := e.mail.Send(e.t.Context(), agent(alice), mail.SendInput{To: []string{bob}, Subject: "about it", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	tk := e.create(agent(alice), tasks.CreateInput{Title: "review", Assignee: bob, LinkMessageID: msg.ID})
	if tk.Assignee != bob {
		t.Fatalf("assignee = %q", tk.Assignee)
	}
	if got := e.inbox(bob); len(got) != 2 || got[0] != "Task assigned: review" {
		t.Errorf("bob inbox = %q", got)
	}
	if _, err := e.act(agent(carol), tk.ID, tasks.ActionClaim); !errors.Is(err, tasks.ErrForbidden) {
		t.Errorf("carol claim err = %v, want ErrForbidden", err)
	}
	if _, err := e.act(agent(carol), tk.ID, tasks.ActionStart); !errors.Is(err, tasks.ErrForbidden) {
		t.Errorf("carol start err = %v, want ErrForbidden", err)
	}
	got := e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim})
	if got.Status != tasks.StatusClaimed || got.Claimant != bob || got.Health != tasks.HealthActive || got.ClaimedAt == nil {
		t.Errorf("claimed = %+v", got)
	}
	d := e.get(tk.ID)
	if want := []string{"created", "assign", "link", "claim"}; strings.Join(kinds(d), ",") != strings.Join(want, ",") {
		t.Errorf("events = %v", kinds(d))
	}
	if len(d.Links) != 1 || d.Links[0].MessageID != msg.ID || d.Links[0].ThreadID != msg.ThreadID || d.Links[0].Subject != "about it" {
		t.Errorf("links = %+v", d.Links)
	}
	// Self-assignment sends nothing.
	before := len(e.inbox(alice))
	e.create(agent(alice), tasks.CreateInput{Assignee: alice})
	if after := len(e.inbox(alice)); after != before {
		t.Errorf("self-assign mailed: %d -> %d", before, after)
	}
}

func TestLifecycle(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(agent(alice), tasks.CreateInput{})
	steps := []struct {
		c    mail.Caller
		a    tasks.Action
		want tasks.Status
	}{
		{agent(bob), tasks.ActionClaim, tasks.StatusClaimed},
		{agent(bob), tasks.ActionStart, tasks.StatusStarted},
		{agent(bob), tasks.ActionRelease, tasks.StatusNotStarted},
		{agent(carol), tasks.ActionStart, tasks.StatusStarted},
		{agent(carol), tasks.ActionFinish, tasks.StatusFinished},
		{agent(alice), tasks.ActionReopen, tasks.StatusNotStarted},
		{agent(bob), tasks.ActionClaim, tasks.StatusClaimed},
		{agent(alice), tasks.ActionRelease, tasks.StatusNotStarted},
		{operator(), tasks.ActionClaim, tasks.StatusClaimed},
		{operator(), tasks.ActionFinish, tasks.StatusFinished},
	}
	for i, s := range steps {
		e.clk.advance(time.Second)
		got, err := e.act(s.c, tk.ID, s.a)
		if err != nil {
			t.Fatalf("step %d %s: %v", i, s.a, err)
		}
		if got.Status != s.want {
			t.Fatalf("step %d %s: status %s, want %s", i, s.a, got.Status, s.want)
		}
		held := s.want == tasks.StatusClaimed || s.want == tasks.StatusStarted
		if held != (got.Claimant != "") || held != (got.Health != "") {
			t.Errorf("step %d: claimant %q health %q", i, got.Claimant, got.Health)
		}
		if (s.want == tasks.StatusFinished) != (got.FinishedAt != nil) {
			t.Errorf("step %d: finished_at %v", i, got.FinishedAt)
		}
	}
	d := e.get(tk.ID)
	if len(d.Events) != len(steps)+1 || d.Events[1].FromStatus != tasks.StatusNotStarted || d.Events[1].ToStatus != tasks.StatusClaimed || d.Events[1].Actor != bob {
		t.Errorf("events = %+v", d.Events)
	}
}

func TestTransitionErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	open := e.create(agent(alice), tasks.CreateInput{})
	held := e.create(agent(alice), tasks.CreateInput{})
	e.update(agent(bob), tasks.UpdateInput{TaskID: held.ID, Action: tasks.ActionStart})
	done := e.create(agent(alice), tasks.CreateInput{})
	e.update(agent(bob), tasks.UpdateInput{TaskID: done.ID, Action: tasks.ActionStart})
	e.update(agent(bob), tasks.UpdateInput{TaskID: done.ID, Action: tasks.ActionFinish})
	claimed := e.create(agent(alice), tasks.CreateInput{})
	e.update(agent(bob), tasks.UpdateInput{TaskID: claimed.ID, Action: tasks.ActionClaim})

	cases := []struct {
		name string
		c    mail.Caller
		id   string
		a    tasks.Action
	}{
		{"finish open", agent(alice), open.ID, tasks.ActionFinish},
		{"release open", agent(alice), open.ID, tasks.ActionRelease},
		{"reopen open", agent(alice), open.ID, tasks.ActionReopen},
		{"unassign unassigned", agent(alice), open.ID, tasks.ActionUnassign},
		{"claim held", agent(carol), held.ID, tasks.ActionClaim},
		{"start started", agent(bob), held.ID, tasks.ActionStart},
		{"start claimed by other", agent(carol), claimed.ID, tasks.ActionStart},
		{"claim finished", agent(bob), done.ID, tasks.ActionClaim},
		{"start finished", agent(bob), done.ID, tasks.ActionStart},
	}
	for _, c := range cases {
		if _, err := e.act(c.c, c.id, c.a); !errors.Is(err, tasks.ErrTransition) {
			t.Errorf("%s: err = %v, want ErrTransition", c.name, err)
		}
	}
	if _, err := e.svc.Update(e.t.Context(), agent(alice), tasks.UpdateInput{TaskID: done.ID, Action: tasks.ActionAssign, Assignee: bob}); !errors.Is(err, tasks.ErrTransition) {
		t.Errorf("assign finished err = %v", err)
	}
	// Re-claiming your own task moves the lease to the new session.
	moved := agent(bob)
	moved.SessionID = "sess-new"
	if got, err := e.act(moved, held.ID, tasks.ActionClaim); err != nil || got.Status != tasks.StatusStarted {
		t.Errorf("reclaim = %+v, %v", got, err)
	}
	if _, err := e.act(agent(alice), "task-nope", tasks.ActionClaim); !errors.Is(err, tasks.ErrNotFound) {
		t.Errorf("missing err = %v", err)
	}
	if _, err := e.act(agent(alice), open.ID, "dance"); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("bad action err = %v", err)
	}
	if _, err := e.act(mail.Caller{}, open.ID, tasks.ActionClaim); !errors.Is(err, mail.ErrNotSignedOn) {
		t.Errorf("unsigned err = %v", err)
	}
	if _, err := e.svc.Update(e.t.Context(), agent(alice), tasks.UpdateInput{TaskID: open.ID, Action: tasks.ActionComment, Note: strings.Repeat("x", tasks.MaxNoteLen+1)}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("long note err = %v", err)
	}
}

func TestPermissions(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(agent(alice), tasks.CreateInput{})
	e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionStart})
	title := "new"
	denied := []struct {
		name string
		c    mail.Caller
		in   tasks.UpdateInput
	}{
		{"finish by creator", agent(alice), tasks.UpdateInput{Action: tasks.ActionFinish}},
		{"release by stranger", agent(carol), tasks.UpdateInput{Action: tasks.ActionRelease}},
		{"assign by stranger", agent(carol), tasks.UpdateInput{Action: tasks.ActionAssign, Assignee: carol}},
		{"edit by stranger", agent(carol), tasks.UpdateInput{Action: tasks.ActionEdit, Title: &title}},
	}
	for _, d := range denied {
		d.in.TaskID = tk.ID
		if _, err := e.svc.Update(e.t.Context(), d.c, d.in); !errors.Is(err, tasks.ErrForbidden) {
			t.Errorf("%s: err = %v, want ErrForbidden", d.name, err)
		}
	}
	// Claimant may edit; finishing then lets only the creator reopen.
	e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionEdit, Title: &title})
	e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionFinish})
	if _, err := e.act(agent(bob), tk.ID, tasks.ActionReopen); !errors.Is(err, tasks.ErrForbidden) {
		t.Errorf("reopen by claimant err = %v", err)
	}
	e.update(operator(), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionReopen})

	// The assignee may hand off; afterwards it is the new assignee's.
	e.update(agent(alice), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionAssign, Assignee: bob})
	if _, err := e.svc.Update(e.t.Context(), agent(alice), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionAssign, Assignee: bob}); !errors.Is(err, tasks.ErrTransition) {
		t.Errorf("same assignee err = %v", err)
	}
	got := e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionAssign, Assignee: "operator", Note: "please look"})
	if got.Assignee != tasks.Operator {
		t.Errorf("assignee = %q", got.Assignee)
	}
	if got := e.inbox("operator"); len(got) != 1 {
		t.Errorf("operator inbox = %q", got)
	}
	if _, err := e.act(agent(bob), tk.ID, tasks.ActionUnassign); !errors.Is(err, tasks.ErrForbidden) {
		t.Errorf("unassign by former assignee err = %v", err)
	}
	if got := e.update(operator(), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionUnassign}); got.Assignee != "" {
		t.Errorf("unassigned = %+v", got)
	}
	if _, err := e.svc.Update(e.t.Context(), agent(alice), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionAssign}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("assign without assignee err = %v", err)
	}
	if _, err := e.svc.Update(e.t.Context(), agent(alice), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionAssign, Assignee: "zed@github.com/acme/web"}); !errors.Is(err, mail.ErrMailboxNotFound) {
		t.Errorf("assign unknown err = %v", err)
	}
}

func TestCommentLinkEdit(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(agent(alice), tasks.CreateInput{DueAt: new(epoch.Add(time.Hour))})
	if _, err := e.act(agent(carol), tk.ID, tasks.ActionComment); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("empty comment err = %v", err)
	}
	e.update(agent(carol), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionComment, Note: "looks hard"})
	if _, err := e.act(agent(carol), tk.ID, tasks.ActionLink); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("empty link err = %v", err)
	}
	if _, err := e.svc.Update(e.t.Context(), agent(carol), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionLink, MessageID: "msg-x"}); !errors.Is(err, mail.ErrMessageNotFound) {
		t.Errorf("bad link err = %v", err)
	}
	msg, err := e.mail.Send(e.t.Context(), agent(carol), mail.SendInput{To: []string{alice}, Subject: "s", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Link(e.t.Context(), agent(carol), tk.ID, msg.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Link(e.t.Context(), agent(carol), tk.ID, msg.ID); err != nil {
		t.Fatalf("relink: %v", err)
	}
	if _, err := e.act(agent(alice), tk.ID, tasks.ActionEdit); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("empty edit err = %v", err)
	}
	title, body, prio, scope := "T2", "B2", tasks.PriorityUrgent, "github.com/acme/web"
	got := e.update(agent(alice), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionEdit, Title: &title, Body: &body, Priority: &prio, Scope: &scope, ClearDue: true})
	if got.Title != "T2" || got.Body != "B2" || got.Priority != prio || got.Scope != scope || got.DueAt != nil {
		t.Errorf("edited = %+v", got)
	}
	due := epoch.Add(time.Hour)
	if got := e.update(agent(alice), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionEdit, DueAt: &due}); got.DueAt == nil {
		t.Error("due not set")
	}
	bad := map[string]tasks.UpdateInput{
		"title":    {Title: new("")},
		"body":     {Body: new(strings.Repeat("x", tasks.MaxBodyLen+1))},
		"priority": {Priority: new(tasks.Priority("x"))},
		"scope":    {Scope: new("x")},
	}
	for name, in := range bad {
		in.TaskID, in.Action = tk.ID, tasks.ActionEdit
		if _, err := e.svc.Update(e.t.Context(), agent(alice), in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("edit %s err = %v", name, err)
		}
	}
	d := e.get(tk.ID)
	if len(d.Links) != 1 {
		t.Errorf("links = %+v", d.Links)
	}
	notes := make([]string, 0, len(d.Events))
	for _, ev := range d.Events {
		notes = append(notes, ev.Note)
	}
	joined := strings.Join(notes, "|")
	for _, want := range []string{"looks hard", msg.ID, "changed title, body, priority, scope, due_at"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes %q missing %q", joined, want)
		}
	}
	if err := e.svc.Exists(e.t.Context(), tk.ID); err != nil {
		t.Error(err)
	}
	if err := e.svc.Exists(e.t.Context(), "task-zzz"); !errors.Is(err, tasks.ErrNotFound) {
		t.Errorf("exists err = %v", err)
	}
	if _, err := e.svc.Get(e.t.Context(), "task-zzz"); !errors.Is(err, tasks.ErrNotFound) {
		t.Errorf("get err = %v", err)
	}
	threads, err := e.svc.ForThreads(e.t.Context(), []string{msg.ThreadID, "msg-none"})
	if err != nil || len(threads) != 1 || threads[0].TaskID != tk.ID || threads[0].Title != "T2" {
		t.Errorf("for threads = %+v, %v", threads, err)
	}
	if got, err := e.svc.ForThreads(e.t.Context(), nil); err != nil || len(got) != 0 {
		t.Errorf("empty for threads = %v, %v", got, err)
	}
}

func TestListFiltersAndPaging(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ids := make([]string, 0, 7)
	for i := range 7 {
		e.clk.advance(time.Second)
		in := tasks.CreateInput{Title: "t", Scope: "github.com/acme/web"}
		if i%2 == 0 {
			in.Scope = "github.com/acme/api"
		}
		if i == 6 {
			in.Scope = "github.com/acme_x"
		}
		ids = append(ids, e.create(agent(alice), in).ID)
	}
	e.clk.advance(time.Second)
	e.update(agent(bob), tasks.UpdateInput{TaskID: ids[0], Action: tasks.ActionClaim})
	e.clk.advance(time.Second)
	e.update(agent(carol), tasks.UpdateInput{TaskID: ids[1], Action: tasks.ActionStart})
	e.update(agent(carol), tasks.UpdateInput{TaskID: ids[1], Action: tasks.ActionFinish})
	e.update(agent(alice), tasks.UpdateInput{TaskID: ids[2], Action: tasks.ActionAssign, Assignee: bob})

	count := func(in tasks.ListInput) int {
		t.Helper()
		r, err := e.svc.List(e.t.Context(), in)
		if err != nil {
			t.Fatalf("list %+v: %v", in, err)
		}
		return len(r.Items)
	}
	checks := []struct {
		in   tasks.ListInput
		want int
	}{
		{tasks.ListInput{}, 7},
		{tasks.ListInput{OpenOnly: true}, 6},
		{tasks.ListInput{Status: tasks.StatusClaimed}, 1},
		{tasks.ListInput{Claimant: bob}, 1},
		{tasks.ListInput{Assignee: "BOB@github.com/acme/web"}, 1},
		{tasks.ListInput{Involves: bob}, 2},
		{tasks.ListInput{Involves: alice}, 7},
		{tasks.ListInput{Scope: "github.com/acme"}, 6},
		{tasks.ListInput{Scope: "github.com/acme/web"}, 3},
		{tasks.ListInput{Scope: "github.com/acme_x"}, 1},
	}
	for _, c := range checks {
		if got := count(c.in); got != c.want {
			t.Errorf("list %+v = %d, want %d", c.in, got, c.want)
		}
	}
	// Newest-updated first, paged by three.
	var seen []string
	cur := ""
	for page := 0; ; page++ {
		r, err := e.svc.List(e.t.Context(), tasks.ListInput{Limit: 3, Cursor: cur})
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range r.Items {
			seen = append(seen, it.ID)
		}
		if r.NextCursor == "" {
			break
		}
		cur = r.NextCursor
		if page > 5 {
			t.Fatal("paging did not end")
		}
	}
	if len(seen) != 7 || seen[0] != ids[2] || seen[1] != ids[1] {
		t.Errorf("paged = %v (ids %v)", seen, ids)
	}
	bad := []tasks.ListInput{{Status: "x"}, {Assignee: "!"}, {Claimant: "!"}, {Involves: "!"}, {Scope: alice}, {Cursor: "zzz"}, {Cursor: "1_msg-x"}}
	for _, in := range bad {
		if _, err := e.svc.List(e.t.Context(), in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("list %+v err = %v", in, err)
		}
	}
	if count(tasks.ListInput{Limit: 1000}) != 7 {
		t.Error("big limit")
	}
}

func TestHealthStages(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(agent(alice), tasks.CreateInput{})
	e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionStart})
	stages := []struct {
		at   time.Duration
		live bool
		want tasks.Health
	}{
		{9 * time.Minute, false, tasks.HealthActive},
		{10 * time.Minute, false, tasks.HealthIdle},
		{20 * time.Minute, false, tasks.HealthStale},
		{30 * time.Minute, true, tasks.HealthStale},
		{30 * time.Minute, false, tasks.HealthAbandoned},
		{2 * time.Hour, true, tasks.HealthAbandoned},
	}
	for _, s := range stages {
		e.clk.set(s.at)
		e.sess.setLive("sess-"+bob, s.live)
		if got := e.get(tk.ID).Health; got != s.want {
			t.Errorf("at %s live=%v: health %s, want %s", s.at, s.live, got, s.want)
		}
	}
}

func TestTouch(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(agent(alice), tasks.CreateInput{})
	other := e.create(agent(alice), tasks.CreateInput{})
	e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim})
	if err := e.svc.Touch(e.t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	e.clk.set(25 * time.Minute)
	if h := e.get(tk.ID).Health; h != tasks.HealthStale {
		t.Fatalf("health = %s", h)
	}
	ch := e.svc.Changed()
	if err := e.svc.Touch(e.t.Context(), "sess-"+bob); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	default:
		t.Error("touch did not broadcast")
	}
	d := e.get(tk.ID)
	if d.Health != tasks.HealthActive || !d.LastActivityAt.Equal(e.clk.now()) {
		t.Errorf("after touch = %+v", d.Task)
	}
	// Within a second nothing is written or broadcast.
	ch = e.svc.Changed()
	if err := e.svc.Touch(e.t.Context(), "sess-"+bob); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
		t.Error("redundant touch broadcast")
	default:
	}
	// A comment from the claimant's session on another task is activity too.
	e.clk.advance(15 * time.Minute)
	e.update(agent(bob), tasks.UpdateInput{TaskID: other.ID, Action: tasks.ActionComment, Note: "hi"})
	if h := e.get(tk.ID).Health; h != tasks.HealthActive {
		t.Errorf("health after update = %s", h)
	}
}

func TestSweepStaleOnceThenAbandon(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(agent(alice), tasks.CreateInput{Title: "port it"})
	e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionStart})
	e.sess.setLive("sess-"+bob, true)

	sweep := func() int {
		t.Helper()
		n, err := e.svc.Sweep(e.t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	e.clk.set(15 * time.Minute)
	if sweep() != 0 || len(e.inbox(alice)) != 0 {
		t.Fatal("idle task swept")
	}
	e.clk.set(21 * time.Minute)
	sweep()
	e.clk.set(22 * time.Minute)
	sweep()
	got := e.inbox(alice)
	if len(got) != 1 || got[0] != "[AIM] Task going stale: port it" {
		t.Fatalf("alice inbox = %q", got)
	}
	if b := e.inbox(bob); len(b) != 1 {
		t.Errorf("bob inbox = %q", b)
	}
	// Connected: not abandoned at AbandonAfter.
	e.clk.set(45 * time.Minute)
	if sweep() != 0 || e.get(tk.ID).Status != tasks.StatusStarted {
		t.Fatal("connected task abandoned early")
	}
	// Disconnected: abandoned.
	e.sess.setLive("sess-"+bob, false)
	if sweep() != 1 {
		t.Fatal("not abandoned")
	}
	d := e.get(tk.ID)
	last := d.Events[len(d.Events)-1]
	if d.Status != tasks.StatusAbandoned || d.Claimant != "" || d.Health != "" ||
		last.Kind != tasks.EventDecayed || last.Actor != bob || !strings.Contains(last.Note, bob) || last.ToStatus != tasks.StatusAbandoned {
		t.Errorf("abandoned = %+v, last %+v", d.Task, last)
	}
	if strings.Join(kinds(d), ",") != "created,start,stale,decayed" {
		t.Errorf("events = %v", kinds(d))
	}
	got = e.inbox(alice)
	if len(got) != 2 || got[0] != "[AIM] Task abandoned: port it" {
		t.Fatalf("alice inbox = %q", got)
	}
	m, err := e.mail.Read(e.t.Context(), agent(alice), mail.ReadInput{MessageID: firstID(t, e, alice)})
	if err != nil {
		t.Fatal(err)
	}
	if m.From != bob || !strings.Contains(m.Body, "automatically") || !strings.Contains(m.Body, tk.ID) {
		t.Errorf("notice = %+v", m)
	}
	// Anyone may claim it again.
	if got := e.update(agent(carol), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim}); got.Claimant != carol {
		t.Errorf("reclaim = %+v", got)
	}
	if sweep() != 0 {
		t.Error("swept fresh claim")
	}
}

func firstID(t *testing.T, e *env, addr string) string {
	t.Helper()
	r, err := e.mail.List(e.t.Context(), mail.ListInput{Address: addr, State: mail.StateAll, Limit: 1})
	if err != nil || len(r.Items) == 0 {
		t.Fatalf("list %s: %v", addr, err)
	}
	return r.Items[0].ID
}

func TestSweepMaxHold(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(operator(), tasks.CreateInput{Title: "op work"})
	e.update(operator(), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim})
	e.sess.setLive("op-sess", true)
	e.clk.set(time.Hour)
	if n, err := e.svc.Sweep(e.t.Context()); err != nil || n != 0 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	e.clk.set(2 * time.Hour)
	if n, err := e.svc.Sweep(e.t.Context()); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	// The operator was both claimant and creator: one stale and one
	// abandonment notice, each to the operator only.
	if got := e.inbox("operator"); len(got) != 2 {
		t.Errorf("operator inbox = %q", got)
	}
}

func TestSweepReleasesReservation(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(agent(alice), tasks.CreateInput{Assignee: bob})
	e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionStart})
	e.clk.set(31 * time.Minute)
	if n, err := e.svc.Sweep(e.t.Context()); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	d := e.get(tk.ID)
	if d.Assignee != "" || !strings.Contains(d.Events[len(d.Events)-1].Note, "reservation for "+bob+" released") {
		t.Errorf("after decay = %+v, last event %+v", d.Task, d.Events[len(d.Events)-1])
	}
	if got := e.update(agent(carol), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim}); got.Claimant != carol {
		t.Errorf("carol claim = %+v", got)
	}
}

func TestTouchGranularityFollowsIdle(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	fast := tasks.New(e.st, e.mail, tasks.WithClock(e.clk.now), tasks.WithDecay(tasks.Decay{IdleAfter: time.Second}))
	tk, err := fast.Create(e.t.Context(), agent(alice), tasks.CreateInput{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fast.Update(e.t.Context(), agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim}); err != nil {
		t.Fatal(err)
	}
	e.clk.advance(200 * time.Millisecond) // past IdleAfter/10, short of a second
	if err := fast.Touch(e.t.Context(), "sess-"+bob); err != nil {
		t.Fatal(err)
	}
	d, err := fast.Get(e.t.Context(), tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !d.LastActivityAt.Equal(e.clk.now()) {
		t.Errorf("last activity = %v, want %v", d.LastActivityAt, e.clk.now())
	}
}

func TestSweepSkipsStaleStraightToAbandon(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	tk := e.create(agent(alice), tasks.CreateInput{Title: strings.Repeat("long ", 39) + "end"})
	e.update(agent(bob), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim})
	e.clk.set(31 * time.Minute)
	if n, err := e.svc.Sweep(e.t.Context()); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	if got := e.inbox(alice); len(got) != 1 || !strings.HasPrefix(got[0], "[AIM] Task abandoned: ") {
		t.Errorf("inbox = %q", got)
	}
	// abandoned -> reopen by creator -> not_started
	if got := e.update(agent(alice), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionReopen}); got.Status != tasks.StatusNotStarted {
		t.Errorf("reopened = %+v", got)
	}
}

func TestCounts(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if c, err := e.svc.Counts(e.t.Context(), bob); err != nil || c != (tasks.Counts{}) {
		t.Fatalf("empty counts = %+v, %v", c, err)
	}
	a := e.create(agent(alice), tasks.CreateInput{})
	b := e.create(agent(alice), tasks.CreateInput{})
	e.create(agent(alice), tasks.CreateInput{Assignee: bob})
	e.update(agent(bob), tasks.UpdateInput{TaskID: a.ID, Action: tasks.ActionClaim})
	e.clk.set(25 * time.Minute)
	e.update(agent(carol), tasks.UpdateInput{TaskID: b.ID, Action: tasks.ActionComment, Note: "x"})
	moved := agent(bob)
	moved.SessionID = "sess-other"
	e.update(moved, tasks.UpdateInput{TaskID: b.ID, Action: tasks.ActionClaim})
	c, err := e.svc.Counts(e.t.Context(), bob)
	if err != nil {
		t.Fatal(err)
	}
	if c != (tasks.Counts{Held: 2, Stale: 1, Assigned: 1}) {
		t.Errorf("counts = %+v", c)
	}
	if _, err := e.svc.Counts(e.t.Context(), "!"); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("bad counts err = %v", err)
	}
	if c, err := e.svc.Counts(e.t.Context(), ""); err != nil || c != (tasks.Counts{}) {
		t.Errorf("empty identity counts = %+v, %v", c, err)
	}
}

func TestChangedAndDecay(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ch := e.svc.Changed()
	e.create(agent(alice), tasks.CreateInput{})
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("create did not broadcast")
	}
	d := tasks.New(e.st, e.mail, tasks.WithDecay(tasks.Decay{IdleAfter: time.Minute}), tasks.WithSessions(nil)).Decay()
	if d.IdleAfter != time.Minute || d.StaleAfter != tasks.DefaultStaleAfter || d.AbandonAfter != tasks.DefaultAbandonAfter || d.MaxHold != tasks.DefaultMaxHold {
		t.Errorf("decay = %+v", d)
	}
}
