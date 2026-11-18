package mcpserver_test

import (
	"context"
	"errors"
	"sync"
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
	ok(t, cs, "agent_sign_on", map[string]any{"address": "bob@github.com/acme/api"})

	params := &mcp.CallToolParams{Name: "mail_wait", Arguments: map[string]any{"timeout_seconds": 1}}
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

type recordToucher struct {
	mu       sync.Mutex
	sessions []string
}

func (r *recordToucher) Touch(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = append(r.sessions, sessionID)
	return errors.New("ignored")
}

// TestToolCallsTouchTasks checks every tool call is task lease activity for
// the connection's session, and that a failed touch never fails the call.
func TestToolCallsTouchTasks(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rt := &recordToucher{}
	cs := e.ClientWith(t, "a", []mcpserver.Option{mcpserver.WithTaskToucher(rt)}, nil)
	ok(t, cs, "agent_whoami", map[string]any{})
	ok(t, cs, "agent_roster", map[string]any{"address": "github.com/acme"})
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.sessions) != 2 || rt.sessions[0] != "sess-a" {
		t.Errorf("touched = %v", rt.sessions)
	}
}
