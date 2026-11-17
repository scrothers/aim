package shim_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/shim"
	"github.com/scrothers/aim/internal/wire"
)

const waitFor = 5 * time.Second

// flakyDaemon is a fake aim daemon that can drop its connections and stop and
// restart listening on the same socket.
type flakyDaemon struct {
	t    *testing.T
	sock string

	mu    sync.Mutex
	ln    net.Listener
	conns []net.Conn
	pres  []wire.Preamble
	lines chan string // every line received, tagged "<conn index>:<line>"
}

func newFlaky(t *testing.T) *flakyDaemon {
	t.Helper()
	dir, err := os.MkdirTemp("", "aimshim")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	d := &flakyDaemon{t: t, sock: filepath.Join(dir, "d.sock"), lines: make(chan string, 256)}
	d.up()
	t.Cleanup(d.down)
	return d
}

func (d *flakyDaemon) up() {
	d.t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", d.sock)
	if err != nil {
		d.t.Fatal(err)
	}
	d.mu.Lock()
	d.ln = ln
	d.mu.Unlock()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go d.serve(c)
		}
	}()
}

// down stops listening and drops every connection.
func (d *flakyDaemon) down() {
	d.mu.Lock()
	ln, conns := d.ln, d.conns
	d.ln, d.conns = nil, nil
	d.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	for _, c := range conns {
		_ = c.Close()
	}
}

func (d *flakyDaemon) preambles() []wire.Preamble {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]wire.Preamble(nil), d.pres...)
}

func (d *flakyDaemon) serve(c net.Conn) {
	br := bufio.NewReader(c)
	p, err := wire.ReadPreamble(br)
	if err != nil {
		_ = c.Close()
		return
	}
	d.mu.Lock()
	idx := len(d.pres)
	d.pres = append(d.pres, p)
	d.conns = append(d.conns, c)
	d.mu.Unlock()
	tag := string(rune('0' + idx))
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		d.lines <- tag + ":" + strings.TrimSpace(line)
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name string `json:"name"`
				Pad  string `json:"pad"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(line), &m) != nil || len(m.ID) == 0 || m.Method == "" {
			continue
		}
		var res string
		switch {
		case m.Method == "initialize":
			res = `{"protocolVersion":"x"}`
		case m.Params.Name == "hang":
			continue
		default:
			res = `{"conn":` + tag + `,"padlen":` + itoa(len(m.Params.Pad)) + `}`
		}
		_, _ = io.WriteString(c, `{"jsonrpc":"2.0","id":`+string(m.ID)+`,"result":`+res+"}\n")
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// expect waits for a received line containing sub and returns it.
func (d *flakyDaemon) expect(sub string) string {
	d.t.Helper()
	timeout := time.After(waitFor)
	for {
		select {
		case l := <-d.lines:
			if strings.Contains(l, sub) {
				return l
			}
		case <-timeout:
			d.t.Fatalf("daemon never received %q", sub)
		}
	}
}

// client drives the shim like Claude would.
type client struct {
	t     *testing.T
	in    *io.PipeWriter
	out   chan string
	done  chan error
	stop  context.CancelFunc
	cfg   shim.Config
	check sync.Mutex
	bad   []string
}

type lineWriter struct{ c *client }

func (w lineWriter) Write(p []byte) (int, error) {
	s := string(p)
	if !strings.HasSuffix(s, "\n") || strings.Count(s, "\n") != 1 || !json.Valid([]byte(s)) {
		w.c.check.Lock()
		w.c.bad = append(w.c.bad, s)
		w.c.check.Unlock()
	}
	w.c.out <- strings.TrimSpace(s)
	return len(p), nil
}

func startShim(t *testing.T, d *flakyDaemon, cfg shim.Config) *client {
	t.Helper()
	cfg.Addr = d.sock
	cfg.BackoffMin, cfg.BackoffMax = 10*time.Millisecond, 50*time.Millisecond
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	c := &client{t: t, in: pw, out: make(chan string, 256), done: make(chan error, 1), stop: cancel, cfg: cfg}
	go func() {
		c.done <- shim.Run(ctx, cfg, wire.Preamble{SessionID: "sess-1", Machine: "m", ProjectDir: "/p"}, pr, lineWriter{c})
	}()
	t.Cleanup(func() {
		cancel()
		_ = pw.Close()
		c.check.Lock()
		defer c.check.Unlock()
		if len(c.bad) > 0 {
			t.Errorf("stdout carried non-JSON-RPC output: %q", c.bad)
		}
	})
	return c
}

func (c *client) send(line string) {
	c.t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

// recv returns the next output line.
func (c *client) recv() map[string]any {
	c.t.Helper()
	select {
	case l := <-c.out:
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			c.t.Fatalf("bad output %q: %v", l, err)
		}
		return m
	case <-time.After(waitFor):
		c.t.Fatal("no output from shim")
		return nil
	}
}

func (c *client) handshake() {
	c.t.Helper()
	c.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"t"}}}`)
	if m := c.recv(); m["id"] != float64(1) {
		c.t.Fatalf("initialize response = %v", m)
	}
	c.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
}

func isUnavailable(m map[string]any) bool {
	e, ok := m["error"].(map[string]any)
	return ok && e["code"] == float64(shim.UnavailableCode) && e["message"] == shim.UnavailableMessage
}

func TestReconnectReplaysHandshake(t *testing.T) {
	t.Parallel()
	d := newFlaky(t)
	c := startShim(t, d, shim.Config{})
	c.handshake()
	c.send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"x"}}`)
	if m := c.recv(); m["id"] != float64(2) {
		t.Fatalf("got %v", m)
	}

	d.down()
	d.up()

	// The fresh connection sees the resumed preamble, a replayed initialize
	// under a new id, then initialized.
	if l := d.expect(`"id":"aim-replay-1"`); !strings.HasPrefix(l, "1:") || !strings.Contains(l, `"method":"initialize"`) {
		t.Fatalf("replayed initialize = %s", l)
	}
	d.expect(`1:{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	c.send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"x"}}`)
	m := c.recv()
	if m["id"] != float64(3) || m["result"].(map[string]any)["conn"] != float64(1) {
		t.Fatalf("after reconnect got %v (the replayed initialize response must be swallowed)", m)
	}
	ps := d.preambles()
	if len(ps) != 2 || ps[0].Resume || !ps[1].Resume || ps[1].SessionID != "sess-1" {
		t.Fatalf("preambles = %+v", ps)
	}
}

func TestInflightRequestFailsOnDrop(t *testing.T) {
	t.Parallel()
	d := newFlaky(t)
	c := startShim(t, d, shim.Config{})
	c.handshake()
	c.send(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"hang"}}`)
	d.expect(`"id":5`)
	d.down()
	m := c.recv()
	if m["id"] != float64(5) || !isUnavailable(m) {
		t.Fatalf("got %v, want unavailable error for id 5", m)
	}
	d.up()
	c.send(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"x"}}`)
	if m := c.recv(); m["id"] != float64(6) || m["error"] != nil {
		t.Fatalf("got %v", m)
	}
}

func TestRequestsQueueWhileDown(t *testing.T) {
	t.Parallel()
	d := newFlaky(t)
	c := startShim(t, d, shim.Config{})
	c.handshake()
	d.down()
	time.Sleep(50 * time.Millisecond) // let the shim notice
	c.send(`{"jsonrpc":"2.0","id":"a","method":"tools/call","params":{"name":"x"}}`)
	c.send(`{"jsonrpc":"2.0","id":"b","method":"tools/call","params":{"name":"x"}}`)
	c.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"zzz"}}`) // dropped while down
	d.up()
	got := map[any]bool{}
	for range 2 {
		m := c.recv()
		if m["error"] != nil {
			t.Fatalf("queued request failed: %v", m)
		}
		got[m["id"]] = true
	}
	if !got["a"] || !got["b"] {
		t.Fatalf("responses = %v", got)
	}
	// Order: replayed initialize, initialized, then the queued requests.
	d.expect("aim-replay")
	d.expect("notifications/initialized")
	d.expect(`"id":"a"`)
	d.expect(`"id":"b"`)
}

func TestQueuedRequestTimesOut(t *testing.T) {
	t.Parallel()
	d := newFlaky(t)
	c := startShim(t, d, shim.Config{ReconnectTimeout: 150 * time.Millisecond})
	c.handshake()
	d.down()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	c.send(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"x"}}`)
	m := c.recv()
	if m["id"] != float64(9) || !isUnavailable(m) {
		t.Fatalf("got %v", m)
	}
	if el := time.Since(start); el < 100*time.Millisecond {
		t.Fatalf("failed after %v, want it to wait out the reconnect timeout", el)
	}
	// The shim keeps retrying: once the daemon is back, calls work again.
	d.up()
	time.Sleep(200 * time.Millisecond)
	c.send(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"x"}}`)
	if m := c.recv(); m["id"] != float64(10) || m["error"] != nil {
		t.Fatalf("got %v", m)
	}
}

func TestLargeLinesBothWays(t *testing.T) {
	t.Parallel()
	d := newFlaky(t)
	c := startShim(t, d, shim.Config{})
	c.handshake()
	const n = 3 << 20
	c.send(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"x","pad":"` + strings.Repeat("a", n) + `"}}`)
	m := c.recv()
	if m["id"] != float64(11) || m["result"].(map[string]any)["padlen"] != float64(n) {
		t.Fatalf("large request got %v", m)
	}
}

func TestStdinEOFWhileDownFinishes(t *testing.T) {
	t.Parallel()
	d := newFlaky(t)
	c := startShim(t, d, shim.Config{})
	c.handshake()
	d.down()
	time.Sleep(50 * time.Millisecond)
	c.send(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"x"}}`)
	_ = c.in.Close()
	if m := c.recv(); m["id"] != float64(12) || !isUnavailable(m) {
		t.Fatalf("got %v", m)
	}
	select {
	case err := <-c.done:
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(waitFor):
		t.Fatal("Run did not return after stdin EOF with the daemon down")
	}
}
