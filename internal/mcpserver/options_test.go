package mcpserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver"
)

// TestOptionsReachFeatures checks that New hands each option to the feature
// package that uses it: progress to the wait tool, the debounce and watcher
// exit to subscriptions.
func TestOptionsReachFeatures(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// resources/subscribe is a plain request (handled by the server's
	// subscription handler) only before SEP-2575.
	e.Protocol = "2025-11-25"
	exit := make(chan struct{})
	progress := make(chan struct{}, 1)
	copts := &mcp.ClientOptions{
		ProgressNotificationHandler: func(context.Context, *mcp.ProgressNotificationClientRequest) {
			select {
			case progress <- struct{}{}:
			default:
			}
		},
	}
	opts := []mcpserver.Option{
		mcpserver.WithProgressInterval(20 * time.Millisecond),
		mcpserver.WithSubscribeDebounce(10 * time.Millisecond),
		mcpserver.WithWatcherExit(exit),
	}
	cs := e.ClientWith(t, "a", opts, copts)
	register(t, cs, "bob@github.com/acme/api", "dev")
	ok(t, cs, "sign_on", map[string]any{"address": "bob@github.com/acme/api"})

	params := &mcp.CallToolParams{Name: "wait", Arguments: map[string]any{"timeout_seconds": 1}}
	params.SetProgressToken("p1")
	if _, err := cs.CallTool(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	select {
	case <-progress:
	default:
		t.Error("no progress notification within the 1s wait at a 20ms interval")
	}

	if err := cs.Subscribe(context.Background(), &mcp.SubscribeParams{URI: "aim://inbox"}); err != nil {
		t.Fatal(err)
	}
	_ = cs.Close()
	select {
	case <-exit:
	case <-time.After(5 * time.Second):
		t.Fatal("subscription watcher did not exit after the session closed")
	}
}
