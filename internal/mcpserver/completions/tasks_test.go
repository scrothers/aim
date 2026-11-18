package completions_test

import (
	"slices"
	"testing"

	"github.com/scrothers/aim/internal/mcpserver/resources"
)

func TestCompleteTaskIDs(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a, b, _, _ := twoAgents(t, e)
	ref := resourceRef(resources.PrefixTask + "{task_id}")
	mine, _ := ok(t, a, "task_create", map[string]any{"title": "mine"})["id"].(string)
	scoped, _ := ok(t, b, "task_create", map[string]any{"title": "scoped", "scope": "github.com/acme/api"})["id"].(string)
	ok(t, b, "task_create", map[string]any{"title": "elsewhere", "scope": "github.com/other"})
	done, _ := ok(t, a, "task_create", map[string]any{"title": "done"})["id"].(string)
	ok(t, a, "task_update", map[string]any{"task_id": done, "action": "start"})
	ok(t, a, "task_update", map[string]any{"task_id": done, "action": "finish"})

	got := complete(t, a, ref, "task_id", "")
	if !slices.Equal(got.Values, []string{mine, scoped}) {
		t.Errorf("alice task ids = %v, want [%s %s]", got.Values, mine, scoped)
	}
	if got := complete(t, a, ref, "task_id", "task-zzz"); len(got.Values) != 0 {
		t.Errorf("non-matching prefix = %v", got.Values)
	}
	e.NoNamespace = true
	if got := complete(t, e.Client(t, "anon"), ref, "task_id", ""); len(got.Values) != 3 {
		t.Errorf("anonymous task ids = %v, want 3 open", got.Values)
	}
}
