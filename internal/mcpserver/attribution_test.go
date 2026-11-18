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
	ok(t, cs, "agent_sign_on", map[string]any{"address": "alice@github.com/acme/api"})

	sendAs := func(meta mcp.Meta, args map[string]any) string {
		t.Helper()
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Meta: meta, Name: "mail_send", Arguments: args})
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
	ok(t, cs, "agent_whoami", map[string]any{})

	// mail_read and mail_mark are attributed too: mail delivered only to the
	// subagent's mailbox is read and marked when the call comes from it.
	call := func(meta mcp.Meta, name string, args map[string]any) map[string]any {
		t.Helper()
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Meta: meta, Name: name, Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %s", name, err, text(res))
		}
		m, _ := res.StructuredContent.(map[string]any)
		return m
	}
	toSub := base()
	toSub["to"] = []string{"helper@alice@github.com/acme/api"}
	id, _ := call(nil, "mail_send", toSub)["id"].(string)
	if r := call(mcp.Meta{"claudecode/toolUseId": "tu-main"}, "mail_read", map[string]any{"message_id": id}); r["marked"] != false {
		t.Errorf("main-agent read marked the subagent's copy: %v", r["marked"])
	}
	if r := call(mcp.Meta{"claudecode/toolUseId": "tu-sub"}, "mail_read", map[string]any{"message_id": id}); r["marked"] != true {
		t.Errorf("attributed read did not mark the subagent's copy: %v", r["marked"])
	}
	unread := map[string]any{"message_ids": []string{id}, "state": "unread"}
	if r := call(mcp.Meta{"claudecode/toolUseId": "tu-main"}, "mail_mark", unread); r["changed"] != float64(0) {
		t.Errorf("main-agent mark changed the subagent's copy: %v", r["changed"])
	}
	if r := call(mcp.Meta{"claudecode/toolUseId": "tu-sub"}, "mail_mark", unread); r["changed"] != float64(1) {
		t.Errorf("attributed mark changed = %v, want 1", r["changed"])
	}
}
