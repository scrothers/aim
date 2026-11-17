package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/wire"
)

type env struct {
	svc *mail.Service
	reg *presence.Registry
	// noNamespace makes later clients connect from a directory without a
	// git remote.
	noNamespace bool
	// protocol pins the MCP protocol version the next clients negotiate;
	// empty means the SDK's latest.
	protocol string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/aim.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	reg := presence.New()
	return &env{svc: mail.New(st, mcpserver.NewPresence(reg)), reg: reg}
}

func (e *env) client(t *testing.T, id string) *mcp.ClientSession {
	t.Helper()
	return e.clientWith(t, id, nil, nil)
}

func (e *env) clientWith(t *testing.T, id string, opts []mcpserver.Option, copts *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	pre := wire.Preamble{
		V: wire.Version, SessionID: "sess-" + id, ProjectDir: "/p", Machine: "box",
		GitRemote: "git@github.com:acme/api.git", Namespace: "github.com/acme/api",
	}
	if e.noNamespace {
		pre.GitRemote, pre.Namespace = "", ""
	}
	entry := e.reg.Add("conn-"+id, pre, time.Now())
	t.Cleanup(func() { e.reg.Remove("conn-" + id) })
	ct, st := mcp.NewInMemoryTransports()
	ss, err := mcpserver.New(e.svc, entry, "test", opts...).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, copts).Connect(ctx, ct, &mcp.ClientSessionOptions{ProtocolVersion: e.protocol})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close(); _ = ss.Wait() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func ok(t *testing.T, cs *mcp.ClientSession, name string, args any) map[string]any {
	t.Helper()
	res := call(t, cs, name, args)
	if res.IsError {
		t.Fatalf("%s: tool error: %s", name, text(res))
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func fail(t *testing.T, cs *mcp.ClientSession, name string, args any, want string) {
	t.Helper()
	res := call(t, cs, name, args)
	if !res.IsError {
		t.Fatalf("%s: expected tool error", name)
	}
	if got := text(res); !strings.Contains(got, want) {
		t.Fatalf("%s: error %q does not contain %q", name, got, want)
	}
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func register(t *testing.T, cs *mcp.ClientSession, addr, role string) {
	t.Helper()
	ok(t, cs, "register", map[string]any{"address": addr, "role": role, "description": "d"})
}

func TestToolList(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).client(t, "a")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 12 {
		t.Fatalf("tools = %d, want 12", len(res.Tools))
	}
	registered := map[string]bool{}
	for _, tool := range res.Tools {
		registered[tool.Name] = true
		// Type lists like ["null","array"] are legal JSON Schema but some
		// clients reject them; schema.go rewrites them at registration.
		for kind, schema := range map[string]any{"input": tool.InputSchema, "output": tool.OutputSchema} {
			b, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), `"type":[`) {
				t.Errorf("%s %s schema has a type list: %s", tool.Name, kind, b)
			}
		}
		want := promptFile(t, "prompts/tools/"+tool.Name+".md")
		if want == "" {
			t.Errorf("prompts/tools/%s.md is empty", tool.Name)
		}
		if tool.Description != want {
			t.Errorf("%s description = %q, want the contents of prompts/tools/%s.md", tool.Name, tool.Description, tool.Name)
		}
	}
	// Every prompt file must belong to a registered tool, so a renamed or
	// removed tool can't leave a stale description behind.
	entries, err := os.ReadDir("prompts/tools")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if e.IsDir() || name == e.Name() || !registered[name] {
			t.Errorf("prompts/tools/%s has no matching registered tool", e.Name())
		}
	}
	if len(entries) != len(res.Tools) {
		t.Errorf("prompt files = %d, tools = %d", len(entries), len(res.Tools))
	}
}

func TestInstructionsEmbedded(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).client(t, "a")
	got := cs.InitializeResult().Instructions
	if got == "" || got != promptFile(t, "prompts/instructions.md") {
		t.Fatalf("initialize instructions = %q, want prompts/instructions.md", got)
	}
	if !strings.Contains(got, "by sending to `operator`") {
		t.Errorf("instructions lost the operator escalation guidance")
	}
}

// promptFile reads an embedded prompt's source file, trimmed the same way the
// server trims it.
func promptFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(b))
}

func TestWhoamiAndSignOn(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).client(t, "a")
	w := ok(t, cs, "whoami", map[string]any{})
	if w["namespace"] != "github.com/acme/api" || w["session_id"] != "sess-a" || w["signed_on_as"] != nil {
		t.Fatalf("whoami = %v", w)
	}
	fail(t, cs, "sign_on", map[string]any{"address": "bob@github.com/acme/api"}, "does not exist")
	fail(t, cs, "sign_on", map[string]any{"address": "github.com/acme/api"}, "identity")
	register(t, cs, "bob@github.com/acme/api", "dev")
	s := ok(t, cs, "sign_on", map[string]any{"address": "bob@github.com/acme/api", "away": "lunch"})
	if s["signed_on_as"] != "bob@github.com/acme/api" {
		t.Fatalf("sign_on = %v", s)
	}
	if w := ok(t, cs, "whoami", map[string]any{}); w["signed_on_as"] != "bob@github.com/acme/api" {
		t.Fatalf("whoami = %v", w)
	}
	r := ok(t, cs, "roster", map[string]any{"address": "github.com/acme"})
	ids := r["identities"].([]any)
	if len(ids) != 1 {
		t.Fatalf("roster = %v", r)
	}
	online := ids[0].(map[string]any)["online"].([]any)
	if len(online) != 1 || online[0].(map[string]any)["away"] != "lunch" {
		t.Fatalf("online = %v", online)
	}
	if o := ok(t, cs, "sign_off", map[string]any{}); o["was_signed_on"] != true {
		t.Fatalf("sign_off = %v", o)
	}
	if o := ok(t, cs, "sign_off", map[string]any{}); o["was_signed_on"] != false {
		t.Fatalf("sign_off = %v", o)
	}
}

func TestRegisterErrors(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).client(t, "a")
	register(t, cs, "bob@github.com/acme/api", "dev")
	fail(t, cs, "register", map[string]any{"address": "bob@github.com/acme/api", "role": "x", "description": "d"}, "already exists")
	fail(t, cs, "register", map[string]any{"address": "!!bad", "role": "x", "description": "d"}, "address grammar")
	fail(t, cs, "roster", map[string]any{"address": "nonsense"}, "address grammar")
}

func TestMailFlow(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a, b := e.client(t, "a"), e.client(t, "b")
	register(t, a, "alice@github.com/acme/api", "lead")
	register(t, b, "bob@github.com/acme/api", "dev")

	fail(t, a, "send", map[string]any{"to": []string{"bob@github.com/acme/api"}, "subject": "s", "body": "b"}, "sign_on")
	fail(t, a, "mark", map[string]any{"message_ids": []string{"msg-x"}, "state": "read"}, "sign_on")
	ok(t, a, "sign_on", map[string]any{"address": "alice@github.com/acme/api"})
	ok(t, b, "sign_on", map[string]any{"address": "bob@github.com/acme/api"})

	fail(t, a, "send", map[string]any{"to": []string{"bobb@github.com/acme/api"}, "subject": "s", "body": "b"}, "bob@github.com/acme/api")

	sent := ok(t, a, "send", map[string]any{
		"to": []string{"bob@github.com/acme/api"}, "cc": []string{"github.com/acme/api"},
		"subject": "deploy plan", "body": "ship the zebra rollout",
	})
	id, thread := sent["id"].(string), sent["thread_id"].(string)

	l := ok(t, b, "list", map[string]any{"address": "bob@github.com/acme/api"})
	if items := l["items"].([]any); len(items) != 1 {
		t.Fatalf("list = %v", l)
	}
	if p := ok(t, b, "read", map[string]any{"message_id": id, "peek": true}); p["marked"] != false {
		t.Fatalf("peek marked = %v", p["marked"])
	}
	if l := ok(t, b, "list", map[string]any{"address": "github.com/acme"}); len(l["items"].([]any)) == 0 {
		t.Fatalf("peek should not mark read: %v", l)
	}
	m := ok(t, b, "read", map[string]any{"message_id": id})
	if m["subject"] != "deploy plan" || m["marked"] != true {
		t.Fatalf("read = %v", m)
	}
	// A caller who has not signed on reads fine but nothing is marked.
	anon := e.client(t, "anon")
	if r := ok(t, anon, "read", map[string]any{"message_id": id}); r["subject"] != "deploy plan" || r["marked"] != false {
		t.Fatalf("unsigned read = %v", r)
	}
	if l := ok(t, b, "list", map[string]any{"address": "bob@github.com/acme/api"}); len(l["items"].([]any)) != 0 {
		t.Fatalf("expected no unread: %v", l)
	}
	if l := ok(t, b, "list", map[string]any{"address": "bob@github.com/acme/api", "state": "read"}); len(l["items"].([]any)) != 1 {
		t.Fatalf("read list = %v", l)
	}

	ok(t, b, "send", map[string]any{
		"to": []string{"alice@github.com/acme/api"}, "subject": "re", "body": "ack", "in_reply_to": id, "reply_all": true,
	})
	th := ok(t, a, "thread", map[string]any{"thread_id": thread})
	if len(th["messages"].([]any)) != 2 {
		t.Fatalf("thread = %v", th)
	}

	if n := ok(t, b, "mark", map[string]any{"message_ids": []string{id}, "state": "archived"}); n["changed"].(float64) != 1 {
		t.Fatalf("mark = %v", n)
	}
	now := time.Now().UTC()
	arch := ok(t, b, "list", map[string]any{
		"address": "bob@github.com/acme/api", "state": "archived",
		"since": now.Add(-time.Hour).Format(time.RFC3339), "until": now.Add(time.Hour).Format(time.RFC3339),
	})
	if len(arch["items"].([]any)) != 1 {
		t.Fatalf("archived = %v", arch)
	}
	fail(t, b, "list", map[string]any{"address": "bob@github.com/acme/api", "state": "archived"}, "since")
	fail(t, b, "list", map[string]any{"address": "bob@github.com/acme/api", "since": "yesterday"}, "RFC3339")
	fail(t, b, "list", map[string]any{"address": "bob@github.com/acme/api", "until": "tomorrow"}, "RFC3339")

	s := ok(t, a, "search", map[string]any{"address": "github.com/acme", "query": "zebra"})
	if len(s["hits"].([]any)) != 1 {
		t.Fatalf("search = %v", s)
	}
	if s := ok(t, a, "search", map[string]any{"address": "github.com/acme", "query": "nomatch"}); len(s["hits"].([]any)) != 0 {
		t.Fatalf("search = %v", s)
	}
	fail(t, a, "read", map[string]any{"message_id": "msg-nope"}, "not found")
	fail(t, a, "thread", map[string]any{"thread_id": "nope"}, "")
}

func TestClientInfoCaptured(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cs := e.client(t, "a")
	if st := e.reg.All()[0]; st.Client != "" {
		t.Fatalf("client before first call = %q", st.Client)
	}
	w := ok(t, cs, "whoami", map[string]any{})
	if w["client"] != "t" || w["client_version"] != "0" {
		t.Fatalf("whoami = %v", w)
	}
	if st := e.reg.All()[0]; st.Client != "t" || st.ClientVersion != "0" {
		t.Errorf("status = %+v", st)
	}
	register(t, cs, "bob@github.com/acme/api", "dev")
	ok(t, cs, "sign_on", map[string]any{"address": "bob@github.com/acme/api"})
	m := ok(t, cs, "send", map[string]any{"to": []string{"operator"}, "subject": "help", "body": "need a decision"})
	if m["from_client"] != "t" || m["from_machine"] != "box" {
		t.Errorf("send = %v", m)
	}
	l := ok(t, cs, "list", map[string]any{"address": "operator"})
	if items, _ := l["items"].([]any); len(items) != 1 {
		t.Errorf("operator inbox = %v", l)
	}
}

func TestOperatorReservedAndAddressRequired(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).client(t, "a")
	fail(t, cs, "sign_on", map[string]any{"address": "operator"}, "operator is reserved for the human operator; agents escalate by sending to operator")
	fail(t, cs, "register", map[string]any{"address": "operator", "role": "x", "description": "d"}, "reserved")
	fail(t, cs, "list", map[string]any{"address": "  "}, "address is required")
	fail(t, cs, "roster", map[string]any{"address": ""}, "address is required")
	fail(t, cs, "search", map[string]any{"address": " ", "query": "x"}, "address is required")
}
