package hooks_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/hooks"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/wire"
)

const (
	ns    = "github.com/acme/api"
	alice = "alice@github.com/acme/api"
	bob   = "bob@github.com/acme/api"
)

type env struct {
	svc *mail.Service
	reg *presence.Registry
	srv *hooks.Server
	// offset shifts the hook server's clock ahead of real time; message ids
	// use real time, so only tests that never compare ids advance it.
	offset time.Duration
	mu     sync.Mutex
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/aim.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &env{reg: presence.New()}
	e.svc = mail.New(st, mcpserver.NewPresence(e.reg))
	e.srv = hooks.New(e.svc, e.reg, hooks.WithDebounce(10*time.Millisecond), hooks.WithPoll(20*time.Millisecond), hooks.WithClock(e.clock))
	for _, a := range []string{alice, bob} {
		if _, err := e.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: a, Role: "dev"}); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func (e *env) clock() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return time.Now().Add(e.offset)
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.offset += d
}

// connect registers an MCP connection for an agent with pid on "box",
// optionally signed on as identity.
func (e *env) connect(t *testing.T, pid int, identity string) {
	t.Helper()
	entry := e.reg.Add("conn-"+identity, wire.Preamble{Machine: "box", AgentPID: pid, Namespace: ns}, time.Now())
	t.Cleanup(func() { e.reg.Remove("conn-" + identity) })
	if identity != "" {
		if err := entry.SignOn(address.MustParse(identity), "", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
}

func (e *env) send(t *testing.T, from, to, subject string) mail.Message {
	t.Helper()
	m, err := e.svc.Send(context.Background(), mail.Caller{Identity: address.MustParse(from), SignedOn: true},
		mail.SendInput{To: []string{to}, Subject: subject, Body: "body of " + subject})
	if err != nil {
		t.Fatal(err)
	}
	// Step past the message's millisecond so later time floors sort after it.
	time.Sleep(2 * time.Millisecond)
	return m
}

func hook(pid int) wire.Preamble {
	return wire.Preamble{Kind: wire.KindHook, Machine: "box", AgentPID: pid}
}

func (e *env) event(t *testing.T, p wire.Preamble, req wire.HookRequest) wire.HookReply {
	t.Helper()
	req.Op = wire.HookOpEvent
	r, err := e.srv.Handle(context.Background(), p, req)
	if err != nil {
		t.Fatalf("Handle(%s): %v", req.Event, err)
	}
	return r
}

func TestSessionStartNotSignedOn(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 10, "")
	r := e.event(t, hook(10), wire.HookRequest{Event: wire.HookSessionStart})
	if r.SignedOn {
		t.Fatal("reply claims signed on")
	}
	for _, want := range []string{"not signed on", "`agent_sign_on`", "`" + alice + "`", "Identities in `" + ns + "`"} {
		if !strings.Contains(r.Context, want) {
			t.Errorf("context lacks %q:\n%s", want, r.Context)
		}
	}
	// Prompts stay silent until the agent signs on.
	if r := e.event(t, hook(10), wire.HookRequest{Event: wire.HookPrompt}); r.Context != "" {
		t.Errorf("prompt context = %q, want empty", r.Context)
	}
}

func TestSessionStartWithoutNamespace(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	r := e.event(t, hook(99), wire.HookRequest{Event: wire.HookSessionStart})
	if !strings.Contains(r.Context, "`agent_roster` tool") {
		t.Errorf("context = %q", r.Context)
	}
}

func TestPromptNudgesOnlyNewMail(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 11, alice)
	e.send(t, bob, alice, "old news")

	r := e.event(t, hook(11), wire.HookRequest{Event: wire.HookSessionStart})
	if !r.SignedOn || r.Identity != alice || !strings.Contains(r.Context, "signed on to AIM as `"+alice+"`") || !strings.Contains(r.Context, "old news") {
		t.Fatalf("session start = %+v", r)
	}
	if r := e.event(t, hook(11), wire.HookRequest{Event: wire.HookPrompt}); r.Context != "" {
		t.Fatalf("prompt after start repeated mail: %q", r.Context)
	}
	e.send(t, bob, alice, "fresh news")
	r = e.event(t, hook(11), wire.HookRequest{Event: wire.HookPrompt})
	if !strings.Contains(r.Context, "fresh news") || strings.Contains(r.Context, "old news") {
		t.Fatalf("prompt nudge = %q", r.Context)
	}
	if !strings.Contains(r.Context, "2 unread in all") {
		t.Errorf("nudge lacks total: %q", r.Context)
	}
	if r := e.event(t, hook(11), wire.HookRequest{Event: wire.HookPostTool}); r.Context != "" {
		t.Fatalf("second nudge repeated mail: %q", r.Context)
	}
}

func TestPromptFirstContactShowsUnread(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 12, alice)
	e.send(t, bob, alice, "waiting")
	if r := e.event(t, hook(12), wire.HookRequest{Event: wire.HookPrompt}); !strings.Contains(r.Context, "waiting") {
		t.Fatalf("first prompt = %q", r.Context)
	}
	// An agent that can't be keyed gets no prompt nudges.
	if r := e.event(t, wire.Preamble{Kind: wire.KindHook}, wire.HookRequest{Event: wire.HookPrompt}); r.Context != "" {
		t.Fatalf("keyless prompt = %q", r.Context)
	}
}

func TestSessionStartNoUnread(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 13, alice)
	if r := e.event(t, hook(13), wire.HookRequest{Event: wire.HookSessionStart}); !strings.Contains(r.Context, "No unread mail") {
		t.Fatalf("context = %q", r.Context)
	}
}

func TestRestoredSignonBySession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if err := e.svc.SaveSignon(context.Background(), mail.Signon{SessionID: "sess-x", Identity: address.MustParse(alice)}); err != nil {
		t.Fatal(err)
	}
	r := e.event(t, wire.Preamble{Kind: wire.KindHook, SessionID: "sess-x"}, wire.HookRequest{Event: wire.HookSessionStart})
	if !r.SignedOn || r.Identity != alice {
		t.Fatalf("reply = %+v", r)
	}
}

func TestSubagentStartAndAttribution(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 14, alice)
	e.send(t, bob, "helper@"+alice, "for the helper")

	r := e.event(t, hook(14), wire.HookRequest{Event: wire.HookSubagentStart, AgentType: "Helper"})
	for _, want := range []string{"subagent `helper`", "`helper@" + alice + "`", "for the helper"} {
		if !strings.Contains(r.Context, want) {
			t.Errorf("subagent context lacks %q:\n%s", want, r.Context)
		}
	}
	if r := e.event(t, hook(14), wire.HookRequest{Event: wire.HookSubagentStart, AgentType: "!!!"}); r.Context != "" {
		t.Errorf("unusable agent type context = %q", r.Context)
	}

	e.event(t, hook(14), wire.HookRequest{Event: wire.HookPreTool, AgentType: "Helper", ToolUseID: "tu-1"})
	e.event(t, hook(14), wire.HookRequest{Event: wire.HookPreTool, ToolUseID: "tu-main"}) // main agent: nothing recorded
	if sub, ok := e.srv.SubagentFor("tu-1"); !ok || sub != "helper" {
		t.Fatalf("SubagentFor = %q, %v", sub, ok)
	}
	if _, ok := e.srv.SubagentFor("tu-1"); ok {
		t.Error("tool use id was not consumed")
	}
	if _, ok := e.srv.SubagentFor("tu-main"); ok {
		t.Error("main-agent tool call attributed")
	}
	e.event(t, hook(14), wire.HookRequest{Event: wire.HookPreTool, Subagent: "x", ToolUseID: "tu-2"})
	e.advance(2 * time.Minute)
	if _, ok := e.srv.SubagentFor("tu-2"); ok {
		t.Error("expired tool use id attributed")
	}
	if _, ok := e.srv.SubagentFor(""); ok {
		t.Error("empty tool use id attributed")
	}
}

func TestSubagentName(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"general-purpose":       "general-purpose",
		"Code Reviewer":         "code-reviewer",
		"  Explore ":            "explore",
		"-lead":                 "lead",
		"":                      "",
		"!!!":                   "",
		strings.Repeat("a", 80): strings.Repeat("a", 64),
	}
	for in, want := range tests {
		if got := hooks.SubagentName(in); got != want {
			t.Errorf("SubagentName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestServeOverConn(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 15, alice)
	srvConn, cliConn := net.Pipe()
	t.Cleanup(func() { _ = cliConn.Close() })
	done := make(chan error, 1)
	go func() {
		defer srvConn.Close()
		done <- e.srv.Serve(context.Background(), srvConn, bufio.NewReader(srvConn), hook(15))
	}()
	if err := wire.WriteLine(cliConn, wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookSessionStart}); err != nil {
		t.Fatal(err)
	}
	var r wire.HookReply
	if err := wire.ReadLine(bufio.NewReader(cliConn), &r); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !r.SignedOn || r.Identity != alice {
		t.Fatalf("reply = %+v", r)
	}
}

func TestServeBadRequest(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	srvConn, cliConn := net.Pipe()
	go func() { _, _ = cliConn.Write([]byte("nope\n")); _ = cliConn.Close() }()
	if err := e.srv.Serve(context.Background(), srvConn, bufio.NewReader(srvConn), hook(1)); err == nil {
		t.Fatal("want error")
	}
}
