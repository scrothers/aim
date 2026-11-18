package resources_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/mcpserver/mcptest"
	"github.com/scrothers/aim/internal/mcpserver/resources"
)

// legacyProtocol is the pre-SEP-2575 protocol, where resources/subscribe is a
// plain request whose errors come back synchronously. Claude Code speaks it.
const legacyProtocol = "2025-11-25"

const (
	alice = mcptest.Alice
	bob   = mcptest.Bob
)

// Short names for the shared test helpers.
var (
	newEnv      = mcptest.New
	ok          = mcptest.OK
	register    = mcptest.Register
	readRes     = mcptest.ReadResource
	mustContain = mcptest.MustContain
)

func twoAgents(t *testing.T, e *mcptest.Env) (a, b *mcp.ClientSession, msgID, threadID string) {
	t.Helper()
	return e.TwoAgents(t)
}

func TestResourceListAndTemplates(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	res, err := cs.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range res.Resources {
		got[r.URI] = true
	}
	for _, u := range []string{"aim://me", "aim://inbox", "aim://roster"} {
		if !got[u] {
			t.Errorf("resource %s not listed", u)
		}
	}
	ts, err := cs.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := map[string]bool{}
	for _, r := range ts.ResourceTemplates {
		tmpl[r.URITemplate] = true
	}
	for _, u := range []string{"aim://thread/{thread_id}", "aim://message/{message_id}", "aim://mailbox/{address}", "aim://roster/{address}"} {
		if !tmpl[u] {
			t.Errorf("template %s not listed", u)
		}
	}
}

func TestStaticResources(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cs := e.Client(t, "a")
	mustContain(t, "me", readRes(t, cs, "aim://me"), "sess-a", "github.com/acme/api", "nobody")
	mustContain(t, "inbox", readRes(t, cs, "aim://inbox"), "Not signed on", "sign_on")
	mustContain(t, "roster", readRes(t, cs, "aim://roster"), "No identities")

	a, b, id, thread := twoAgents(t, e)
	mustContain(t, "me", readRes(t, a, "aim://me"), "signed on as: "+alice)
	in := readRes(t, b, "aim://inbox")
	mustContain(t, "inbox", in, "deploy plan", id, thread, alice, "zebra")
	mustContain(t, "roster", readRes(t, b, "aim://roster"), alice, bob, "presence: online")
	if strings.Contains(readRes(t, a, "aim://inbox"), "deploy plan") {
		t.Error("sender's inbox lists the message it sent")
	}

	// Reading resources must not have marked anything read.
	if l := ok(t, b, "mail_list", map[string]any{"address": bob}); len(l["items"].([]any)) != 1 {
		t.Fatalf("resources marked mail read: %v", l)
	}
}

func TestNoNamespaceRoster(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.NoNamespace = true
	cs := e.Client(t, "a")
	mustContain(t, "roster", readRes(t, cs, "aim://roster"), "no git namespace")
}

func TestTemplateResources(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, b, id, thread := twoAgents(t, e)
	ok(t, b, "mail_send", map[string]any{"to": []string{alice}, "subject": "re", "body": "ack", "in_reply_to": id})

	th := readRes(t, b, "aim://thread/"+thread)
	mustContain(t, "thread", th, "2 message(s)", "zebra", "ack")
	if strings.Index(th, "zebra") > strings.Index(th, "ack") {
		t.Error("thread is not oldest first")
	}
	mustContain(t, "message", readRes(t, b, "aim://message/"+id), "deploy plan", "zebra")

	// Addresses are percent-encoded into one segment and decoded by the handler.
	enc := resources.EncodeSegment(bob)
	if strings.ContainsAny(enc, "@/") {
		t.Fatalf("encoded segment %q keeps reserved characters", enc)
	}
	mustContain(t, "mailbox", readRes(t, b, "aim://mailbox/"+enc), "deploy plan")
	mustContain(t, "mailbox of repo", readRes(t, b, "aim://mailbox/"+resources.EncodeSegment("github.com/acme/api")), "deploy plan")
	mustContain(t, "roster", readRes(t, b, "aim://roster/"+resources.EncodeSegment("github.com/acme")), alice, bob)
	mustContain(t, "roster of identity", readRes(t, b, "aim://roster/"+enc), bob)

	// Reads through templates are peeks.
	if l := ok(t, b, "mail_list", map[string]any{"address": bob}); len(l["items"].([]any)) != 1 {
		t.Fatalf("template resources marked mail read: %v", l)
	}
}

func TestResourceNotFound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cs := e.Client(t, "a")
	for _, uri := range []string{
		"aim://nothing", "aim://thread/nope", "aim://message/msg-nope", "aim://thread/",
		"aim://message/a%2Fb", "aim://mailbox/%zz",
	} {
		if _, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Errorf("read %s: want an error", uri)
		}
	}
	if _, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "aim://mailbox/" + resources.EncodeSegment("!!bad")}); err == nil ||
		!strings.Contains(err.Error(), "address grammar") {
		t.Errorf("invalid address error = %v", err)
	}
}

type updates struct {
	ch chan string
}

func newUpdates() *updates { return &updates{ch: make(chan string, 16)} }

func (u *updates) handler(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
	u.ch <- req.Params.URI
}

func (u *updates) expect(t *testing.T, uri string) {
	t.Helper()
	select {
	case got := <-u.ch:
		if got != uri {
			t.Fatalf("update for %s, want %s", got, uri)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no ResourceUpdated for %s", uri)
	}
}

func (u *updates) expectNone(t *testing.T) {
	t.Helper()
	select {
	case got := <-u.ch:
		t.Fatalf("unexpected ResourceUpdated for %s", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestSubscriptions(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.Protocol = legacyProtocol
	up := newUpdates()
	exit := make(chan struct{})
	opts := []mcpserver.Option{mcpserver.WithSubscribeDebounce(10 * time.Millisecond), mcpserver.WithWatcherExit(exit)}
	b := e.ClientWith(t, "b", opts, &mcp.ClientOptions{ResourceUpdatedHandler: up.handler})
	a := e.Client(t, "a")
	register(t, a, alice, "lead")
	register(t, b, bob, "dev")
	ok(t, a, "agent_sign_on", map[string]any{"address": alice})
	ok(t, b, "agent_sign_on", map[string]any{"address": bob})

	ctx := context.Background()
	if err := b.Subscribe(ctx, &mcp.SubscribeParams{URI: "aim://roster"}); err == nil {
		t.Error("subscribing to a static non-mail resource should fail")
	}
	if err := b.Subscribe(ctx, &mcp.SubscribeParams{URI: "aim://thread/nope"}); err == nil {
		t.Error("subscribing to an unknown thread should fail")
	}
	for _, u := range []string{"aim://inbox", "aim://mailbox/" + resources.EncodeSegment("github.com/acme/api")} {
		if err := b.Subscribe(ctx, &mcp.SubscribeParams{URI: u}); err != nil {
			t.Fatalf("subscribe %s: %v", u, err)
		}
	}

	sent := ok(t, a, "mail_send", map[string]any{"to": []string{bob}, "subject": "hi", "body": "x"})
	got := map[string]bool{}
	for range 2 {
		select {
		case u := <-up.ch:
			got[u] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("updates so far: %v", got)
		}
	}
	if !got["aim://inbox"] || !got["aim://mailbox/"+resources.EncodeSegment("github.com/acme/api")] {
		t.Fatalf("updates = %v", got)
	}

	// Marking read changes the unread digest, so it notifies again; a mark
	// that changes nothing does not.
	id := sent["id"].(string)
	ok(t, b, "mail_mark", map[string]any{"message_ids": []string{id}, "state": "read"})
	for range 2 {
		<-up.ch
	}
	ok(t, b, "mail_mark", map[string]any{"message_ids": []string{id}, "state": "read"})
	up.expectNone(t)

	// After unsubscribe, no more updates for that URI.
	if err := b.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: "aim://inbox"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: "aim://mailbox/" + resources.EncodeSegment("github.com/acme/api")}); err != nil {
		t.Fatal(err)
	}
	ok(t, a, "mail_send", map[string]any{"to": []string{bob}, "subject": "again", "body": "x"})
	up.expectNone(t)

	// The watcher ends with the session.
	select {
	case <-exit:
		t.Fatal("watcher exited while the session is open")
	default:
	}
	if err := b.Close(); err != nil {
		t.Logf("close: %v", err)
	}
	select {
	case <-exit:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher goroutines still running after the session ended")
	}
}

// TestSubscriptionsModernProtocol covers SEP-2575 clients, where Subscribe
// opens a subscriptions/listen stream instead of calling resources/subscribe.
func TestSubscriptionsModernProtocol(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	up := newUpdates()
	opts := []mcpserver.Option{mcpserver.WithSubscribeDebounce(10 * time.Millisecond)}
	b := e.ClientWith(t, "b", opts, &mcp.ClientOptions{ResourceUpdatedHandler: up.handler})
	a := e.Client(t, "a")
	register(t, a, alice, "lead")
	register(t, b, bob, "dev")
	ok(t, a, "agent_sign_on", map[string]any{"address": alice})
	ok(t, b, "agent_sign_on", map[string]any{"address": bob})
	if err := b.Subscribe(context.Background(), &mcp.SubscribeParams{URI: "aim://inbox"}); err != nil {
		t.Fatal(err)
	}
	// The listen stream registers the subscription asynchronously, so keep
	// sending until the first update shows it is live.
	deadline := time.After(5 * time.Second)
	for {
		ok(t, a, "mail_send", map[string]any{"to": []string{bob}, "subject": "hi", "body": "x"})
		select {
		case got := <-up.ch:
			if got != "aim://inbox" {
				t.Fatalf("update for %s", got)
			}
			return
		case <-time.After(100 * time.Millisecond):
		case <-deadline:
			t.Fatal("no ResourceUpdated over the modern protocol")
		}
	}
}

func TestThreadSubscription(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.Protocol = legacyProtocol
	up := newUpdates()
	opts := []mcpserver.Option{mcpserver.WithSubscribeDebounce(10 * time.Millisecond)}
	a, b, id, thread := twoAgents(t, e)
	_ = a
	// Reconnect bob's side with an update handler: a second connection as bob.
	c := e.ClientWith(t, "c", opts, &mcp.ClientOptions{ResourceUpdatedHandler: up.handler})
	ok(t, c, "agent_sign_on", map[string]any{"address": bob})
	uri := "aim://thread/" + thread
	if err := c.Subscribe(context.Background(), &mcp.SubscribeParams{URI: uri}); err != nil {
		t.Fatal(err)
	}
	ok(t, b, "mail_send", map[string]any{"to": []string{alice}, "subject": "re", "body": "ack", "in_reply_to": id})
	up.expect(t, uri)
}
