package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/mcpserver/mcptest"
)

// Short names for the shared test helpers.
var (
	newEnv   = mcptest.New
	ok       = mcptest.OK
	fail     = mcptest.Fail
	register = mcptest.Register
)

func TestToolList(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
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
		want := descriptionFile(t, "markdown/"+tool.Name+".md")
		if want == "" {
			t.Errorf("markdown/%s.md is empty", tool.Name)
		}
		if tool.Description != want {
			t.Errorf("%s description = %q, want the contents of markdown/%s.md", tool.Name, tool.Description, tool.Name)
		}
	}
	// Every description file must belong to a registered tool, so a renamed
	// or removed tool can't leave a stale description behind.
	entries, err := os.ReadDir("markdown")
	if err != nil {
		t.Fatal(err)
	}
	descriptions := 0
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if e.IsDir() || !ok {
			continue
		}
		descriptions++
		if !registered[name] {
			t.Errorf("markdown/%s has no matching registered tool", e.Name())
		}
	}
	if descriptions != len(res.Tools) {
		t.Errorf("description files = %d, tools = %d", descriptions, len(res.Tools))
	}
}

// descriptionFile reads a description's source file, trimmed the same way
// the server trims it.
func descriptionFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(b))
}

func TestWhoamiAndSignOn(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
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
	cs := newEnv(t).Client(t, "a")
	register(t, cs, "bob@github.com/acme/api", "dev")
	fail(t, cs, "register", map[string]any{"address": "bob@github.com/acme/api", "role": "x", "description": "d"}, "already exists")
	fail(t, cs, "register", map[string]any{"address": "!!bad", "role": "x", "description": "d"}, "address grammar")
	fail(t, cs, "roster", map[string]any{"address": "nonsense"}, "address grammar")
}

func TestMailFlow(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a, b := e.Client(t, "a"), e.Client(t, "b")
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
	anon := e.Client(t, "anon")
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

func TestOperatorReservedAndAddressRequired(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	fail(t, cs, "sign_on", map[string]any{"address": "operator"}, "operator is reserved for the human operator; agents escalate by sending to operator")
	fail(t, cs, "register", map[string]any{"address": "operator", "role": "x", "description": "d"}, "reserved")
	fail(t, cs, "list", map[string]any{"address": "  "}, "address is required")
	fail(t, cs, "roster", map[string]any{"address": ""}, "address is required")
	fail(t, cs, "search", map[string]any{"address": " ", "query": "x"}, "address is required")
}
