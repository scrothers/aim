package completions_test

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver/mcptest"
)

const (
	alice = mcptest.Alice
	bob   = mcptest.Bob
)

// Short names for the shared test helpers.
var (
	newEnv   = mcptest.New
	ok       = mcptest.OK
	register = mcptest.Register
)

func twoAgents(t *testing.T, e *mcptest.Env) (a, b *mcp.ClientSession, msgID, threadID string) {
	t.Helper()
	return e.TwoAgents(t)
}

func promptRef(name string) *mcp.CompleteReference {
	return &mcp.CompleteReference{Type: "ref/prompt", Name: name}
}

func resourceRef(tmpl string) *mcp.CompleteReference {
	return &mcp.CompleteReference{Type: "ref/resource", URI: tmpl}
}

func complete(t *testing.T, cs *mcp.ClientSession, ref *mcp.CompleteReference, arg, value string) mcp.CompletionResultDetails {
	t.Helper()
	res, err := cs.Complete(context.Background(), &mcp.CompleteParams{Ref: ref, Argument: mcp.CompleteParamsArgument{Name: arg, Value: value}})
	if err != nil {
		t.Fatalf("complete %v %s=%q: %v", ref, arg, value, err)
	}
	if res.Completion.Values == nil {
		t.Fatalf("complete %v %s=%q: values is nil, want a JSON array", ref, arg, value)
	}
	return res.Completion
}

func TestCompleteIdentities(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a, b, _, _ := twoAgents(t, e)
	ok(t, a, "sign_off", map[string]any{})

	// bob is online, alice is not: online identities rank first.
	got := complete(t, b, promptRef("handoff"), "to", "")
	if want := []string{bob, alice, "operator"}; !slices.Equal(got.Values, want) || got.Total != 3 || got.HasMore {
		t.Fatalf("handoff.to = %+v, want %v", got, want)
	}
	if got := complete(t, b, promptRef("handoff"), "to", "AL"); !slices.Equal(got.Values, []string{alice}) || got.Total != 1 {
		t.Fatalf("handoff.to prefix = %+v", got)
	}
	if got := complete(t, b, promptRef("handoff"), "to", "zzz"); len(got.Values) != 0 || got.Total != 0 {
		t.Fatalf("handoff.to no match = %+v", got)
	}
	if got := complete(t, b, promptRef("handoff"), "to", "bob%40"); !slices.Equal(got.Values, []string{bob}) {
		t.Fatalf("encoded typed value = %+v", got)
	}
}

func TestCompleteAddresses(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, b, _, _ := twoAgents(t, e)
	got := complete(t, b, promptRef("standup"), "address", "github")
	if want := []string{"github.com/acme/api", "github.com/acme"}; !slices.Equal(got.Values, want) {
		t.Errorf("standup.address scopes = %v, want %v", got.Values, want)
	}
	got = complete(t, b, promptRef("standup"), "address", "")
	for _, want := range []string{"github.com/acme/api", "github.com/acme", alice, bob, "operator"} {
		if !slices.Contains(got.Values, want) {
			t.Errorf("standup.address lacks %q: %v", want, got.Values)
		}
	}
	if got.Total != len(got.Values) {
		t.Errorf("total = %d, values = %d", got.Total, len(got.Values))
	}
}

func TestCompleteTemplateValuesRoundTrip(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, b, id, thread := twoAgents(t, e)

	for _, tmpl := range []string{"aim://mailbox/{address}", "aim://roster/{address}"} {
		got := complete(t, b, resourceRef(tmpl), "address", "")
		if len(got.Values) == 0 {
			t.Fatalf("%s: no completions", tmpl)
		}
		for _, v := range got.Values {
			dec, err := url.PathUnescape(v)
			if err != nil || dec == v && (len(v) > 0 && (v[0] == '@')) {
				t.Errorf("%s: value %q is not valid percent-encoding: %v", tmpl, v, err)
			}
			if tmpl == "aim://roster/{address}" && dec == "operator" {
				t.Errorf("roster template offers the reserved operator address")
			}
			// Every suggestion must be readable as a resource.
			uri := tmpl[:len(tmpl)-len("{address}")] + v
			if _, err := b.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri}); err != nil {
				t.Errorf("completed value %q does not read back (%s): %v", v, uri, err)
			}
		}
		if !slices.Contains(got.Values, "bob%40github.com%2Facme%2Fapi") {
			t.Errorf("%s: encoded bob missing: %v", tmpl, got.Values)
		}
	}

	if got := complete(t, b, resourceRef("aim://thread/{thread_id}"), "thread_id", ""); !slices.Equal(got.Values, []string{encodeID(thread)}) {
		t.Errorf("thread ids = %v, want [%s]", got.Values, thread)
	}
	if got := complete(t, b, resourceRef("aim://message/{message_id}"), "message_id", "msg-"); !slices.Equal(got.Values, []string{encodeID(id)}) {
		t.Errorf("message ids = %v, want [%s]", got.Values, id)
	}
}

// encodeID is the expected percent-encoding of ids that are already unreserved.
func encodeID(s string) string { return url.PathEscape(s) }

func TestCompleteMessageAndThreadIDs(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a, b, id, thread := twoAgents(t, e)
	if got := complete(t, b, promptRef("reply"), "message_id", ""); !slices.Equal(got.Values, []string{id}) {
		t.Errorf("reply.message_id = %v, want [%s]", got.Values, id)
	}
	if got := complete(t, b, promptRef("escalate"), "thread_id", ""); !slices.Equal(got.Values, []string{thread}) {
		t.Errorf("escalate.thread_id = %v, want [%s]", got.Values, thread)
	}
	if got := complete(t, b, promptRef("reply"), "message_id", "msg-zzz"); len(got.Values) != 0 {
		t.Errorf("non-matching prefix = %v", got.Values)
	}
	// A reply in the same thread yields one thread id but two message ids.
	ok(t, b, "send", map[string]any{"to": []string{alice}, "subject": "re", "body": "ack", "in_reply_to": id})
	if got := complete(t, b, promptRef("escalate"), "thread_id", ""); len(got.Values) != 1 {
		t.Errorf("thread ids not deduplicated: %v", got.Values)
	}
	if got := complete(t, a, promptRef("reply"), "message_id", ""); len(got.Values) != 1 {
		t.Errorf("alice sees %v", got.Values)
	}

	// Without signing on, ids come from the namespace.
	anon := e.Client(t, "anon")
	if got := complete(t, anon, promptRef("reply"), "message_id", ""); len(got.Values) != 2 {
		t.Errorf("namespace message ids = %v, want 2", got.Values)
	}
	e2 := newEnv(t)
	e2.NoNamespace = true
	if got := complete(t, e2.Client(t, "n"), promptRef("reply"), "message_id", ""); len(got.Values) != 0 {
		t.Errorf("no namespace, not signed on: %v", got.Values)
	}
}

func TestCompleteUnknownRefs(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	for name, ref := range map[string]*mcp.CompleteReference{
		"prompt":        promptRef("nope"),
		"resource":      resourceRef("aim://nope/{x}"),
		"wrong-arg":     promptRef("handoff"),
		"template-var":  resourceRef("aim://thread/{thread_id}"),
		"free-text-arg": promptRef("escalate"),
	} {
		arg := "x"
		if name == "free-text-arg" {
			arg = "topic"
		}
		if got := complete(t, cs, ref, arg, ""); len(got.Values) != 0 || got.Total != 0 || got.HasMore {
			t.Errorf("%s: %+v, want empty", name, got)
		}
	}
	for tmpl, arg := range map[string]string{
		"aim://message/{message_id}": "thread_id", "aim://mailbox/{address}": "message_id", "aim://roster/{address}": "x",
	} {
		if got := complete(t, cs, resourceRef(tmpl), arg, ""); len(got.Values) != 0 {
			t.Errorf("%s %s: %v, want empty", tmpl, arg, got.Values)
		}
	}
}

func TestCompleteCapsAtHundred(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cs := e.Client(t, "a")
	for i := range 105 {
		register(t, cs, fmt.Sprintf("u%03d@github.com/acme/api", i), "dev")
	}
	got := complete(t, cs, promptRef("handoff"), "to", "u")
	if len(got.Values) != 100 || got.Total != 105 || !got.HasMore {
		t.Fatalf("values=%d total=%d hasMore=%v, want 100/105/true", len(got.Values), got.Total, got.HasMore)
	}
	if got := complete(t, cs, promptRef("handoff"), "to", "u10"); len(got.Values) != 5 || got.HasMore {
		t.Fatalf("narrowed = %d values, hasMore=%v", len(got.Values), got.HasMore)
	}
}
