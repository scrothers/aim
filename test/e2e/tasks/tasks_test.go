//go:build e2e

// Package tasks_test drives real aim daemon and shim processes through the
// task board: assignment, claims kept alive by activity, and lease decay.
package tasks_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var aimBin string

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "aim-e2e-tasks-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mktemp:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	aimBin = filepath.Join(dir, "aim")
	out, err := exec.Command("go", "build", "-o", aimBin, "github.com/scrothers/aim/cmd/aim").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build aim: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return addr
}

// isolatedEnv points HOME and XDG_STATE_HOME at dir so no process touches
// the real ~/.local/state/aim or agent configuration.
func isolatedEnv(dir string, extra ...string) []string {
	env := append(os.Environ(), "HOME="+dir, "XDG_STATE_HOME="+filepath.Join(dir, "state"))
	return append(env, extra...)
}

// startDaemon runs a daemon with tiny task decay thresholds and waits until
// it serves.
func startDaemon(t *testing.T) (socket string) {
	t.Helper()
	dir := t.TempDir()
	socket = filepath.Join(dir, "aim.sock")
	web := freeAddr(t)
	cmd := exec.Command(aimBin, "daemon",
		"--db", filepath.Join(dir, "aim.db"), "--socket", socket, "--listen", "", "--http", web,
		"--backup-dir", filepath.Join(dir, "backups"),
		"--task-idle-after", "1s", "--task-stale-after", "2s", "--task-abandon-after", "3s", "--task-max-hold", "1h")
	cmd.Env = isolatedEnv(dir)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for {
		if healthy(web) {
			if _, err := os.Stat(socket); err == nil {
				return socket
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func healthy(web string) bool {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+web+"/healthz", http.NoBody)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type client struct {
	t  *testing.T
	cs *mcp.ClientSession
}

func startShim(ctx context.Context, t *testing.T, session, socket string) *client {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command(aimBin, "mcp")
	cmd.Env = isolatedEnv(dir, "CLAUDE_CODE_SESSION_ID="+session, "AIM_ADDR="+socket)
	cmd.Dir = dir
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil).
		Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 3 * time.Second}, nil)
	if err != nil {
		t.Fatalf("connect shim %s: %v", session, err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return &client{t: t, cs: cs}
}

func (c *client) call(ctx context.Context, name string, args map[string]any, out any) {
	c.t.Helper()
	res, err := c.cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		c.t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		c.t.Fatalf("%s: tool error: %+v", name, res.Content)
	}
	if out == nil {
		return
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		c.t.Fatalf("%s: marshal: %v", name, err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		c.t.Fatalf("%s: decode %s: %v", name, b, err)
	}
}

func (c *client) signOn(ctx context.Context, addr string) {
	c.t.Helper()
	c.call(ctx, "register", map[string]any{"address": addr, "role": "dev", "description": "d"}, nil)
	c.call(ctx, "sign_on", map[string]any{"address": addr}, nil)
}

type task struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	Assignee       string     `json:"assignee"`
	Claimant       string     `json:"claimant"`
	Health         string     `json:"health"`
	LastActivityAt *time.Time `json:"last_activity_at"`
	Events         []struct {
		Kind  string `json:"kind"`
		Actor string `json:"actor"`
		Note  string `json:"note"`
	} `json:"events"`
}

type listOut struct {
	Items []struct {
		ID      string `json:"id"`
		From    string `json:"from"`
		Subject string `json:"subject"`
	} `json:"items"`
}

const (
	alice = "alice@github.com/acme/api"
	bob   = "bob@github.com/acme/api"
	carol = "carol@github.com/acme/api"
)

func TestTaskLeaseDecay(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	socket := startDaemon(t)
	a := startShim(ctx, t, "sess-a", socket)
	b := startShim(ctx, t, "sess-b", socket)
	a.signOn(ctx, alice)
	b.signOn(ctx, bob)

	// A creates and assigns; B is mailed and claims, then starts.
	var tk task
	a.call(ctx, "task_create", map[string]any{"title": "Port the parser", "assignee": bob, "scope": "github.com/acme/api"}, &tk)
	var inbox listOut
	b.call(ctx, "list", map[string]any{"address": bob}, &inbox)
	if len(inbox.Items) != 1 || inbox.Items[0].Subject != "Task assigned: Port the parser" || inbox.Items[0].From != alice {
		t.Fatalf("bob inbox = %+v", inbox.Items)
	}
	b.call(ctx, "task_update", map[string]any{"task_id": tk.ID, "action": "claim"}, &tk)
	b.call(ctx, "task_update", map[string]any{"task_id": tk.ID, "action": "start"}, &tk)
	if tk.Status != "started" || tk.Claimant != bob || tk.Health != "active" || tk.LastActivityAt == nil {
		t.Fatalf("started = %+v", tk)
	}

	// B's ordinary tool calls keep the lease fresh: across several stale
	// periods its health never leaves active or idle.
	first := *tk.LastActivityAt
	for range 12 {
		time.Sleep(400 * time.Millisecond)
		b.call(ctx, "whoami", map[string]any{}, nil)
		var seen task
		a.call(ctx, "task_get", map[string]any{"task_id": tk.ID}, &seen)
		if seen.Status != "started" || (seen.Health != "active" && seen.Health != "idle") {
			t.Fatalf("held task decayed while bob was active: %+v", seen)
		}
		tk = seen
	}
	if !tk.LastActivityAt.After(first) {
		t.Fatalf("last_activity_at %v did not advance past %v", tk.LastActivityAt, first)
	}

	// B disconnects: the lease decays to abandoned and A gets the notice,
	// sent from B's identity.
	if err := b.cs.Close(); err != nil {
		t.Logf("close bob: %v", err)
	}
	var notice string
	deadline := time.Now().Add(30 * time.Second)
	for notice == "" {
		if time.Now().After(deadline) {
			t.Fatal("no abandonment notice for alice")
		}
		time.Sleep(200 * time.Millisecond)
		var l listOut
		a.call(ctx, "list", map[string]any{"address": alice}, &l)
		for _, it := range l.Items {
			if strings.HasPrefix(it.Subject, "[AIM] Task abandoned: ") && it.From == bob {
				notice = it.ID
			}
		}
	}
	var msg struct {
		Body string `json:"body"`
	}
	a.call(ctx, "read", map[string]any{"message_id": notice}, &msg)
	if !strings.Contains(msg.Body, tk.ID) || !strings.Contains(msg.Body, "automatically") {
		t.Errorf("notice body = %q", msg.Body)
	}
	// Decode into fresh values: omitted fields must read as cleared.
	var ab task
	a.call(ctx, "task_get", map[string]any{"task_id": tk.ID}, &ab)
	last := ab.Events[len(ab.Events)-1]
	if ab.Status != "abandoned" || ab.Claimant != "" || ab.Assignee != "" || last.Kind != "decayed" || last.Actor != bob {
		t.Fatalf("abandoned = %+v", ab)
	}

	// A third agent claims it.
	c := startShim(ctx, t, "sess-c", socket)
	c.signOn(ctx, carol)
	var claimed task
	c.call(ctx, "task_update", map[string]any{"task_id": tk.ID, "action": "claim"}, &claimed)
	if claimed.Status != "claimed" || claimed.Claimant != carol {
		t.Fatalf("carol claim = %+v", claimed)
	}
}
