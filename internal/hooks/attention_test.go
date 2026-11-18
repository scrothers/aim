package hooks_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/hooks"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/wire"
)

const alertAfter = 20 * time.Millisecond

// alerting swaps in a server that alerts the operator after alertAfter.
func (e *env) alerting() {
	e.srv = hooks.New(e.svc, e.reg, hooks.WithAlertAfter(alertAfter), hooks.WithClock(e.clock))
}

func (e *env) attention(t *testing.T, pid int) (state, detail string) {
	t.Helper()
	entry, ok := e.reg.Agent("box", "", pid)
	if !ok {
		t.Fatalf("no connection for pid %d", pid)
	}
	st := entry.Status()
	return st.Attention, st.AttentionDetail
}

func (e *env) operatorMail(t *testing.T) []mail.Header {
	t.Helper()
	res, err := e.svc.List(context.Background(), mail.ListInput{Address: "operator", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	return res.Items
}

// waitOperatorMail waits up to d for n messages in the operator's inbox.
func (e *env) waitOperatorMail(t *testing.T, n int, d time.Duration) []mail.Header {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		items := e.operatorMail(t)
		if len(items) >= n || time.Now().After(deadline) {
			return items
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func status(state, detail string) wire.HookRequest {
	return wire.HookRequest{Event: wire.HookStatus, Attention: state, Detail: detail}
}

func TestAttentionSetAndClearedByActivity(t *testing.T) {
	t.Parallel()
	for _, ev := range []string{wire.HookPrompt, wire.HookPostTool, wire.HookPreTool, wire.HookSessionStart, wire.HookSubagentStart} {
		t.Run(ev, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			e.connect(t, 10, alice)
			e.event(t, hook(10), status(wire.AttentionBlocked, "Claude needs your permission to use Bash"))
			if s, d := e.attention(t, 10); s != wire.AttentionBlocked || !strings.Contains(d, "Bash") {
				t.Fatalf("attention = %q %q", s, d)
			}
			e.event(t, hook(10), wire.HookRequest{Event: ev})
			if s, _ := e.attention(t, 10); s != "" {
				t.Errorf("after %s attention = %q, want cleared", ev, s)
			}
		})
	}
}

func TestAttentionEmptyStatusClears(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 10, alice)
	e.event(t, hook(10), status(wire.AttentionWaiting, ""))
	e.event(t, hook(10), status("", ""))
	if s, _ := e.attention(t, 10); s != "" {
		t.Errorf("attention = %q", s)
	}
}

func TestAttentionWithoutConnectionIsIgnored(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.alerting()
	r := e.event(t, hook(99), status(wire.AttentionBlocked, "x"))
	if r.SignedOn || r.Context != "" {
		t.Errorf("reply = %+v", r)
	}
	if got := e.waitOperatorMail(t, 1, 4*alertAfter); len(got) != 0 {
		t.Errorf("operator mail = %d", len(got))
	}
}

func TestAttentionDetailTruncated(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 10, alice)
	e.event(t, hook(10), status(wire.AttentionBlocked, strings.Repeat("é", 400)))
	_, d := e.attention(t, 10)
	if len(d) > 504 || !strings.HasSuffix(d, "…") {
		t.Errorf("detail len %d suffix %q", len(d), d[len(d)-3:])
	}
}

func TestAlertBlockedOnce(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.alerting()
	e.connect(t, 10, alice)
	e.event(t, hook(10), status(wire.AttentionBlocked, "Claude needs your permission to use Bash"))
	items := e.waitOperatorMail(t, 1, time.Second)
	if len(items) != 1 {
		t.Fatalf("operator mail = %d, want 1", len(items))
	}
	if h := items[0]; h.From != alice || !strings.Contains(h.Subject, "is blocked") {
		t.Errorf("alert from %q subject %q", h.From, h.Subject)
	}
	m, err := e.svc.Read(context.Background(), mail.OperatorCaller("op", "test", "box"), mail.ReadInput{MessageID: items[0].ID, Peek: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Body, "> Claude needs your permission to use Bash") || !strings.Contains(m.Body, "- Machine: box") {
		t.Errorf("body:\n%s", m.Body)
	}
	// Repeating the state in the same spell sends nothing more.
	e.event(t, hook(10), status(wire.AttentionBlocked, "again"))
	if got := e.waitOperatorMail(t, 2, 4*alertAfter); len(got) != 1 {
		t.Errorf("operator mail = %d after repeat, want 1", len(got))
	}
	// A new spell alerts again.
	e.event(t, hook(10), wire.HookRequest{Event: wire.HookPostTool})
	e.event(t, hook(10), status(wire.AttentionFailed, "API error: overloaded"))
	items = e.waitOperatorMail(t, 2, time.Second)
	if len(items) != 2 || !strings.Contains(items[0].Subject, "stopped on an error") {
		t.Errorf("operator mail = %+v", items)
	}
}

func TestAlertSkipped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"cleared before delay", func(t *testing.T, e *env) {
			e.event(t, hook(10), status(wire.AttentionBlocked, "x"))
			e.event(t, hook(10), wire.HookRequest{Event: wire.HookPostTool})
		}},
		{"waiting never alerts", func(t *testing.T, e *env) {
			e.event(t, hook(10), status(wire.AttentionWaiting, "idle"))
		}},
		{"interrupted never alerts", func(t *testing.T, e *env) {
			e.event(t, hook(10), status(wire.AttentionInterrupted, ""))
		}},
		{"session end", func(t *testing.T, e *env) {
			e.event(t, hook(10), status(wire.AttentionBlocked, "x"))
			e.event(t, hook(10), wire.HookRequest{Event: wire.HookSessionEnd})
		}},
		{"server closed", func(t *testing.T, e *env) {
			e.event(t, hook(10), status(wire.AttentionBlocked, "x"))
			e.srv.Close()
			e.event(t, hook(10), status(wire.AttentionFailed, "y"))
		}},
		{"activity over MCP", func(t *testing.T, e *env) {
			e.event(t, hook(10), status(wire.AttentionBlocked, "x"))
			entry, _ := e.reg.Agent("box", "", 10)
			entry.Touch(time.Now())
		}},
		{"disconnected", func(t *testing.T, e *env) {
			e.event(t, hook(10), status(wire.AttentionBlocked, "x"))
			e.reg.Remove("conn-" + alice)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			e.alerting()
			e.connect(t, 10, alice)
			tc.run(t, e)
			if got := e.waitOperatorMail(t, 1, 4*alertAfter); len(got) != 0 {
				t.Errorf("operator mail = %d, want 0", len(got))
			}
		})
	}
}

func TestAlertNotSignedOn(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.alerting()
	e.connect(t, 10, "")
	e.event(t, hook(10), status(wire.AttentionBlocked, "x"))
	if s, _ := e.attention(t, 10); s != wire.AttentionBlocked {
		t.Errorf("attention = %q", s)
	}
	if got := e.waitOperatorMail(t, 1, 4*alertAfter); len(got) != 0 {
		t.Errorf("operator mail = %d, want 0", len(got))
	}
}

func TestPostToolNudgesSubagentMailbox(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 10, alice)
	e.event(t, hook(10), wire.HookRequest{Event: wire.HookSessionStart})
	e.send(t, bob, alice, "for the parent")
	e.send(t, bob, "reviewer@"+alice, "for the reviewer")
	r := e.event(t, hook(10), wire.HookRequest{Event: wire.HookPostTool, AgentType: "Reviewer"})
	if !strings.Contains(r.Context, "for the reviewer") || strings.Contains(r.Context, "for the parent") {
		t.Fatalf("subagent nudge:\n%s", r.Context)
	}
	if r := e.event(t, hook(10), wire.HookRequest{Event: wire.HookPostTool, AgentType: "Reviewer"}); r.Context != "" {
		t.Errorf("second subagent nudge:\n%s", r.Context)
	}
	// The parent's watermark is untouched by the subagent's nudges.
	if r := e.event(t, hook(10), wire.HookRequest{Event: wire.HookPostTool}); !strings.Contains(r.Context, "for the parent") {
		t.Errorf("parent nudge:\n%s", r.Context)
	}
}
