//go:build e2e

// Package mail_test drives real aim daemon and shim processes through the
// mail flow.
package mail_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func run(m *testing.M) int {
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

type daemon struct {
	cmd              *exec.Cmd
	socket, tcp, web string
	done             chan error
}

func startDaemon(t *testing.T) *daemon {
	t.Helper()
	dir := t.TempDir()
	d := &daemon{
		socket: filepath.Join(dir, "aim.sock"),
		tcp:    freeAddr(t),
		web:    freeAddr(t),
		done:   make(chan error, 1),
	}
	d.cmd = exec.Command(aimBin, "daemon",
		"--db", filepath.Join(dir, "aim.db"),
		"--socket", d.socket, "--listen", d.tcp, "--http", d.web)
	d.cmd.Stderr = os.Stderr
	if err := d.cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	go func() { d.done <- d.cmd.Wait() }()
	t.Cleanup(func() {
		if d.cmd.ProcessState == nil {
			_ = d.cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-d.done:
			case <-time.After(5 * time.Second):
				_ = d.cmd.Process.Kill()
			}
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for {
		if d.healthz() == http.StatusOK {
			if _, err := os.Stat(d.socket); err == nil {
				return d
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (d *daemon) healthz() int {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+d.web+"/healthz", http.NoBody)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// api calls the HTTP API, with the browser guard headers when guarded, and
// decodes a JSON response into out (if non-nil). It returns the status code.
func (d *daemon) api(ctx context.Context, method, path string, body any, guarded bool, out any) int {
	var rd io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+d.web+path, rd)
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	if guarded {
		req.Header.Set("X-AIM-Request", "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

type client struct {
	t  *testing.T
	cs *mcp.ClientSession
}

func startShim(ctx context.Context, t *testing.T, session, addr string) *client {
	t.Helper()
	cmd := exec.Command(aimBin, "mcp")
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SESSION_ID="+session, "AIM_ADDR="+addr)
	cmd.Dir = t.TempDir()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil).
		Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 3 * time.Second}, nil)
	if err != nil {
		t.Fatalf("connect shim %s: %v", session, err)
	}
	c := &client{t: t, cs: cs}
	t.Cleanup(func() { _ = cs.Close() })
	return c
}

// call invokes a tool and decodes its structured result into out (if non-nil).
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

type header struct {
	ID       string `json:"id"`
	ThreadID string `json:"thread_id"`
	From     string `json:"from"`
	Subject  string `json:"subject"`
}

type listOut struct {
	Items []header `json:"items"`
}

type message struct {
	ID       string   `json:"id"`
	ThreadID string   `json:"thread_id"`
	From     string   `json:"from"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	To       []string `json:"to"`
	Cc       []string `json:"cc"`
}

type rosterOut struct {
	Identities []struct {
		Address string            `json:"address"`
		Online  []json.RawMessage `json:"online"`
	} `json:"identities"`
}

func (r rosterOut) online(addr string) (found bool, n int) {
	for _, e := range r.Identities {
		if e.Address == addr {
			return true, len(e.Online)
		}
	}
	return false, 0
}

func TestMailFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	d := startDaemon(t)
	a := startShim(ctx, t, "sess-a", d.socket)
	b := startShim(ctx, t, "sess-b", d.tcp)

	const (
		coord    = "coordinator@github.com/acme"
		reviewer = "reviewer@github.com/acme/tool"
		org      = "github.com/acme"
		repo     = "github.com/acme/tool"
	)

	a.call(ctx, "register", map[string]any{"address": coord, "role": "lead", "description": "coordinates work"}, nil)
	a.call(ctx, "register", map[string]any{"address": reviewer, "role": "reviewer", "description": "reviews code"}, nil)
	a.call(ctx, "sign_on", map[string]any{"address": coord}, nil)
	b.call(ctx, "sign_on", map[string]any{"address": reviewer}, nil)

	var roster rosterOut
	a.call(ctx, "roster", map[string]any{"address": org}, &roster)
	for _, addr := range []string{coord, reviewer} {
		if found, n := roster.online(addr); !found || n != 1 {
			t.Fatalf("roster %s: found=%v online=%d", addr, found, n)
		}
	}

	var sent message
	a.call(ctx, "send", map[string]any{
		"to": []string{reviewer}, "cc": []string{repo},
		"subject": "please review", "body": "the zebra patch is ready",
	}, &sent)
	if sent.ID == "" {
		t.Fatalf("send returned no id: %+v", sent)
	}

	var unread listOut
	b.call(ctx, "list", map[string]any{"address": reviewer}, &unread)
	if len(unread.Items) != 1 || unread.Items[0].ID != sent.ID {
		t.Fatalf("unread = %+v, want %s", unread.Items, sent.ID)
	}

	var got message
	b.call(ctx, "read", map[string]any{"message_id": sent.ID}, &got)
	if got.Body != "the zebra patch is ready" || got.From != coord {
		t.Fatalf("read = %+v", got)
	}
	b.call(ctx, "list", map[string]any{"address": reviewer}, &unread)
	if len(unread.Items) != 0 {
		t.Fatalf("unread after read = %+v", unread.Items)
	}

	var reply message
	b.call(ctx, "send", map[string]any{
		"to": []string{}, "subject": "Re: please review", "body": "looks good",
		"in_reply_to": sent.ID, "reply_all": true,
	}, &reply)
	if reply.ThreadID != sent.ThreadID {
		t.Fatalf("reply thread %s, want %s", reply.ThreadID, sent.ThreadID)
	}

	var all listOut
	a.call(ctx, "list", map[string]any{"address": org, "state": "all"}, &all)
	seen := map[string]bool{}
	for _, h := range all.Items {
		seen[h.ID] = true
	}
	if !seen[sent.ID] || !seen[reply.ID] {
		t.Fatalf("org subtree list missing messages: %+v", all.Items)
	}
	var thread struct {
		Messages []message `json:"messages"`
	}
	a.call(ctx, "thread", map[string]any{"thread_id": sent.ThreadID}, &thread)
	if len(thread.Messages) != 2 {
		t.Fatalf("thread = %+v", thread.Messages)
	}

	var hits struct {
		Hits []struct {
			MessageID string `json:"message_id"`
		} `json:"hits"`
	}
	a.call(ctx, "search", map[string]any{"address": org, "query": "zebra"}, &hits)
	if len(hits.Hits) == 0 || hits.Hits[0].MessageID != sent.ID {
		t.Fatalf("search hits = %+v", hits.Hits)
	}

	// The agent escalates to the operator, who sees it over HTTP and replies.
	var esc message
	a.call(ctx, "send", map[string]any{
		"to": []string{"operator"}, "subject": "need a decision", "body": "ship the **zebra** patch?",
	}, &esc)
	var inbox listOut
	status := d.api(ctx, http.MethodGet, "/api/messages?address=operator", nil, true, &inbox)
	if status != http.StatusOK || len(inbox.Items) != 1 || inbox.Items[0].ID != esc.ID {
		t.Fatalf("operator inbox: status=%d items=%+v, want %s", status, inbox.Items, esc.ID)
	}
	var shown struct {
		message
		BodyHTML string `json:"body_html"`
	}
	if status := d.api(ctx, http.MethodGet, "/api/messages/"+esc.ID, nil, true, &shown); status != http.StatusOK ||
		!strings.Contains(shown.BodyHTML, "<strong>zebra</strong>") || shown.From != coord {
		t.Fatalf("operator message: status=%d %+v", status, shown)
	}
	req := map[string]any{"to": []string{coord}, "subject": "Re: need a decision", "body": "approved", "in_reply_to": esc.ID}
	var opReply message
	if status := d.api(ctx, http.MethodPost, "/api/messages", req, true, &opReply); status != http.StatusCreated || opReply.From != "operator" {
		t.Fatalf("operator send: status=%d %+v", status, opReply)
	}
	var coordInbox listOut
	a.call(ctx, "list", map[string]any{"address": coord}, &coordInbox)
	if !slices.ContainsFunc(coordInbox.Items, func(h header) bool { return h.ID == opReply.ID && h.From == "operator" }) {
		t.Fatalf("coordinator inbox = %+v, want %s", coordInbox.Items, opReply.ID)
	}
	if status := d.api(ctx, http.MethodPost, "/api/messages", req, false, nil); status != http.StatusForbidden {
		t.Fatalf("POST without X-AIM-Request = %d, want 403", status)
	}

	// B's process exits; the daemon should notice and show reviewer offline.
	if err := b.cs.Close(); err != nil {
		t.Logf("close shim b: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		a.call(ctx, "roster", map[string]any{"address": org}, &roster)
		if found, n := roster.online(reviewer); found && n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("reviewer still online: %+v", roster)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if found, n := roster.online(coord); !found || n != 1 {
		t.Fatalf("coordinator online=%d", n)
	}

	if code := d.healthz(); code != http.StatusOK {
		t.Fatalf("healthz = %d", code)
	}

	_ = a.cs.Close()
	if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	select {
	case err := <-d.done:
		if err != nil {
			t.Fatalf("daemon exit: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("daemon did not exit after SIGTERM")
	}
}
