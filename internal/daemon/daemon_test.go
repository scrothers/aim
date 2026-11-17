package daemon_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/daemon"
	"github.com/scrothers/aim/internal/wire"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type running struct {
	addrs  daemon.Addrs
	cancel context.CancelFunc
	done   chan error
}

func start(t *testing.T, cfg daemon.Config) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan daemon.Addrs, 1)
	cfg.Ready = func(a daemon.Addrs) { ready <- a }
	done := make(chan error, 1)
	go func() { done <- daemon.Run(ctx, cfg, quiet()) }()
	r := &running{cancel: cancel, done: done}
	select {
	case r.addrs = <-ready:
	case err := <-done:
		cancel()
		t.Fatalf("daemon exited early: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("daemon not ready")
	}
	t.Cleanup(func() { cancel() })
	return r
}

func (r *running) stop(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case err := <-r.done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func testCfg(t *testing.T) daemon.Config {
	t.Helper()
	dir := t.TempDir()
	return daemon.Config{
		DBPath: filepath.Join(dir, "aim.db"),
		Socket: filepath.Join(dir, "run", "aim.sock"),
		Listen: "127.0.0.1:0",
		HTTP:   "127.0.0.1:0",
	}
}

func connect(t *testing.T, network, addr string, p wire.Preamble) (*mcp.ClientSession, net.Conn) {
	t.Helper()
	ctx := context.Background()
	var d net.Dialer
	c, err := d.DialContext(ctx, network, addr)
	if err != nil {
		t.Fatal(err)
	}
	if err := wire.WritePreamble(c, p); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).
		Connect(ctx, &mcp.IOTransport{Reader: c, Writer: c}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return cs, c
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s: tool error %v", name, res.Content)
	}
	return res
}

func exercise(t *testing.T, network, addr, session string) {
	t.Helper()
	cs, _ := connect(t, network, addr, wire.Preamble{SessionID: session, Machine: "box", ProjectDir: "/p"})
	defer func() { _ = cs.Close() }()
	res := call(t, cs, "whoami", map[string]any{})
	m, ok := res.StructuredContent.(map[string]any)
	if !ok || m["session_id"] != session {
		t.Fatalf("whoami = %v", res.StructuredContent)
	}
	id := session + "@github.com/acme/api"
	call(t, cs, "register", map[string]any{"address": id, "role": "r", "description": "d"})
	call(t, cs, "sign_on", map[string]any{"address": id})
	call(t, cs, "send", map[string]any{"to": []string{id}, "subject": "s", "body": "b"})
}

func TestServeUnixAndTCP(t *testing.T) {
	t.Parallel()
	r := start(t, testCfg(t))
	exercise(t, "unix", r.addrs.Socket, "alice")
	exercise(t, "tcp", r.addrs.TCP, "bob")

	resp, err := http.Get("http://" + r.addrs.HTTP + "/healthz") //nolint:noctx // test
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}

	r.stop(t)
	if _, err := os.Stat(r.addrs.Socket); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket not removed: %v", err)
	}
}

func TestBadPreambleClosed(t *testing.T) {
	t.Parallel()
	cfg := testCfg(t)
	cfg.PreambleTimeout = 200 * time.Millisecond
	r := start(t, cfg)

	for name, payload := range map[string]string{"garbage": "not json\n", "silent": ""} {
		c, err := net.Dial("tcp", r.addrs.TCP)
		if err != nil {
			t.Fatal(err)
		}
		if payload != "" {
			_, _ = c.Write([]byte(payload))
		}
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
			t.Fatalf("%s: want EOF, got %v", name, err)
		}
		_ = c.Close()
	}
	r.stop(t)
}

func TestPresenceRemovedOnDisconnect(t *testing.T) {
	t.Parallel()
	r := start(t, testCfg(t))
	cs, _ := connect(t, "unix", r.addrs.Socket, wire.Preamble{SessionID: "carol", Machine: "box"})
	id := "carol@github.com/acme/api"
	call(t, cs, "register", map[string]any{"address": id, "role": "r", "description": "d"})
	call(t, cs, "sign_on", map[string]any{"address": id})

	online := func(c *mcp.ClientSession) int {
		res := call(t, c, "roster", map[string]any{"address": "github.com/acme"})
		ids := res.StructuredContent.(map[string]any)["identities"].([]any)
		return len(ids[0].(map[string]any)["online"].([]any))
	}
	if n := online(cs); n != 1 {
		t.Fatalf("online = %d", n)
	}
	obs, _ := connect(t, "unix", r.addrs.Socket, wire.Preamble{SessionID: "obs"})
	defer func() { _ = obs.Close() }()
	_ = cs.Close()
	deadline := time.Now().Add(5 * time.Second)
	for online(obs) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("presence entry not removed after disconnect")
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.stop(t)
}

func TestStaleSocketReplaced(t *testing.T) {
	t.Parallel()
	cfg := testCfg(t)
	if err := os.MkdirAll(filepath.Dir(cfg.Socket), 0o700); err != nil {
		t.Fatal(err)
	}
	// Create a stale socket file: listen, then leave the file behind.
	l, err := net.Listen("unix", cfg.Socket)
	if err != nil {
		t.Fatal(err)
	}
	ul := l.(*net.UnixListener)
	ul.SetUnlinkOnClose(false)
	_ = ul.Close()
	if _, err := os.Stat(cfg.Socket); err != nil {
		t.Fatalf("stale socket missing: %v", err)
	}
	r := start(t, cfg)
	exercise(t, "unix", r.addrs.Socket, "dave")
	r.stop(t)
}

func TestAlreadyRunning(t *testing.T) {
	t.Parallel()
	cfg := testCfg(t)
	r := start(t, cfg)
	cfg2 := cfg
	cfg2.DBPath = filepath.Join(t.TempDir(), "other.db")
	cfg2.Listen, cfg2.HTTP = "", ""
	err := daemon.Run(context.Background(), cfg2, quiet())
	if !errors.Is(err, daemon.ErrAlreadyRunning) {
		t.Fatalf("err = %v", err)
	}
	r.stop(t)
}

func TestSocketPathNotSocket(t *testing.T) {
	t.Parallel()
	cfg := testCfg(t)
	cfg.Socket = filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(cfg.Socket, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := daemon.Run(context.Background(), cfg, quiet())
	if err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Fatalf("err = %v", err)
	}
}

func TestListenErrors(t *testing.T) {
	t.Parallel()
	cfg := testCfg(t)
	cfg.Listen = "256.0.0.1:1"
	if err := daemon.Run(context.Background(), cfg, quiet()); err == nil {
		t.Fatal("want tcp error")
	}
	cfg = testCfg(t)
	cfg.HTTP = "256.0.0.1:1"
	if err := daemon.Run(context.Background(), cfg, quiet()); err == nil {
		t.Fatal("want http error")
	}
	cfg = testCfg(t)
	cfg.DBPath = t.TempDir() // a directory is not a database
	if err := daemon.Run(context.Background(), cfg, quiet()); err == nil {
		t.Fatal("want store error")
	}
}

func TestSocketOnly(t *testing.T) {
	t.Parallel()
	cfg := testCfg(t)
	cfg.Listen, cfg.HTTP = "", ""
	r := start(t, cfg)
	if r.addrs.TCP != "" || r.addrs.HTTP != "" {
		t.Fatalf("addrs = %+v", r.addrs)
	}
	r.stop(t)
}

// TestHalfClosedClientStillGetsResponse covers a shim that writes its
// preamble and one request, then half-closes: the response must still arrive.
func TestHalfClosedClientStillGetsResponse(t *testing.T) {
	t.Parallel()
	r := start(t, testCfg(t))
	for _, network := range []string{"unix", "tcp"} {
		addr := r.addrs.Socket
		if network == "tcp" {
			addr = r.addrs.TCP
		}
		c, err := net.Dial(network, addr)
		if err != nil {
			t.Fatal(err)
		}
		if err := wire.WritePreamble(c, wire.Preamble{SessionID: "half", Machine: "box", ProjectDir: "/p"}); err != nil {
			t.Fatal(err)
		}
		const initReq = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}` + "\n"
		if _, err := io.WriteString(c, initReq); err != nil {
			t.Fatal(err)
		}
		cw, ok := c.(interface{ CloseWrite() error })
		if !ok {
			t.Fatalf("%s conn has no CloseWrite", network)
		}
		if err := cw.CloseWrite(); err != nil {
			t.Fatal(err)
		}
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		got, err := io.ReadAll(c)
		if err != nil || !strings.Contains(string(got), `"id":1`) || !strings.Contains(string(got), "serverInfo") {
			t.Errorf("%s: response after half-close = %q, %v", network, got, err)
		}
		_ = c.Close()
	}
	r.stop(t)
}
