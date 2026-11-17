package daemon_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/shim"
	"github.com/scrothers/aim/internal/wire"
)

// callRetry calls a tool, retrying while the shim reports the daemon
// unavailable, as an agent told to "retry" would.
func callRetry(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err == nil {
			if res.IsError {
				t.Fatalf("%s: tool error %v", name, res.Content)
			}
			return res
		}
		if !strings.Contains(err.Error(), shim.UnavailableMessage) || time.Now().After(deadline) {
			t.Fatalf("%s: %v", name, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestShimSurvivesDaemonRestart(t *testing.T) {
	t.Parallel()
	cfg := testCfg(t)
	r1 := start(t, cfg)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	scfg := shim.Config{
		Addr: cfg.Socket, ReconnectTimeout: 20 * time.Second,
		BackoffMin: 10 * time.Millisecond, BackoffMax: 100 * time.Millisecond,
	}
	go func() {
		_ = shim.Run(ctx, scfg, wire.Preamble{SessionID: "sess-restart", Machine: "box", ProjectDir: "/p"}, inR, outW)
	}()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).
		Connect(ctx, &mcp.IOTransport{Reader: outR, Writer: inW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	const id = "alice@github.com/acme/api"
	call(t, cs, "register", map[string]any{"address": id, "role": "dev", "description": "d"})
	call(t, cs, "sign_on", map[string]any{"address": id, "away": "brb"})

	r1.stop(t)
	r2 := start(t, cfg) // same database, same socket path

	res := callRetry(t, cs, "whoami", map[string]any{})
	m, ok := res.StructuredContent.(map[string]any)
	if !ok || m["session_id"] != "sess-restart" || m["signed_on_as"] != id {
		t.Fatalf("whoami after restart = %v, want signed on as %s", res.StructuredContent, id)
	}

	// Mail tools that need the sign-on work without signing on again.
	callRetry(t, cs, "send", map[string]any{"to": []string{id}, "subject": "after restart", "body": "hi"})

	// An explicit sign-off must not be resurrected by a second restart.
	callRetry(t, cs, "sign_off", map[string]any{})
	r2.stop(t)
	_ = start(t, cfg)
	res = callRetry(t, cs, "whoami", map[string]any{})
	if m, _ := res.StructuredContent.(map[string]any); m["signed_on_as"] != nil {
		t.Fatalf("whoami after sign_off and restart = %v, want signed off", res.StructuredContent)
	}
}
