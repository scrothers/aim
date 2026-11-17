package daemon

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

func testDaemon(timeout time.Duration) *daemon {
	return &daemon{
		cfg:    Config{ShutdownTimeout: timeout},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		conns:  map[net.Conn]struct{}{},
	}
}

func TestShutdownConnsRecordsLeak(t *testing.T) {
	t.Parallel()
	d := testDaemon(50 * time.Millisecond)
	c, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	if !d.track(c) { // never untracked: a handler stuck past the timeout
		t.Fatal("track refused")
	}
	d.shutdownConns(context.Background())
	if !d.leakedConns() {
		t.Error("leaked = false after drain timeout; store would be closed under the handler")
	}

	d = testDaemon(time.Second)
	c2, peer2 := net.Pipe()
	t.Cleanup(func() { _ = peer2.Close() })
	d.track(c2)
	d.untrack(c2)
	d.shutdownConns(context.Background())
	if d.leakedConns() {
		t.Error("leaked = true after clean drain")
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func TestInflightHoldsEOFUntilAnswered(t *testing.T) {
	t.Parallel()
	f := newInflight(5 * time.Second)
	in := f.reader(strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"ping"}`+"\n"+`{"jsonrpc":"2.0","method":"notifications/x"}`+"\n"), nopCloser{})
	var out strings.Builder
	w := f.writer(nopWriteCloser{&out})
	eof := make(chan struct{})
	go func() { _, _ = io.ReadAll(in); close(eof) }()
	select {
	case <-eof:
		t.Fatal("EOF delivered while request 7 was unanswered")
	case <-time.After(100 * time.Millisecond):
	}
	// A split write still retires the request.
	_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":7,`))
	_, _ = w.Write([]byte(`"result":{}}` + "\n"))
	select {
	case <-eof:
	case <-time.After(5 * time.Second):
		t.Fatal("EOF still held after response written")
	}
	if err := w.Close(); err != nil {
		t.Error(err)
	}
}

func TestInflightReleasedByCloseAndGrace(t *testing.T) {
	t.Parallel()
	req := `{"jsonrpc":"2.0","id":"a","method":"ping"}` + "\n"
	for name, tt := range map[string]struct {
		grace time.Duration
		close bool
	}{"grace": {50 * time.Millisecond, false}, "close": {time.Minute, true}} {
		t.Run(name, func(t *testing.T) {
			f := newInflight(tt.grace)
			in := f.reader(strings.NewReader(req), nopCloser{})
			done := make(chan struct{})
			go func() { _, _ = io.ReadAll(in); close(done) }()
			if tt.close {
				time.Sleep(50 * time.Millisecond)
				_ = in.Close()
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("reader never released")
			}
		})
	}
}
