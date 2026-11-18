package mcptest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/tasks"
	"github.com/scrothers/aim/internal/wire"
)

// Env is a mail service over a fresh database plus a presence registry, from
// which tests connect in-memory MCP clients.
type Env struct {
	Svc *mail.Service
	Reg *presence.Registry
	// Tasks is the task board every client's server is given.
	Tasks *tasks.Service
	// NoNamespace makes later clients connect from a directory without a
	// git remote.
	NoNamespace bool
	// Protocol pins the MCP protocol version later clients negotiate; empty
	// means the SDK's latest.
	Protocol string
}

// New returns an Env whose database lives in t.TempDir().
func New(t *testing.T) *Env {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir()+"/aim.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	reg := presence.New()
	svc := mail.New(st, mcpserver.NewPresence(reg))
	return &Env{Svc: svc, Reg: reg, Tasks: tasks.New(st, svc, tasks.WithSessions(reg))}
}

// Client connects a client as session "sess-<id>" on machine "box" in the
// namespace github.com/acme/api.
func (e *Env) Client(t *testing.T, id string) *mcp.ClientSession {
	t.Helper()
	return e.ClientWith(t, id, nil, nil)
}

// ClientWith is Client with server options and client options.
func (e *Env) ClientWith(t *testing.T, id string, opts []mcpserver.Option, copts *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	pre := wire.Preamble{
		V: wire.Version, SessionID: "sess-" + id, ProjectDir: "/p", Machine: "box",
		GitRemote: "git@github.com:acme/api.git", Namespace: "github.com/acme/api",
	}
	if e.NoNamespace {
		pre.GitRemote, pre.Namespace = "", ""
	}
	entry := e.Reg.Add("conn-"+id, pre, time.Now())
	t.Cleanup(func() { e.Reg.Remove("conn-" + id) })
	ct, st := mcp.NewInMemoryTransports()
	opts = append([]mcpserver.Option{mcpserver.WithTasks(e.Tasks)}, opts...)
	ss, err := mcpserver.New(e.Svc, entry, "test", opts...).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, copts).Connect(ctx, ct, &mcp.ClientSessionOptions{ProtocolVersion: e.Protocol})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close(); _ = ss.Wait() })
	return cs
}

// Call calls a tool and fails the test on a protocol error.
func Call(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

// OK calls a tool that must succeed and returns its structured content.
func OK(t *testing.T, cs *mcp.ClientSession, name string, args any) map[string]any {
	t.Helper()
	res := Call(t, cs, name, args)
	if res.IsError {
		t.Fatalf("%s: tool error: %s", name, Text(res))
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

// Fail calls a tool that must return a tool error containing want.
func Fail(t *testing.T, cs *mcp.ClientSession, name string, args any, want string) {
	t.Helper()
	res := Call(t, cs, name, args)
	if !res.IsError {
		t.Fatalf("%s: expected tool error", name)
	}
	if got := Text(res); !strings.Contains(got, want) {
		t.Fatalf("%s: error %q does not contain %q", name, got, want)
	}
}

// Text joins a tool result's text content.
func Text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// Register registers addr with role through the register tool.
func Register(t *testing.T, cs *mcp.ClientSession, addr, role string) {
	t.Helper()
	OK(t, cs, "register", map[string]any{"address": addr, "role": role, "description": "d"})
}

// Identities used by TwoAgents.
const (
	Alice = "alice@github.com/acme/api"
	Bob   = "bob@github.com/acme/api"
)

// TwoAgents registers and signs on Alice (session "a") and Bob (session "b")
// and has Alice mail Bob "deploy plan".
func (e *Env) TwoAgents(t *testing.T) (a, b *mcp.ClientSession, msgID, threadID string) {
	t.Helper()
	a, b = e.Client(t, "a"), e.Client(t, "b")
	Register(t, a, Alice, "lead")
	Register(t, b, Bob, "dev")
	OK(t, a, "sign_on", map[string]any{"address": Alice})
	OK(t, b, "sign_on", map[string]any{"address": Bob})
	sent := OK(t, a, "send", map[string]any{"to": []string{Bob}, "subject": "deploy plan", "body": "ship the zebra rollout"})
	return a, b, sent["id"].(string), sent["thread_id"].(string)
}

// ReadResource reads a single-content Markdown resource and returns its text.
func ReadResource(t *testing.T, cs *mcp.ClientSession, uri string) string {
	t.Helper()
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("read %s: %v", uri, err)
	}
	if len(res.Contents) != 1 || res.Contents[0].MIMEType != "text/markdown" || res.Contents[0].URI != uri {
		t.Fatalf("read %s: contents = %+v", uri, res.Contents)
	}
	return res.Contents[0].Text
}

// MustContain reports each of wants missing from got.
func MustContain(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, got)
		}
	}
}
