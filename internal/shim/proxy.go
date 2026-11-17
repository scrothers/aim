package shim

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scrothers/aim/internal/wire"
)

// Defaults for reconnecting.
const (
	// DefaultReconnectTimeout is how long a request may wait for the daemon to
	// come back before it fails with an unavailable error.
	DefaultReconnectTimeout = 30 * time.Second
	defaultBackoffMin       = 100 * time.Millisecond
	defaultBackoffMax       = 2 * time.Second
	replayTimeout           = 10 * time.Second
)

// UnavailableCode and UnavailableMessage are the JSON-RPC error the shim
// returns for requests it cannot deliver because the daemon is down.
const (
	UnavailableCode    = -32000
	UnavailableMessage = "AIM daemon unavailable, retry"
)

const (
	methodInitialize  = "initialize"
	methodInitialized = "notifications/initialized"
)

// rpcHead is the part of a JSON-RPC message the proxy needs.
type rpcHead struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

// parseHead extracts the id and method of a JSON-RPC line. A line that is not
// a JSON object yields a zero head and is forwarded untouched.
func parseHead(line []byte) (id, method string) {
	var h rpcHead
	if json.Unmarshal(line, &h) != nil {
		return "", ""
	}
	if string(h.ID) != "null" {
		id = string(h.ID)
	}
	return id, h.Method
}

// link is one connection to the daemon.
type link struct {
	conn net.Conn
	br   *bufio.Reader
	// dead is set, under proxy.mu, once the reader has seen the connection fail.
	dead bool
	// replayID is the id of the replayed initialize request whose response is
	// swallowed; nil when no replay is pending.
	replayID atomic.Pointer[string]
	replayCh chan error
}

type queued struct {
	line  []byte
	id    string
	timer *time.Timer
}

type proxy struct {
	cfg    Config
	p      wire.Preamble
	stdout io.Writer
	log    *slog.Logger
	ctx    context.Context //nolint:containedctx // lifetime of the proxy; its goroutines outlive any single call

	outMu sync.Mutex // serializes writes to stdout

	// mu guards the fields below and writes to the daemon connection.
	mu          sync.Mutex
	cur         *link
	queue       []queued
	initLine    []byte
	initialized []byte
	stdinEOF    bool

	imu      sync.Mutex // guards inflight
	inflight map[string]struct{}

	replaySeq atomic.Uint64
	lost      chan struct{} // signals the supervisor to redial
	done      chan struct{}
	doneOnce  sync.Once
	err       error
}

func (c Config) backoff() (lo, hi time.Duration) {
	lo, hi = c.BackoffMin, c.BackoffMax
	if lo <= 0 {
		lo = defaultBackoffMin
	}
	if hi <= 0 {
		hi = defaultBackoffMax
	}
	return lo, max(lo, hi)
}

func (c Config) reconnectTimeout() time.Duration {
	if c.ReconnectTimeout <= 0 {
		return DefaultReconnectTimeout
	}
	return c.ReconnectTimeout
}

func (px *proxy) finish(err error) {
	px.doneOnce.Do(func() {
		px.err = err
		close(px.done)
	})
}

func (px *proxy) dial(ctx context.Context, p wire.Preamble) (*link, error) {
	nw, addr := network(px.cfg.Addr)
	var d net.Dialer
	conn, err := d.DialContext(ctx, nw, addr)
	if err != nil {
		return nil, err
	}
	if err := wire.WritePreamble(conn, p); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &link{conn: conn, br: bufio.NewReader(conn), replayCh: make(chan error, 1)}, nil
}

// writeOut writes one line to stdout.
func (px *proxy) writeOut(line []byte) {
	px.outMu.Lock()
	defer px.outMu.Unlock()
	if _, err := px.stdout.Write(line); err != nil {
		px.log.LogAttrs(px.ctx, slog.LevelError, "write stdout failed", slog.String("error", err.Error()))
	}
}

// failRequest answers a request the daemon will never see (or never answer).
func (px *proxy) failRequest(id string) {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"error":   map[string]any{"code": UnavailableCode, "message": UnavailableMessage},
	})
	px.writeOut(append(b, '\n'))
}

func (px *proxy) track(id string) {
	px.imu.Lock()
	px.inflight[id] = struct{}{}
	px.imu.Unlock()
}

func (px *proxy) retire(id string) {
	px.imu.Lock()
	delete(px.inflight, id)
	px.imu.Unlock()
}

// failInflight fails every request sent to the daemon and not yet answered.
func (px *proxy) failInflight() {
	px.imu.Lock()
	ids := make([]string, 0, len(px.inflight))
	for id := range px.inflight {
		ids = append(ids, id)
	}
	clear(px.inflight)
	px.imu.Unlock()
	for _, id := range ids {
		px.failRequest(id)
	}
}

// readDaemon forwards daemon output to stdout until the connection ends.
func (px *proxy) readDaemon(l *link) {
	for {
		line, err := readLine(l.br)
		if len(line) > 0 {
			px.fromDaemon(l, line)
		}
		if err != nil {
			px.lose(l, err)
			return
		}
	}
}

func (px *proxy) fromDaemon(l *link, line []byte) {
	id, method := parseHead(line)
	if method == "" && id != "" {
		if rid := l.replayID.Load(); rid != nil && id == *rid && l.replayID.CompareAndSwap(rid, nil) {
			select {
			case l.replayCh <- nil:
			default:
			}
			return
		}
		px.retire(id)
	}
	px.writeOut(line)
}

// lose handles the end of link l: it fails in-flight requests and either
// shuts the proxy down (stdin already closed) or asks the supervisor to redial.
func (px *proxy) lose(l *link, cause error) {
	px.mu.Lock()
	l.dead = true
	_ = l.conn.Close()
	select {
	case l.replayCh <- cause:
	default:
	}
	if px.cur != l {
		px.mu.Unlock()
		return
	}
	px.cur = nil
	eof := px.stdinEOF
	px.mu.Unlock()
	if px.ctx.Err() != nil {
		return // shutting down; Run reports it
	}

	px.failInflight()
	if eof {
		px.mu.Lock()
		px.failQueueLocked()
		px.mu.Unlock()
		px.finish(nil)
		return
	}
	if px.ctx.Err() == nil {
		px.log.LogAttrs(px.ctx, slog.LevelWarn, "daemon connection lost", slog.String("cause", describe(cause)))
	}
	select {
	case px.lost <- struct{}{}:
	default:
	}
}

func describe(err error) string {
	if errors.Is(err, io.EOF) {
		return "eof"
	}
	return err.Error()
}

// failQueueLocked fails every queued request. The caller must hold px.mu.
func (px *proxy) failQueueLocked() {
	for _, q := range px.queue {
		q.timer.Stop()
		px.failRequest(q.id)
	}
	px.queue = nil
}

// fromClient routes one line read from stdin.
func (px *proxy) fromClient(line []byte) {
	id, method := parseHead(line)
	px.mu.Lock()
	defer px.mu.Unlock()
	switch {
	case method == methodInitialize && id != "":
		px.initLine = bytes.Clone(line)
	case method == methodInitialized:
		px.initialized = bytes.Clone(line)
	}
	isReq := method != "" && id != ""
	if px.cur == nil {
		if isReq {
			px.enqueueLocked(line, id)
		}
		// Notifications and responses cannot outlive their connection.
		return
	}
	if isReq {
		px.track(id)
	}
	if _, err := px.cur.conn.Write(line); err != nil {
		l := px.cur
		// lose takes px.mu; release it by finishing on a goroutine.
		go px.lose(l, err)
		if isReq {
			px.retire(id)
			px.enqueueLocked(line, id)
		}
	}
}

// enqueueLocked holds a request until the daemon is back or the reconnect
// timeout passes. The caller must hold px.mu.
func (px *proxy) enqueueLocked(line []byte, id string) {
	q := queued{line: bytes.Clone(line), id: id}
	q.timer = time.AfterFunc(px.cfg.reconnectTimeout(), func() { px.expire(id) })
	px.queue = append(px.queue, q)
}

func (px *proxy) expire(id string) {
	px.mu.Lock()
	found := false
	for i, q := range px.queue {
		if q.id == id {
			px.queue = append(px.queue[:i], px.queue[i+1:]...)
			found = true
			break
		}
	}
	px.mu.Unlock()
	if found {
		px.failRequest(id)
	}
}

// readLine reads one newline-terminated line of any length. The returned line
// includes its newline; a final unterminated line gets one appended.
func readLine(br *bufio.Reader) ([]byte, error) {
	line, err := br.ReadBytes('\n')
	if err != nil && len(line) > 0 && line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	return line, err
}

// replay re-establishes the MCP session on a fresh link: it re-sends the
// cached initialize with a new id, waits for and swallows the response, then
// re-sends the initialized notification.
func (px *proxy) replay(l *link, initLine []byte) error {
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(initLine, &msg); err != nil {
		return fmt.Errorf("decode cached initialize: %w", err)
	}
	id := fmt.Sprintf("%q", fmt.Sprintf("aim-replay-%d", px.replaySeq.Add(1)))
	msg["id"] = json.RawMessage(id)
	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode initialize: %w", err)
	}
	l.replayID.Store(&id)
	if _, err := l.conn.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write initialize: %w", err)
	}
	select {
	case err := <-l.replayCh:
		if err != nil {
			return fmt.Errorf("await initialize response: %w", err)
		}
	case <-time.After(replayTimeout):
		return errors.New("timed out waiting for initialize response")
	case <-px.ctx.Done():
		return px.ctx.Err()
	}
	return nil
}

// publish makes l the live link: it replays initialized, flushes the queued
// requests in order and starts forwarding new client lines.
func (px *proxy) publish(l *link) error {
	px.mu.Lock()
	defer px.mu.Unlock()
	if l.dead {
		return errors.New("connection lost during replay")
	}
	if px.initLine != nil && px.initialized != nil {
		if _, err := l.conn.Write(px.initialized); err != nil {
			return fmt.Errorf("write initialized: %w", err)
		}
	}
	for i, q := range px.queue {
		px.track(q.id)
		if _, err := l.conn.Write(q.line); err != nil {
			// Requests already written count as in flight on a dead link;
			// the rest stay queued for the next attempt.
			px.retire(q.id)
			for _, w := range px.queue[:i] {
				w.timer.Stop()
				px.retire(w.id)
				px.failRequest(w.id)
			}
			px.queue = px.queue[i:]
			return fmt.Errorf("flush queue: %w", err)
		}
	}
	for _, q := range px.queue {
		q.timer.Stop()
	}
	px.queue = nil
	px.cur = l
	return nil
}

// supervise redials after each loss until the proxy finishes.
func (px *proxy) supervise() {
	for {
		select {
		case <-px.done:
			return
		case <-px.ctx.Done():
			return
		case <-px.lost:
		}
		px.reconnect()
	}
}

func (px *proxy) reconnect() {
	lo, hi := px.cfg.backoff()
	delay := lo
	for attempt := 1; ; attempt++ {
		select {
		case <-px.done:
			return
		case <-px.ctx.Done():
			return
		default:
		}
		if px.tryReconnect() {
			px.log.LogAttrs(px.ctx, slog.LevelInfo, "reconnected to daemon", slog.Int("attempts", attempt))
			return
		}
		t := time.NewTimer(delay)
		select {
		case <-px.done:
			t.Stop()
			return
		case <-px.ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		delay = min(delay*2, hi)
	}
}

func (px *proxy) tryReconnect() bool {
	p := px.p
	p.Resume = true
	l, err := px.dial(px.ctx, p)
	if err != nil {
		return false
	}
	go px.readDaemon(l)

	px.mu.Lock()
	initLine := px.initLine
	px.mu.Unlock()
	if initLine != nil {
		if err := px.replay(l, initLine); err != nil {
			px.log.LogAttrs(px.ctx, slog.LevelWarn, "session replay failed", slog.String("error", err.Error()))
			_ = l.conn.Close()
			return false
		}
	}
	if err := px.publish(l); err != nil {
		px.log.LogAttrs(px.ctx, slog.LevelWarn, "publish failed", slog.String("error", err.Error()))
		_ = l.conn.Close()
		return false
	}
	// stdin may have closed while we were reconnecting.
	px.mu.Lock()
	eof := px.stdinEOF
	px.mu.Unlock()
	if eof {
		closeWrite(l.conn)
	}
	return true
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}

// readStdin feeds client lines to the proxy until stdin ends.
func (px *proxy) readStdin(stdin io.Reader) {
	br := bufio.NewReader(stdin)
	for {
		line, err := readLine(br)
		if len(line) > 0 {
			px.fromClient(line)
		}
		if err != nil {
			break
		}
	}
	px.mu.Lock()
	px.stdinEOF = true
	l := px.cur
	if l == nil {
		px.failQueueLocked()
	}
	px.mu.Unlock()
	if l != nil {
		closeWrite(l.conn)
		return
	}
	px.finish(nil)
}

// Run dials the daemon, sends p, and proxies newline-delimited JSON-RPC
// between stdin/stdout and the daemon. When the daemon connection drops it
// redials with backoff, re-sends the preamble with Resume set, replays the
// client's initialize handshake (swallowing the replayed response) and
// delivers requests queued meanwhile; requests that cannot be delivered in
// time are failed with a JSON-RPC unavailable error so the client never
// hangs. Run returns when stdin ends and the daemon has finished answering,
// or when ctx ends. An unreachable daemon at start is an error.
func Run(ctx context.Context, cfg Config, p wire.Preamble, stdin io.Reader, stdout io.Writer) error {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	px := &proxy{
		cfg: cfg, p: p, stdout: stdout, log: logger, ctx: ctx,
		inflight: map[string]struct{}{}, lost: make(chan struct{}, 1), done: make(chan struct{}),
	}
	l, err := px.dial(ctx, p)
	if err != nil {
		return fmt.Errorf("aim daemon not reachable at %s: %w", cfg.Addr, err)
	}
	px.cur = l
	go px.readDaemon(l)
	go px.supervise()
	go px.readStdin(stdin)

	select {
	case <-px.done:
		return px.err
	case <-ctx.Done():
		px.mu.Lock()
		if px.cur != nil {
			_ = px.cur.conn.Close()
		}
		px.failQueueLocked()
		px.mu.Unlock()
		return ctx.Err()
	}
}
