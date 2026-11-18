package hooks_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/wire"
)

type watchResult struct {
	reply wire.HookReply
	err   error
}

func (e *env) watch(ctx context.Context, p wire.Preamble) <-chan watchResult {
	out := make(chan watchResult, 1)
	go func() {
		r, err := e.srv.Watch(ctx, p, wire.HookRequest{Op: wire.HookOpWatch})
		out <- watchResult{r, err}
	}()
	return out
}

func await(t *testing.T, ch <-chan watchResult) wire.HookReply {
	t.Helper()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("Watch: %v", r.err)
		}
		return r.reply
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not return")
	}
	return wire.HookReply{}
}

// settle gives a started watch time to register and take its first look.
func settle() { time.Sleep(50 * time.Millisecond) }

func TestWatchWakesOnNewMailOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 20, alice)
	e.send(t, bob, alice, "already unread")
	ch := e.watch(context.Background(), hook(20))
	settle()
	select {
	case r := <-ch:
		t.Fatalf("watch woke for old mail: %+v", r)
	default:
	}
	e.send(t, bob, alice, "new arrival")
	r := await(t, ch)
	if !r.Wake || r.Identity != alice || !strings.Contains(r.Context, "new arrival") || strings.Contains(r.Context, "already unread") {
		t.Fatalf("reply = %+v", r)
	}
	// The wake advanced the watermark, so the next prompt is silent.
	if got := e.event(t, hook(20), wire.HookRequest{Event: wire.HookPrompt}); got.Context != "" {
		t.Errorf("prompt after wake = %q", got.Context)
	}
}

func TestWatchWaitsForSignOn(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ch := e.watch(context.Background(), hook(21))
	settle()
	e.connect(t, 21, alice)
	settle() // let the poll see the sign-on and set nothing off
	e.send(t, bob, alice, "after sign-on")
	if r := await(t, ch); !r.Wake || !strings.Contains(r.Context, "after sign-on") {
		t.Fatalf("reply = %+v", r)
	}
}

func TestWatchSupersededAndEnded(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 22, alice)
	first := e.watch(context.Background(), hook(22))
	settle()
	second := e.watch(context.Background(), hook(22))
	if r := await(t, first); !r.Superseded {
		t.Fatalf("first watch = %+v, want superseded", r)
	}
	settle()
	e.event(t, hook(22), wire.HookRequest{Event: wire.HookSessionEnd})
	if r := await(t, second); !r.Superseded || r.Wake {
		t.Fatalf("second watch after session end = %+v", r)
	}
}

func TestWatchEndsWithContextAndUntracked(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if r := await(t, e.watch(context.Background(), wire.Preamble{Kind: wire.KindHook})); r != (wire.HookReply{}) {
		t.Fatalf("untracked watch = %+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := e.watch(ctx, hook(23))
	settle()
	cancel()
	if r := await(t, ch); r.Wake || r.Superseded {
		t.Fatalf("canceled watch = %+v", r)
	}
}

func TestServeWatchEndsWhenHookLeaves(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.connect(t, 24, alice)
	srvConn, cliConn := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer srvConn.Close()
		done <- e.srv.Serve(context.Background(), srvConn, bufio.NewReader(srvConn), hook(24))
	}()
	if err := wire.WriteLine(cliConn, wire.HookRequest{Op: wire.HookOpWatch}); err != nil {
		t.Fatal(err)
	}
	settle()
	_ = cliConn.Close()
	select {
	case <-done: // the reply write fails or succeeds; either way Serve returns
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the hook closed")
	}
}
