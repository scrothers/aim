package daemon

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// halfCloseGrace bounds how long a half-closed connection waits for its
// in-flight requests to be answered.
const halfCloseGrace = 30 * time.Second

// inflight tracks JSON-RPC requests read from a connection that have not yet
// been answered. A client such as the shim may write its requests and then
// half-close; the MCP transport treats read EOF as a hang-up and abandons
// pending calls, so the reader holds EOF back until the responses are out.
type inflight struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending map[string]struct{}
	closed  bool
	grace   time.Duration
}

func newInflight(grace time.Duration) *inflight {
	f := &inflight{pending: map[string]struct{}{}, grace: grace}
	f.cond = sync.NewCond(&f.mu)
	return f
}

type rpcHeader struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

// lineScanner splits a byte stream into newline-terminated lines.
type lineScanner struct{ buf []byte }

func (s *lineScanner) feed(p []byte, fn func(line []byte)) {
	s.buf = append(s.buf, p...)
	for {
		i := bytes.IndexByte(s.buf, '\n')
		if i < 0 {
			return
		}
		fn(s.buf[:i])
		s.buf = s.buf[i+1:]
	}
}

func parseHeader(line []byte) (id string, isReq, ok bool) {
	var h rpcHeader
	if json.Unmarshal(line, &h) != nil || len(h.ID) == 0 || string(h.ID) == "null" {
		return "", false, false
	}
	return string(h.ID), h.Method != "", true
}

// reader wraps the inbound side; c is closed by Close.
func (f *inflight) reader(r io.Reader, c io.Closer) io.ReadCloser {
	return &inflightReader{f: f, r: r, c: c}
}

type inflightReader struct {
	f    *inflight
	r    io.Reader
	c    io.Closer
	scan lineScanner
}

func (ir *inflightReader) Read(p []byte) (int, error) {
	n, err := ir.r.Read(p)
	if n > 0 {
		ir.scan.feed(p[:n], func(line []byte) {
			if id, isReq, ok := parseHeader(line); ok && isReq {
				ir.f.mu.Lock()
				ir.f.pending[id] = struct{}{}
				ir.f.mu.Unlock()
			}
		})
	}
	if err == io.EOF {
		ir.f.drain()
	}
	return n, err
}

func (ir *inflightReader) Close() error {
	ir.f.mu.Lock()
	ir.f.closed = true
	ir.f.mu.Unlock()
	ir.f.cond.Broadcast()
	return ir.c.Close()
}

// drain blocks until no request is pending, the grace period ends or the
// reader is closed.
func (f *inflight) drain() {
	timer := time.AfterFunc(f.grace, func() {
		f.mu.Lock()
		f.closed = true
		f.mu.Unlock()
		f.cond.Broadcast()
	})
	defer timer.Stop()
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.pending) > 0 && !f.closed {
		f.cond.Wait()
	}
}

// writer wraps the outbound side, retiring requests as their responses pass.
func (f *inflight) writer(w io.WriteCloser) io.WriteCloser { return &inflightWriter{f: f, w: w} }

type inflightWriter struct {
	f    *inflight
	w    io.WriteCloser
	scan lineScanner
}

func (iw *inflightWriter) Close() error { return iw.w.Close() }

func (iw *inflightWriter) Write(p []byte) (int, error) {
	n, err := iw.w.Write(p)
	iw.scan.feed(p[:n], func(line []byte) {
		if id, isReq, ok := parseHeader(line); ok && !isReq {
			iw.f.mu.Lock()
			delete(iw.f.pending, id)
			iw.f.mu.Unlock()
			iw.f.cond.Broadcast()
		}
	})
	return n, err
}
