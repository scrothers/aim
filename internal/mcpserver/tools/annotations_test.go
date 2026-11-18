package tools_test

import (
	"context"
	"testing"
)

func TestToolAnnotations(t *testing.T) {
	t.Parallel()
	type want struct {
		title      string
		readOnly   bool
		idempotent bool
	}
	wants := map[string]want{
		"agent_whoami":   {"Who am I", true, false},
		"agent_register": {"Register identity", false, true},
		"agent_roster":   {"Roster", true, false},
		"agent_sign_on":  {"Sign on", false, true},
		"agent_sign_off": {"Sign off", false, true},
		"mail_send":      {"Send message", false, false},
		"mail_list":      {"List mail", true, false},
		"mail_read":      {"Read message", false, true},
		"mail_thread":    {"Read thread", true, false},
		"mail_mark":      {"Mark messages", false, true},
		"mail_search":    {"Search mail", true, false},
		"mail_wait":      {"Wait for mail", true, false},
		// Task tools.
		"task_create": {"Create task", false, false},
		"task_list":   {"List tasks", true, false},
		"task_get":    {"Get task", true, false},
		"task_update": {"Update task", false, false},
	}
	cs := newEnv(t).Client(t, "a")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != len(wants) {
		t.Fatalf("tools = %d, want %d", len(res.Tools), len(wants))
	}
	for _, tool := range res.Tools {
		w, found := wants[tool.Name]
		if !found {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		a := tool.Annotations
		if a == nil {
			t.Errorf("%s: no annotations", tool.Name)
			continue
		}
		if tool.Title != w.title || a.Title != w.title {
			t.Errorf("%s: title = %q / %q, want %q", tool.Name, tool.Title, a.Title, w.title)
		}
		if a.ReadOnlyHint != w.readOnly {
			t.Errorf("%s: readOnly = %v, want %v", tool.Name, a.ReadOnlyHint, w.readOnly)
		}
		if a.IdempotentHint != w.idempotent {
			t.Errorf("%s: idempotent = %v, want %v", tool.Name, a.IdempotentHint, w.idempotent)
		}
		if a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s: destructive = %v, want explicit false", tool.Name, a.DestructiveHint)
		}
		if a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("%s: openWorld = %v, want explicit false", tool.Name, a.OpenWorldHint)
		}
	}
}
