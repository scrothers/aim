package mcpserver_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/wire"
)

func TestWaitToolWakesOnSend(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	waiter := e.client(t, "w")
	sender := e.client(t, "s")
	register(t, waiter, "bob@github.com/acme/api", "dev")
	register(t, sender, "amy@github.com/acme/api", "dev")
	ok(t, waiter, "sign_on", map[string]any{"address": "bob@github.com/acme/api"})
	ok(t, sender, "sign_on", map[string]any{"address": "amy@github.com/acme/api"})

	type out struct {
		m map[string]any
	}
	done := make(chan out, 1)
	go func() {
		done <- out{ok(t, waiter, "wait", map[string]any{"timeout_seconds": 30})}
	}()
	select {
	case <-done:
		t.Fatal("wait returned before any mail was sent")
	case <-time.After(100 * time.Millisecond):
	}
	ok(t, sender, "send", map[string]any{"to": []string{"bob@github.com/acme/api"}, "subject": "ping", "body": "b"})
	select {
	case o := <-done:
		items, _ := o.m["items"].([]any)
		if o.m["timed_out"] != false || len(items) != 1 || items[0].(map[string]any)["subject"] != "ping" {
			t.Fatalf("wait = %v", o.m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not wake on send")
	}
}

func TestWaitToolTimeoutAndValidation(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).client(t, "a")
	// Anonymous callers must name an address.
	fail(t, cs, "wait", map[string]any{"timeout_seconds": 1}, "address is required")
	fail(t, cs, "wait", map[string]any{"address": "github.com/acme", "timeout_seconds": -1}, "negative")
	fail(t, cs, "wait", map[string]any{"address": "github.com/acme", "since_id": "nope"}, "since_id")
	start := time.Now()
	m := ok(t, cs, "wait", map[string]any{"address": "github.com/acme", "timeout_seconds": 1})
	if m["timed_out"] != true || len(m["items"].([]any)) != 0 {
		t.Fatalf("wait = %v", m)
	}
	if el := time.Since(start); el < 900*time.Millisecond {
		t.Fatalf("returned after %v, before the timeout", el)
	}
}

func TestWaitToolProgress(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	entry := e.reg.Add("conn-p", wire.Preamble{V: wire.Version, SessionID: "sess-p", Machine: "box"}, time.Now())
	t.Cleanup(func() { e.reg.Remove("conn-p") })
	ct, st := mcp.NewInMemoryTransports()
	ss, err := mcpserver.New(e.svc, entry, "test", mcpserver.WithProgressInterval(50*time.Millisecond)).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	var progress atomic.Int64
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			if req.Params.ProgressToken == "tok" {
				progress.Add(1)
			}
		},
	})
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close(); _ = ss.Wait() })

	params := &mcp.CallToolParams{Name: "wait", Arguments: map[string]any{"address": "github.com/acme", "timeout_seconds": 1}}
	params.SetProgressToken("tok")
	res, err := cs.CallTool(ctx, params)
	if err != nil || res.IsError {
		t.Fatalf("wait: %v %v", err, res)
	}
	if n := progress.Load(); n < 2 {
		t.Fatalf("progress notifications = %d, want several", n)
	}
}
