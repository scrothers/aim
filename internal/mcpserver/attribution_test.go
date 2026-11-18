package mcpserver_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver"
)

func TestSubagentAttributionFromToolUseID(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	resolver := func(id string) (string, bool) { return "helper", id == "tu-sub" }
	cs := e.ClientWith(t, "attr", []mcpserver.Option{mcpserver.WithSubagentResolver(resolver)}, nil)
	register(t, cs, "alice@github.com/acme/api", "lead")
	ok(t, cs, "sign_on", map[string]any{"address": "alice@github.com/acme/api"})

	sendAs := func(meta mcp.Meta, args map[string]any) string {
		t.Helper()
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Meta: meta, Name: "send", Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("send: %v %s", err, text(res))
		}
		m, _ := res.StructuredContent.(map[string]any)
		from, _ := m["from"].(string)
		return from
	}
	base := func() map[string]any {
		return map[string]any{"to": []string{"alice@github.com/acme/api"}, "subject": "s", "body": "b"}
	}

	if got := sendAs(mcp.Meta{"claudecode/toolUseId": "tu-sub"}, base()); got != "helper@alice@github.com/acme/api" {
		t.Errorf("attributed from = %q", got)
	}
	if got := sendAs(mcp.Meta{"claudecode/toolUseId": "tu-main"}, base()); got != "alice@github.com/acme/api" {
		t.Errorf("main-agent from = %q", got)
	}
	explicit := base()
	explicit["subagent"] = "chosen"
	if got := sendAs(mcp.Meta{"claudecode/toolUseId": "tu-sub"}, explicit); got != "chosen@alice@github.com/acme/api" {
		t.Errorf("explicit subagent overridden: from = %q", got)
	}
	// Tools without a subagent argument are left alone.
	ok(t, cs, "whoami", map[string]any{})
}
