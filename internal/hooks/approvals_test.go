package hooks_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/hooks"
	"github.com/scrothers/aim/internal/wire"
)

// approving rebuilds the env's server with remote approvals in mode.
func (e *env) approving(mode hooks.ApprovalMode, err error) {
	e.srv = hooks.New(e.svc, e.reg, hooks.WithClock(e.clock),
		hooks.WithApprovalMode(func(context.Context) (hooks.ApprovalMode, error) { return mode, err }))
}

func approveReq() wire.HookRequest {
	return wire.HookRequest{
		Op: wire.HookOpApprove, ToolName: "Bash", Detail: "Bash: git push",
		ToolInput: json.RawMessage(`{"command":"git push"}`),
	}
}

type approveResult struct {
	reply wire.HookReply
	err   error
}

func (e *env) approve(ctx context.Context, p wire.Preamble) <-chan approveResult {
	ch := make(chan approveResult, 1)
	go func() {
		r, err := e.srv.Approve(ctx, p, approveReq())
		ch <- approveResult{r, err}
	}()
	return ch
}

// pending waits until n approvals are pending and returns them.
func (e *env) pending(t *testing.T, n int) []hooks.Approval {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := e.srv.Approvals(); len(got) == n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("pending approvals = %d, want %d", len(e.srv.Approvals()), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func awaitApprove(t *testing.T, ch <-chan approveResult) wire.HookReply {
	t.Helper()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.reply
	case <-time.After(3 * time.Second):
		t.Fatal("approve did not return")
		return wire.HookReply{}
	}
}

func TestApproveDecisions(t *testing.T) {
	t.Parallel()
	for _, verdict := range []string{wire.DecisionAllow, wire.DecisionDeny} {
		t.Run(verdict, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			e.approving(hooks.ApprovalMode{Enabled: true, Timeout: time.Minute}, nil)
			e.connect(t, 31, alice)
			p := hook(31)
			p.Client = "claude-code"
			ch := e.approve(context.Background(), p)
			got := e.pending(t, 1)[0]
			if !strings.HasPrefix(got.ID, "appr-") || got.Identity != alice || got.Tool != "Bash" ||
				got.Summary != "Bash: git push" || string(got.Input) != `{"command":"git push"}` ||
				got.Client != "claude-code" || got.Machine != "box" || got.ExpiresAt.Sub(got.CreatedAt) != time.Minute {
				t.Fatalf("approval = %+v", got)
			}
			if st, detail := e.attention(t, 31); st != wire.AttentionBlocked || !strings.Contains(detail, "git push") {
				t.Fatalf("attention = %q %q", st, detail)
			}
			if err := e.srv.Decide(got.ID, verdict, "  because  "); err != nil {
				t.Fatal(err)
			}
			r := awaitApprove(t, ch)
			if r.Decision != verdict || r.Reason != "because" || !r.SignedOn || r.Identity != alice {
				t.Fatalf("reply = %+v", r)
			}
			if n := len(e.srv.Approvals()); n != 0 {
				t.Fatalf("still pending: %d", n)
			}
			if st, _ := e.attention(t, 31); st != "" {
				t.Fatalf("attention after decision = %q", st)
			}
			if err := e.srv.Decide(got.ID, verdict, ""); !errors.Is(err, hooks.ErrApprovalNotFound) {
				t.Fatalf("second decide err = %v", err)
			}
		})
	}
}

func TestApproveDecideErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.approving(hooks.ApprovalMode{Enabled: true, Timeout: time.Minute}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	ch := e.approve(ctx, hook(32))
	got := e.pending(t, 1)[0]
	if got.Identity != "" {
		t.Fatalf("unsigned identity = %q", got.Identity)
	}
	if err := e.srv.Decide(got.ID, "maybe", ""); !errors.Is(err, hooks.ErrInvalidDecision) {
		t.Fatalf("invalid decision err = %v", err)
	}
	if err := e.srv.Decide("appr-nope", wire.DecisionAllow, ""); !errors.Is(err, hooks.ErrApprovalNotFound) {
		t.Fatalf("unknown id err = %v", err)
	}
	if n := len(e.srv.Approvals()); n != 1 {
		t.Fatalf("invalid decision dropped the approval: %d pending", n)
	}
	cancel()
	awaitApprove(t, ch)
}

func TestApproveTimeoutFallsBack(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.approving(hooks.ApprovalMode{Enabled: true, Timeout: 50 * time.Millisecond}, nil)
	e.connect(t, 33, alice)
	r := awaitApprove(t, e.approve(context.Background(), hook(33)))
	if r.Decision != "" {
		t.Fatalf("timeout decided %q", r.Decision)
	}
	if n := len(e.srv.Approvals()); n != 0 {
		t.Fatalf("still pending after timeout: %d", n)
	}
	if st, _ := e.attention(t, 33); st != wire.AttentionBlocked {
		t.Fatalf("attention after timeout = %q, want blocked (terminal prompt)", st)
	}
}

func TestApproveDisabledOrBroken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode hooks.ApprovalMode
		err  error
	}{
		{"disabled", hooks.ApprovalMode{Timeout: time.Minute}, nil},
		{"zero timeout", hooks.ApprovalMode{Enabled: true}, nil},
		{"mode error", hooks.ApprovalMode{Enabled: true, Timeout: time.Minute}, errors.New("db gone")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			e.approving(tt.mode, tt.err)
			start := time.Now()
			r, err := e.srv.Approve(context.Background(), hook(34), approveReq())
			if err != nil || r.Decision != "" || time.Since(start) > time.Second {
				t.Fatalf("reply = %+v, err = %v after %s", r, err, time.Since(start))
			}
			if n := len(e.srv.Approvals()); n != 0 {
				t.Fatalf("pending = %d", n)
			}
		})
	}
	t.Run("no mode option", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t)
		if r, err := e.srv.Approve(context.Background(), hook(35), approveReq()); err != nil || r.Decision != "" {
			t.Fatalf("reply = %+v, err = %v", r, err)
		}
	})
}

func TestApproveCloseReleases(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.approving(hooks.ApprovalMode{Enabled: true, Timeout: time.Minute}, nil)
	ch := e.approve(context.Background(), hook(36))
	e.pending(t, 1)
	e.srv.Close()
	if r := awaitApprove(t, ch); r.Decision != "" {
		t.Fatalf("close decided %q", r.Decision)
	}
	if r, _ := e.srv.Approve(context.Background(), hook(36), approveReq()); r.Decision != "" || len(e.srv.Approvals()) != 0 {
		t.Fatalf("approve after close = %+v", r)
	}
}

// TestServeApproveDisconnect covers the wire path: a hook that hangs up
// abandons its approval.
func TestServeApproveDisconnect(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.approving(hooks.ApprovalMode{Enabled: true, Timeout: time.Minute}, nil)
	srvConn, cliConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer srvConn.Close()
		done <- e.srv.Serve(context.Background(), srvConn, bufio.NewReader(srvConn), hook(37))
	}()
	if err := wire.WriteLine(cliConn, approveReq()); err != nil {
		t.Fatal(err)
	}
	e.pending(t, 1)
	_ = cliConn.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not return after disconnect")
	}
	if n := len(e.srv.Approvals()); n != 0 {
		t.Fatalf("pending after disconnect = %d", n)
	}
}

func TestServeApproveDecision(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.approving(hooks.ApprovalMode{Enabled: true, Timeout: time.Minute}, nil)
	srvConn, cliConn := net.Pipe()
	t.Cleanup(func() { _ = cliConn.Close() })
	go func() {
		defer srvConn.Close()
		_ = e.srv.Serve(context.Background(), srvConn, bufio.NewReader(srvConn), hook(38))
	}()
	if err := wire.WriteLine(cliConn, approveReq()); err != nil {
		t.Fatal(err)
	}
	if err := e.srv.Decide(e.pending(t, 1)[0].ID, wire.DecisionAllow, ""); err != nil {
		t.Fatal(err)
	}
	var r wire.HookReply
	if err := wire.ReadLine(bufio.NewReader(cliConn), &r); err != nil {
		t.Fatal(err)
	}
	if r.Decision != wire.DecisionAllow {
		t.Fatalf("reply = %+v", r)
	}
}
