//go:build e2e

// Package hooks_test drives real aim daemon, shim and hook processes through
// the agent hook flow.
package hooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// launcherEnv makes the test binary act as a launcher that runs `aim mcp` as
// its child. Hooks and shims correlate by agent pid, and every shim started
// directly from the test would share the test's pid; a launcher gives each
// simulated agent its own.
const launcherEnv = "AIM_E2E_LAUNCH_BIN"

func launch(bin string) int {
	cmd := exec.Command(bin, "mcp")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return 1
	}
	return 0
}

func run(m *testing.M) int {
	if bin := os.Getenv(launcherEnv); bin != "" {
		return launch(bin)
	}
	dir, err := os.MkdirTemp("", "aim-e2e-bin-")
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

type daemon struct {
	cmd    *exec.Cmd
	socket string
}

// startDaemon runs a daemon on a short-path unix socket (the HTTP listener
// takes an ephemeral port; TCP is off).
func startDaemon(t *testing.T) *daemon {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "aimh")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	d := &daemon{socket: filepath.Join(dir, "a.sock")}
	d.cmd = exec.Command(aimBin, "daemon", "--db", filepath.Join(dir, "aim.db"),
		"--socket", d.socket, "--listen", "", "--http", "127.0.0.1:0")
	d.cmd.Env = append(os.Environ(), "XDG_STATE_HOME="+dir)
	d.cmd.Stderr = os.Stderr
	if err := d.cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- d.cmd.Wait() }()
	t.Cleanup(func() {
		_ = d.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = d.cmd.Process.Kill()
			<-done
		}
		_ = os.RemoveAll(dir)
	})
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(d.socket); err == nil {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon socket never appeared")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

type client struct {
	t  *testing.T
	cs *mcp.ClientSession
}

func startShim(ctx context.Context, t *testing.T, session, addr string) *client {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	cmd := exec.Command(self)
	cmd.Env = append(os.Environ(), launcherEnv+"="+aimBin, "CLAUDE_CODE_SESSION_ID="+session, "AIM_ADDR="+addr)
	cmd.Dir = t.TempDir()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil).
		Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 3 * time.Second}, nil)
	if err != nil {
		t.Fatalf("connect shim %s: %v", session, err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return &client{t: t, cs: cs}
}

func (c *client) call(ctx context.Context, name string, args map[string]any) {
	c.t.Helper()
	res, err := c.cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		c.t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		c.t.Fatalf("%s: tool error: %+v", name, res.Content)
	}
}

// hookCmd builds an `aim hook claude [args]` process with payload on stdin.
func hookCmd(ctx context.Context, d *daemon, payload string, args ...string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
	cmd := exec.CommandContext(ctx, aimBin, append([]string{"hook", "claude"}, args...)...)
	cmd.Env = append(os.Environ(), "AIM_ADDR="+d.socket, "AIM_HOOK_DEBUG=1")
	cmd.Stdin = strings.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	return cmd, &stdout, &stderr
}

func runHook(ctx context.Context, t *testing.T, d *daemon, payload string) string {
	t.Helper()
	cmd, stdout, stderr := hookCmd(ctx, d, payload)
	if err := cmd.Run(); err != nil {
		t.Fatalf("hook: %v\nstderr: %s", err, stderr)
	}
	if stderr.Len() != 0 {
		t.Fatalf("hook stderr: %s", stderr)
	}
	return stdout.String()
}

func additionalContext(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		Out struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("hook stdout is not JSON: %q: %v", out, err)
	}
	return v.Out.Context
}

// TestHookAttentionAndNudge covers the status event (silent) and the
// mid-turn PostToolUse nudge (once per mail).
func TestHookAttentionAndNudge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	d := startDaemon(t)
	a := startShim(ctx, t, "sess-attn", d.socket)
	b := startShim(ctx, t, "sess-attn-other", d.socket)

	const (
		alice = "alice@github.com/acme/tool"
		bob   = "bob@github.com/acme/tool"
	)
	a.call(ctx, "register", map[string]any{"address": alice, "role": "dev", "description": "d"})
	b.call(ctx, "register", map[string]any{"address": bob, "role": "dev", "description": "d"})
	a.call(ctx, "sign_on", map[string]any{"address": alice})
	b.call(ctx, "sign_on", map[string]any{"address": bob})

	cwd := t.TempDir()
	payload := func(event, extra string) string {
		return fmt.Sprintf(`{"session_id":"sess-attn","cwd":%q,"hook_event_name":%q%s}`, cwd, event, extra)
	}

	notify := payload("Notification", `,"notification_type":"permission_prompt","message":"needs permission"`)
	if out := runHook(ctx, t, d, notify); out != "" {
		t.Fatalf("notification printed %q", out)
	}

	post := payload("PostToolUse", `,"tool_use_id":"tu1","tool_name":"Bash"`)
	if out := runHook(ctx, t, d, post); out != "" {
		t.Fatalf("post-tool without mail printed %q", out)
	}
	b.call(ctx, "send", map[string]any{"to": []string{alice}, "subject": "midturn-subject", "body": "hi"})
	out := runHook(ctx, t, d, post)
	if got := additionalContext(t, out); !strings.Contains(got, "midturn-subject") {
		t.Fatalf("post-tool context does not name the subject: %q", got)
	}
	if out := runHook(ctx, t, d, post); out != "" {
		t.Fatalf("second post-tool nudged again: %q", out)
	}
}

func TestHookFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	d := startDaemon(t)
	a := startShim(ctx, t, "sess-e2e", d.socket)
	b := startShim(ctx, t, "sess-other", d.socket)

	const (
		alice = "alice@github.com/acme/tool"
		bob   = "bob@github.com/acme/tool"
	)
	a.call(ctx, "register", map[string]any{"address": alice, "role": "dev", "description": "d"})
	b.call(ctx, "register", map[string]any{"address": bob, "role": "dev", "description": "d"})
	a.call(ctx, "sign_on", map[string]any{"address": alice})
	b.call(ctx, "sign_on", map[string]any{"address": bob})

	cwd := t.TempDir()
	payload := func(event string) string {
		return fmt.Sprintf(`{"session_id":"sess-e2e","cwd":%q,"hook_event_name":%q,"source":"startup"}`, cwd, event)
	}

	out := runHook(ctx, t, d, payload("SessionStart"))
	if got := additionalContext(t, out); !strings.Contains(got, alice) {
		t.Fatalf("session start context does not name %s: %q", alice, got)
	}

	if out := runHook(ctx, t, d, payload("UserPromptSubmit")); out != "" {
		t.Fatalf("prompt without mail printed %q", out)
	}

	b.call(ctx, "send", map[string]any{"to": []string{alice}, "subject": "ping", "body": "hello alice"})
	out = runHook(ctx, t, d, payload("UserPromptSubmit"))
	if got := additionalContext(t, out); got == "" {
		t.Fatalf("prompt with mail printed nothing")
	}
	if out := runHook(ctx, t, d, payload("UserPromptSubmit")); out != "" {
		t.Fatalf("second prompt nudged again: %q", out)
	}

	// Watch: exits 2 with the new mail on stderr shortly after a send.
	wcmd, wout, werr := hookCmd(ctx, d, payload("Stop"), "watch")
	if err := wcmd.Start(); err != nil {
		t.Fatalf("start watch: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- wcmd.Wait() }()
	time.Sleep(500 * time.Millisecond)
	b.call(ctx, "send", map[string]any{"to": []string{alice}, "subject": "wake up", "body": "second mail"})
	select {
	case err := <-done:
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 2 {
			t.Fatalf("watch exit = %v, want status 2\nstderr: %s", err, werr)
		}
	case <-time.After(5 * time.Second):
		_ = wcmd.Process.Kill()
		<-done
		t.Fatalf("watch did not wake within 5s\nstderr: %s", werr)
	}
	if strings.TrimSpace(werr.String()) == "" || wout.Len() != 0 {
		t.Fatalf("watch stdout=%q stderr=%q", wout, werr)
	}
}
