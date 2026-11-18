package resources_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/mcpserver/resources"
	"github.com/scrothers/aim/internal/wire"
)

func TestTaskResources(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a, b, msgID, _ := twoAgents(t, e)
	held, _ := ok(t, b, "task_create", map[string]any{"title": "Held work", "priority": "high"})["id"].(string)
	ok(t, b, "task_update", map[string]any{"task_id": held, "action": "start", "note": "going"})
	assigned, _ := ok(t, a, "task_create", map[string]any{"title": "For bob", "assignee": bob, "message_id": msgID})["id"].(string)
	ok(t, a, "task_create", map[string]any{"title": "Up for grabs", "scope": "github.com/acme/api"})

	digest := readRes(t, b, resources.URITasks)
	mustContain(t, "bob's digest", digest,
		"# Your tasks", "## Held by you", "**Held work** `"+held+"` - started (active), high priority, held by "+bob,
		"## Assigned to you", "**For bob** `"+assigned+"`", "## Open in github.com/acme/api", "**Up for grabs**")
	if strings.Contains(digest, "Not signed on") {
		t.Error("signed-on digest shows sign-on hint")
	}
	empty := readRes(t, a, resources.URITasks)
	mustContain(t, "alice's digest", empty, "_You hold no tasks._", "_Nothing is assigned to you._")

	detail := readRes(t, b, resources.PrefixTask+assigned)
	mustContain(t, "detail", detail, "# For bob", "- id: "+assigned, "- status: not_started", "- assignee: "+bob,
		"## History", "created by "+alice+" (-> not_started)", "assign by "+alice+": "+bob,
		"## Linked messages", "**deploy plan** (id `"+msgID+"`")
	heldDetail := readRes(t, b, resources.PrefixTask+held)
	mustContain(t, "held detail", heldDetail, "- status: started (active)", "start by "+bob+" (not_started -> started): going",
		"- last activity: ", "_None. Link one")

	if _, err := b.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: resources.PrefixTask + "task-nope"}); err == nil {
		t.Error("unknown task read succeeded")
	}

	anon := e.Client(t, "anon")
	mustContain(t, "anonymous digest", readRes(t, anon, resources.URITasks), "## Open in github.com/acme/api", "Not signed on")
}

func TestTaskResourcesWithoutBoard(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	entry := e.Reg.Add("conn-x", wire.Preamble{SessionID: "sess-x"}, time.Now())
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
	for _, uri := range []string{resources.URITasks, resources.PrefixTask + "task-x"} {
		if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); err == nil || !strings.Contains(err.Error(), "not available") {
			t.Errorf("read %s err = %v", uri, err)
		}
	}
}
