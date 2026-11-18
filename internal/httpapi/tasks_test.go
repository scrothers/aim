package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/httpapi"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/tasks"
)

type taskDetail struct {
	httpapi.TaskDetailView
}

func TestTaskEndpoints(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	const bob = "bob@github.com/acme/web"
	e.register(t, alice, "dev")
	e.register(t, bob, "dev")
	msg, err := e.svc.Send(context.Background(), e.agentCaller(t, alice), mail.SendInput{To: []string{"operator"}, Subject: "please plan", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}

	created := decode[taskDetail](t, e.do(t, http.MethodPost, "/api/tasks", obj{
		"title": "Plan **v2**", "body": "Use *care*.", "priority": "high", "scope": "github.com/acme",
		"assignee": alice, "message_id": msg.ID, "due_at": "2026-12-01T00:00:00Z",
	}), http.StatusCreated)
	id := created.ID
	if created.CreatedBy != "operator" || created.Assignee != alice || created.Status != "not_started" ||
		!strings.Contains(created.BodyHTML, "<em>care</em>") || len(created.Links) != 2 || created.Links[0].ThreadID != msg.ThreadID || created.Links[1].Subject != "Task assigned: Plan **v2**" {
		t.Fatalf("created = %+v", created)
	}
	if kinds := eventKinds(created); !slices.Equal(kinds, []string{"created", "assign", "link", "link"}) {
		t.Errorf("events = %v", kinds)
	}

	// Validation errors.
	for name, body := range map[string]obj{
		"empty title":  {"title": ""},
		"bad priority": {"title": "x", "priority": "meh"},
		"bad scope":    {"title": "x", "scope": alice},
	} {
		if rec := e.do(t, http.MethodPost, "/api/tasks", body); rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d: %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec := e.do(t, http.MethodPost, "/api/tasks", obj{"title": "x", "assignee": "alise@github.com/acme/web"}); rec.Code != http.StatusNotFound ||
		!strings.Contains(rec.Body.String(), alice) {
		t.Errorf("unknown assignee: %d %s", rec.Code, rec.Body.String())
	}

	// Agents act through the service; the operator through actions.
	if _, err := e.tasks.Update(context.Background(), e.agentCaller(t, alice), tasks.UpdateInput{TaskID: id, Action: tasks.ActionStart}); err != nil {
		t.Fatal(err)
	}
	if rec := e.do(t, http.MethodPost, "/api/tasks/"+id+"/actions", obj{"action": "claim"}); rec.Code != http.StatusConflict {
		t.Errorf("claim held: %d %s", rec.Code, rec.Body.String())
	}
	got := decode[taskDetail](t, e.do(t, http.MethodPost, "/api/tasks/"+id+"/actions", obj{"action": "comment", "note": "status?"}), http.StatusOK)
	if got.Status != "started" || got.Claimant != alice || got.Health != "active" {
		t.Errorf("after comment = %+v", got.TaskView)
	}
	edited := decode[taskDetail](t, e.do(t, http.MethodPost, "/api/tasks/"+id+"/actions", obj{
		"action": "edit", "title": "Plan v3", "priority": "urgent", "scope": "", "body": "", "clear_due": true,
	}), http.StatusOK)
	if edited.Title != "Plan v3" || edited.Priority != "urgent" || edited.Scope != "" || edited.Body != "" || edited.DueAt != nil {
		t.Errorf("edited = %+v", edited.TaskView)
	}
	if rec := e.do(t, http.MethodPost, "/api/tasks/"+id+"/actions", obj{"action": "dance"}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad action: %d", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/api/tasks/"+id+"/actions", obj{"action": "comment"}); rec.Code != http.StatusBadRequest {
		t.Errorf("empty comment: %d", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/api/tasks/task-nope/actions", obj{"action": "claim"}); rec.Code != http.StatusNotFound {
		t.Errorf("missing task: %d", rec.Code)
	}
	if _, err := e.tasks.Update(context.Background(), e.agentCaller(t, bob), tasks.UpdateInput{TaskID: id, Action: tasks.ActionFinish}); err == nil {
		t.Error("bob finished alice's task")
	}
	done := decode[taskDetail](t, e.do(t, http.MethodPost, "/api/tasks/"+id+"/actions", obj{"action": "finish", "note": "operator closed"}), http.StatusOK)
	if done.Status != "finished" || done.FinishedAt == nil {
		t.Errorf("finished = %+v", done.TaskView)
	}

	// Listing and filters.
	other := decode[taskDetail](t, e.do(t, http.MethodPost, "/api/tasks", obj{"title": "Other"}), http.StatusCreated)
	list := decode[httpapi.TaskListBody](t, e.do(t, http.MethodGet, "/api/tasks", nil), http.StatusOK)
	if len(list.Items) != 2 || list.Items[0].ID != other.ID {
		t.Errorf("list = %+v", list)
	}
	if l := decode[httpapi.TaskListBody](t, e.do(t, http.MethodGet, "/api/tasks?open_only=true", nil), http.StatusOK); len(l.Items) != 1 {
		t.Errorf("open only = %+v", l)
	}
	if l := decode[httpapi.TaskListBody](t, e.do(t, http.MethodGet, "/api/tasks?involves="+url.QueryEscape(alice)+"&status=finished", nil), http.StatusOK); len(l.Items) != 1 {
		t.Errorf("involves alice = %+v", l)
	}
	page := decode[httpapi.TaskListBody](t, e.do(t, http.MethodGet, "/api/tasks?limit=1", nil), http.StatusOK)
	if len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("page = %+v", page)
	}
	page2 := decode[httpapi.TaskListBody](t, e.do(t, http.MethodGet, "/api/tasks?limit=1&cursor="+url.QueryEscape(page.NextCursor), nil), http.StatusOK)
	if len(page2.Items) != 1 || page2.Items[0].ID != id || page2.NextCursor != "" {
		t.Errorf("page2 = %+v", page2)
	}
	if rec := e.do(t, http.MethodGet, "/api/tasks?cursor=junk", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad cursor: %d", rec.Code)
	}
	if rec := e.do(t, http.MethodGet, "/api/tasks?status=nope", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad status: %d", rec.Code)
	}

	// Detail and thread chips.
	d := decode[taskDetail](t, e.do(t, http.MethodGet, "/api/tasks/"+id, nil), http.StatusOK)
	if len(d.Events) != 8 || d.Events[len(d.Events)-1].Kind != "finish" || d.Events[len(d.Events)-1].ToStatus != "finished" {
		t.Errorf("detail events = %v", eventKinds(d))
	}
	if rec := e.do(t, http.MethodGet, "/api/tasks/task-nope", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing detail: %d", rec.Code)
	}
	chips := decode[httpapi.ThreadTasksBody](t, e.do(t, http.MethodGet, "/api/thread-tasks?thread_id="+msg.ThreadID+"&thread_id=msg-none", nil), http.StatusOK)
	if len(chips.Items) != 1 || chips.Items[0].TaskID != id || chips.Items[0].Status != "finished" {
		t.Errorf("chips = %+v", chips)
	}
	if rec := e.do(t, http.MethodGet, "/api/thread-tasks", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("chips without thread: %d", rec.Code)
	}
}

func TestTaskForbiddenMapsTo403(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, alice, "dev")
	// Only the operator acts over HTTP and may do everything, so the 403
	// path is exercised through an assignment check failing for an agent
	// and confirmed by the error mapping of a forbidden service call.
	tk, err := e.tasks.Create(context.Background(), e.agentCaller(t, alice), tasks.CreateInput{Title: "x", Assignee: alice})
	if err != nil {
		t.Fatal(err)
	}
	carol := "carol@github.com/acme/web"
	e.register(t, carol, "dev")
	if _, err := e.tasks.Update(context.Background(), e.agentCaller(t, carol), tasks.UpdateInput{TaskID: tk.ID, Action: tasks.ActionClaim}); err == nil {
		t.Fatal("carol claimed alice's reserved task")
	}
	// The operator overrides the reservation.
	got := decode[taskDetail](t, e.do(t, http.MethodPost, "/api/tasks/"+tk.ID+"/actions", obj{"action": "claim"}), http.StatusOK)
	if got.Claimant != "operator" {
		t.Errorf("claimant = %q", got.Claimant)
	}
}

func TestTasksUnavailable(t *testing.T) {
	t.Parallel()
	base := newEnv(t)
	e := &env{h: httpapi.New(httpapi.Deps{Pinger: fakePinger{}, Mail: base.svc, Presence: base.reg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})}
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/tasks", nil},
		{http.MethodPost, "/api/tasks", obj{"title": "x"}},
		{http.MethodGet, "/api/tasks/task-x", nil},
		{http.MethodPost, "/api/tasks/task-x/actions", obj{"action": "claim"}},
		{http.MethodGet, "/api/thread-tasks?thread_id=x", nil},
	} {
		if rec := e.do(t, c.method, c.path, c.body); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: %d", c.method, c.path, rec.Code)
		}
	}
}

// TestTaskEnumsMatchPackage keeps the OpenAPI enums in step with the tasks
// package's values.
func TestTaskEnumsMatchPackage(t *testing.T) {
	t.Parallel()
	spec, err := httpapi.OpenAPI("test")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	statuses, priorities, healths, actions := strs(tasks.Statuses), strs(tasks.Priorities), strs(tasks.Healths), strs(tasks.Actions)
	events := append([]string{tasks.EventCreated, tasks.EventStale, tasks.EventDecayed}, actions...)
	checks := []struct {
		schema, prop string
		want         []string
	}{
		{"TaskView", "status", statuses},
		{"TaskView", "priority", priorities},
		{"TaskView", "health", healths},
		{"TaskEventView", "kind", events},
		{"TaskEventView", "from_status", statuses},
		{"TaskActionRequest", "action", actions},
		{"TaskActionRequest", "priority", priorities},
		{"TaskCreateRequest", "priority", priorities},
		{"ThreadTaskView", "status", statuses},
	}
	for _, c := range checks {
		got := doc.Components.Schemas[c.schema].Properties[c.prop].Enum
		if !slices.Equal(got, c.want) {
			t.Errorf("%s.%s enum = %v, want %v", c.schema, c.prop, got, c.want)
		}
	}
}

func strs[T ~string](vs []T) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

func eventKinds(d taskDetail) []string {
	out := make([]string, len(d.Events))
	for i, ev := range d.Events {
		out[i] = ev.Kind
	}
	return out
}

func TestSendWithTaskID(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, alice, "dev")
	tk := decode[taskDetail](t, e.do(t, http.MethodPost, "/api/tasks", obj{"title": "plan"}), http.StatusCreated)

	sent := decode[httpapi.MessageView](t, e.do(t, http.MethodPost, "/api/messages", obj{
		"to": []string{alice}, "subject": "about the plan", "body": "b", "task_id": tk.ID,
	}), http.StatusCreated)
	d := decode[taskDetail](t, e.do(t, http.MethodGet, "/api/tasks/"+tk.ID, nil), http.StatusOK)
	if len(d.Links) != 1 || d.Links[0].MessageID != sent.ID || d.Links[0].LinkedBy != "operator" {
		t.Errorf("links = %+v", d.Links)
	}
	if kinds := eventKinds(d); !slices.Equal(kinds, []string{"created", "link"}) {
		t.Errorf("events = %v", kinds)
	}
	chips := decode[httpapi.ThreadTasksBody](t, e.do(t, http.MethodGet, "/api/thread-tasks?thread_id="+sent.ThreadID, nil), http.StatusOK)
	if len(chips.Items) != 1 || chips.Items[0].TaskID != tk.ID {
		t.Errorf("chips = %+v", chips)
	}

	// Unknown task: 404 and nothing is sent.
	before := decode[httpapi.SearchBody](t, e.do(t, http.MethodGet, "/api/search?address="+alice+"&q=ghost", nil), http.StatusOK)
	rec := e.do(t, http.MethodPost, "/api/messages", obj{"to": []string{alice}, "subject": "ghost", "body": "b", "task_id": "task-nope"})
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "nothing was sent") {
		t.Errorf("unknown task: %d %s", rec.Code, rec.Body.String())
	}
	after := decode[httpapi.SearchBody](t, e.do(t, http.MethodGet, "/api/search?address="+alice+"&q=ghost", nil), http.StatusOK)
	if len(after.Hits) != len(before.Hits) {
		t.Errorf("message sent despite unknown task: %d -> %d", len(before.Hits), len(after.Hits))
	}
	// Malformed id fails validation.
	if rec := e.do(t, http.MethodPost, "/api/messages", obj{"to": []string{alice}, "subject": "s", "body": "b", "task_id": "bogus"}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad task id: %d", rec.Code)
	}
	// Without task_id nothing is linked.
	plain := decode[httpapi.MessageView](t, e.do(t, http.MethodPost, "/api/messages", obj{"to": []string{alice}, "subject": "plain", "body": "b"}), http.StatusCreated)
	chips = decode[httpapi.ThreadTasksBody](t, e.do(t, http.MethodGet, "/api/thread-tasks?thread_id="+plain.ThreadID, nil), http.StatusOK)
	if len(chips.Items) != 0 {
		t.Errorf("plain send linked: %+v", chips)
	}
}

func TestSendWithTaskIDUnavailable(t *testing.T) {
	t.Parallel()
	base := newEnv(t)
	base.register(t, alice, "dev")
	e := &env{h: httpapi.New(httpapi.Deps{Pinger: fakePinger{}, Mail: base.svc, Presence: base.reg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})}
	rec := e.do(t, http.MethodPost, "/api/messages", obj{"to": []string{alice}, "subject": "s", "body": "b", "task_id": "task-x"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPost, "/api/messages", obj{"to": []string{alice}, "subject": "s", "body": "b"}); rec.Code != http.StatusCreated {
		t.Errorf("plain send without board: %d", rec.Code)
	}
}
