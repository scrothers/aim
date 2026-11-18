package prompts_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver/mcptest"
)

const bob = mcptest.Bob

// Short names for the shared test helpers.
var (
	newEnv      = mcptest.New
	ok          = mcptest.OK
	mustContain = mcptest.MustContain
)

func twoAgents(t *testing.T, e *mcptest.Env) (a, b *mcp.ClientSession, msgID, threadID string) {
	t.Helper()
	return e.TwoAgents(t)
}

func getPrompt(t *testing.T, cs *mcp.ClientSession, name string, args map[string]string) *mcp.GetPromptResult {
	t.Helper()
	res, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("get %s: %v", name, err)
	}
	return res
}

func promptText(res *mcp.GetPromptResult) (script string, embedded []*mcp.EmbeddedResource) {
	for i, m := range res.Messages {
		switch c := m.Content.(type) {
		case *mcp.TextContent:
			if i == 0 {
				script = c.Text
			}
		case *mcp.EmbeddedResource:
			embedded = append(embedded, c)
		}
	}
	return script, embedded
}

func TestPromptsRegisteredMatchFiles(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	res, err := cs.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*mcp.Prompt{}
	for _, p := range res.Prompts {
		got[p.Name] = p
		if p.Title == "" || p.Description == "" {
			t.Errorf("prompt %s lacks title or description", p.Name)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, e := range entries {
		name, isMD := strings.CutSuffix(e.Name(), ".md")
		if e.IsDir() || !isMD {
			continue
		}
		files++
		if got[name] == nil {
			t.Errorf("%s is not registered", e.Name())
		}
		delete(got, name)
	}
	for name := range got {
		t.Errorf("prompt %s has no workflow file", name)
	}
	if files != len(res.Prompts) {
		t.Errorf("files = %d, prompts = %d", files, len(res.Prompts))
	}
}

func TestPromptArguments(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	res, err := cs.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"check-inbox": nil,
		"reply":       {"message_id*"},
		"escalate":    {"topic*", "thread_id"},
		"handoff":     {"to*", "summary"},
		"standup":     {"address"},
	}
	for _, p := range res.Prompts {
		got := make([]string, 0, len(p.Arguments))
		for _, a := range p.Arguments {
			s := a.Name
			if a.Required {
				s += "*"
			}
			got = append(got, s)
		}
		if strings.Join(got, ",") != strings.Join(want[p.Name], ",") {
			t.Errorf("%s arguments = %v, want %v", p.Name, got, want[p.Name])
		}
	}
}

func TestCheckInboxPrompt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	anon := e.Client(t, "anon")
	script, emb := promptText(getPrompt(t, anon, "check-inbox", nil))
	mustContain(t, "script", script, "You are not signed on", "github.com/acme/api", "sign_on")
	if len(emb) != 1 || emb[0].Resource.URI != "aim://inbox" || !strings.Contains(emb[0].Resource.Text, "Not signed on") {
		t.Fatalf("embedded = %+v", emb)
	}

	_, b, id, _ := twoAgents(t, e)
	script, emb = promptText(getPrompt(t, b, "check-inbox", nil))
	mustContain(t, "script", script, "signed on as `"+bob+"`")
	if len(emb) != 1 || !strings.Contains(emb[0].Resource.Text, id) {
		t.Fatalf("embedded = %+v", emb)
	}
	// Prompts never mark mail read.
	if l := ok(t, b, "list", map[string]any{"address": bob}); len(l["items"].([]any)) != 1 {
		t.Fatalf("prompt marked mail read: %v", l)
	}
}

func TestReplyPrompt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, b, id, thread := twoAgents(t, e)
	script, emb := promptText(getPrompt(t, b, "reply", map[string]string{"message_id": id}))
	mustContain(t, "script", script, "Reply to "+id, `"`+id+`"`, "in_reply_to")
	if len(emb) != 1 || emb[0].Resource.URI != "aim://thread/"+thread || !strings.Contains(emb[0].Resource.Text, "zebra") {
		t.Fatalf("embedded = %+v", emb)
	}
	if l := ok(t, b, "list", map[string]any{"address": bob}); len(l["items"].([]any)) != 1 {
		t.Fatalf("prompt marked mail read: %v", l)
	}
	for name, args := range map[string]map[string]string{
		"reply": nil, "escalate": {"topic": " "}, "handoff": {},
	} {
		if _, err := b.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name, Arguments: args}); err == nil {
			t.Errorf("%s with args %v: want a missing-argument error", name, args)
		}
	}
	if _, err := b.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "reply", Arguments: map[string]string{"message_id": "msg-nope"}}); err == nil {
		t.Error("reply to an unknown message should fail")
	}
}

func TestEscalatePrompt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, b, _, thread := twoAgents(t, e)
	script, emb := promptText(getPrompt(t, b, "escalate", map[string]string{"topic": "deploy-approval"}))
	mustContain(t, "script", script, "Escalate: deploy-approval", "Decision needed: deploy-approval", "starts a new one")
	if len(emb) != 0 || strings.Contains(script, "in_reply_to") {
		t.Errorf("escalate without thread: embedded=%d script mentions in_reply_to=%v", len(emb), strings.Contains(script, "in_reply_to"))
	}
	script, emb = promptText(getPrompt(t, b, "escalate", map[string]string{"topic": "t", "thread_id": thread}))
	mustContain(t, "script", script, thread, "in_reply_to")
	if len(emb) != 1 || !strings.Contains(emb[0].Resource.Text, "zebra") {
		t.Fatalf("embedded = %+v", emb)
	}
	if _, err := b.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "escalate", Arguments: map[string]string{"topic": "t", "thread_id": "nope"}}); err == nil {
		t.Error("escalate with an unknown thread should fail")
	}
}

func TestHandoffPrompt(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	script, emb := promptText(getPrompt(t, cs, "handoff", map[string]string{"to": bob}))
	mustContain(t, "script", script, "Hand off to "+bob, "roster", "You are not signed on")
	if len(emb) != 0 || strings.Contains(script, "Topic hint") {
		t.Errorf("unexpected attachment or hint: %d %q", len(emb), script)
	}
	script, _ = promptText(getPrompt(t, cs, "handoff", map[string]string{"to": bob, "summary": "migrations"}))
	mustContain(t, "script", script, "Topic hint: migrations")
}

func TestStandupPrompt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, b, id, _ := twoAgents(t, e)
	res := getPrompt(t, b, "standup", nil)
	script, _ := promptText(res)
	mustContain(t, "script", script, "Standup for github.com/acme/api")
	if len(res.Messages) != 2 {
		t.Fatalf("messages = %d, want script plus digest", len(res.Messages))
	}
	digest := res.Messages[1].Content.(*mcp.TextContent).Text
	mustContain(t, "digest", digest, "Mail since", id, "deploy plan")
	// Read mail still shows in the standup digest, and standup marks nothing.
	ok(t, b, "read", map[string]any{"message_id": id})
	res = getPrompt(t, b, "standup", map[string]string{"address": "github.com/acme"})
	mustContain(t, "digest", res.Messages[1].Content.(*mcp.TextContent).Text, id)
	if _, err := b.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "standup", Arguments: map[string]string{"address": "!!bad"}}); err == nil {
		t.Error("standup with an invalid address should fail")
	}

	e2 := newEnv(t)
	e2.NoNamespace = true
	if _, err := e2.Client(t, "n").GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "standup"}); err == nil {
		t.Error("standup without address or namespace should fail")
	}
}
