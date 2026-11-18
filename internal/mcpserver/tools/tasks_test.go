package tools_test

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/mcpserver/mcptest"
	"github.com/scrothers/aim/internal/wire"
)

func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, _ := m["items"].([]any)
	out := make([]map[string]any, len(raw))
	for i, r := range raw {
		out[i], _ = r.(map[string]any)
	}
	return out
}

func TestTaskToolsFlow(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a, b, msgID, threadID := e.TwoAgents(t)

	created := ok(t, a, "task_create", map[string]any{
		"title": "Ship zebra", "body": "details", "priority": "high", "scope": "github.com/acme/api",
		"assignee": mcptest.Bob, "due_at": "2026-12-01T00:00:00Z", "message_id": msgID,
	})
	id, _ := created["id"].(string)
	if created["status"] != "not_started" || created["assignee"] != mcptest.Bob || created["created_by"] != mcptest.Alice {
		t.Fatalf("created = %v", created)
	}
	fail(t, a, "task_create", map[string]any{"title": "x", "due_at": "tomorrow"}, "due_at must be")
	fail(t, a, "task_create", map[string]any{"title": "x", "priority": "meh"}, "priority")
	fail(t, a, "task_update", map[string]any{"task_id": id, "action": "claim"}, "assigned to "+mcptest.Bob)

	// Bob's default list shows the task assigned to him.
	if got := items(t, ok(t, b, "task_list", map[string]any{})); len(got) != 1 || got[0]["id"] != id {
		t.Fatalf("bob default list = %v", got)
	}
	got := ok(t, b, "task_update", map[string]any{"task_id": id, "action": "start", "note": "on it"})
	if got["status"] != "started" || got["claimant"] != mcptest.Bob || got["health"] != "active" {
		t.Fatalf("started = %v", got)
	}
	ok(t, b, "task_update", map[string]any{"task_id": id, "action": "comment", "note": "halfway"})
	fail(t, b, "task_update", map[string]any{"task_id": id, "action": "comment"}, "needs a note")
	edited := ok(t, b, "task_update", map[string]any{
		"task_id": id, "action": "edit", "title": "Ship zebra v2", "body": "b2", "priority": "urgent",
		"scope": "github.com/acme", "due_at": "2026-12-02T00:00:00Z",
	})
	if edited["title"] != "Ship zebra v2" || edited["scope"] != "github.com/acme" || edited["priority"] != "urgent" {
		t.Errorf("edited = %v", edited)
	}
	cleared := ok(t, b, "task_update", map[string]any{"task_id": id, "action": "edit", "clear_scope": true, "clear_due": true})
	if _, has := cleared["scope"]; has {
		t.Errorf("scope not cleared: %v", cleared)
	}
	if _, has := cleared["due_at"]; has {
		t.Errorf("due not cleared: %v", cleared)
	}
	fail(t, b, "task_update", map[string]any{"task_id": id, "action": "edit", "due_at": "soon"}, "due_at must be")

	// send with task_id links the message.
	sent := ok(t, b, "mail_send", map[string]any{"to": []string{mcptest.Alice}, "subject": "done soon", "body": "b", "task_id": id})
	fail(t, b, "mail_send", map[string]any{"to": []string{mcptest.Alice}, "subject": "s", "body": "b", "task_id": "task-nope"}, "nothing was sent")

	detail := ok(t, a, "task_get", map[string]any{"task_id": id})
	links, _ := detail["links"].([]any)
	if events, _ := detail["events"].([]any); len(events) != 9 {
		t.Errorf("events = %v", events)
	}
	linked := map[string]bool{}
	for _, l := range links {
		lm, _ := l.(map[string]any)
		linked[lm["message_id"].(string)] = true
	}
	if !linked[msgID] || !linked[sent["id"].(string)] {
		t.Errorf("links = %v (want %s and %s)", links, msgID, sent["id"])
	}
	fail(t, a, "task_get", map[string]any{"task_id": "task-nope"}, "task not found")

	ok(t, b, "task_update", map[string]any{"task_id": id, "action": "finish"})
	if got := items(t, ok(t, a, "task_list", map[string]any{"involves": mcptest.Alice})); len(got) != 0 {
		t.Errorf("open list after finish = %v", got)
	}
	if got := items(t, ok(t, a, "task_list", map[string]any{"involves": mcptest.Alice, "include_finished": true, "limit": 1})); len(got) != 1 {
		t.Errorf("finished list = %v", got)
	}
	fail(t, a, "task_list", map[string]any{"scope": mcptest.Alice}, "not an org or repo address")

	// Thread chips see the linked thread.
	chips, err := e.Tasks.ForThreads(context.Background(), []string{threadID})
	if err != nil || len(chips) != 1 {
		t.Errorf("chips = %v, %v", chips, err)
	}
}

func TestTaskListDefaults(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a, b, _, _ := e.TwoAgents(t)
	mine := ok(t, a, "task_create", map[string]any{"title": "mine"})["id"]
	open := ok(t, b, "task_create", map[string]any{"title": "open", "scope": "github.com/acme/api"})["id"]
	ok(t, b, "task_create", map[string]any{"title": "elsewhere", "scope": "github.com/other"})
	taken := ok(t, b, "task_create", map[string]any{"title": "taken", "scope": "github.com/acme/api"})["id"]
	ok(t, b, "task_update", map[string]any{"task_id": taken, "action": "claim"})

	got := items(t, ok(t, a, "task_list", map[string]any{}))
	if len(got) != 2 || got[0]["id"] != mine || got[1]["id"] != open {
		t.Errorf("alice default = %v", got)
	}
	if got := items(t, ok(t, a, "task_list", map[string]any{"limit": 1})); len(got) != 1 {
		t.Errorf("limited default = %v", got)
	}

	// A session that is not signed on and has no namespace sees recent open
	// tasks everywhere.
	e.NoNamespace = true
	anon := e.Client(t, "anon")
	if got := items(t, ok(t, anon, "task_list", map[string]any{})); len(got) != 4 {
		t.Errorf("anonymous default = %d items", len(got))
	}
	fail(t, anon, "task_create", map[string]any{"title": "x"}, "sign_on")
}

func TestTaskToolsWithoutBoard(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	entry := e.Reg.Add("conn-x", wire.Preamble{SessionID: "sess-x", Machine: "box"}, time.Now())
	t.Cleanup(func() { e.Reg.Remove("conn-x") })
	ct, st := mcp.NewInMemoryTransports()
	ss, err := mcpserver.New(e.Svc, entry, "test").Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close(); _ = ss.Wait() })
	for name, args := range map[string]map[string]any{
		"task_create": {"title": "x"},
		"task_list":   {},
		"task_get":    {"task_id": "task-x"},
		"task_update": {"task_id": "task-x", "action": "claim"},
		"mail_send":   {"to": []string{"operator"}, "subject": "s", "body": "b", "task_id": "task-x"},
	} {
		fail(t, cs, name, args, "task board is not available")
	}
}
