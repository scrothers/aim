package wire_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/wire"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	in := wire.Preamble{SessionID: "s1", ProjectDir: "/p", Machine: "m", GitRemote: "r", Namespace: "n"}
	var buf bytes.Buffer
	if err := wire.WritePreamble(&buf, in); err != nil {
		t.Fatal(err)
	}
	buf.WriteString(`{"jsonrpc":"2.0"}` + "\n")
	br := bufio.NewReader(&buf)
	got, err := wire.ReadPreamble(br)
	if err != nil {
		t.Fatal(err)
	}
	in.V = wire.Version
	if got != in {
		t.Fatalf("got %+v want %+v", got, in)
	}
	rest, _ := io.ReadAll(wire.BufferedConn(br, io.NopCloser(nil)))
	if string(rest) != `{"jsonrpc":"2.0"}`+"\n" {
		t.Fatalf("rest = %q", rest)
	}
}

func TestReadPreambleErrors(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"empty":      "",
		"no newline": `{"v":1}`,
		"bad json":   "nope\n",
		"bad ver":    `{"v":2}` + "\n",
		"too large":  strings.Repeat("a", wire.MaxPreambleBytes+10) + "\n",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := wire.ReadPreamble(bufio.NewReaderSize(strings.NewReader(in), 4096))
			if !errors.Is(err, wire.ErrBadPreamble) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func TestWritePreambleError(t *testing.T) {
	t.Parallel()
	if err := wire.WritePreamble(failWriter{}, wire.Preamble{}); err == nil {
		t.Fatal("want error")
	}
}

func TestNamespaceFromRemote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"git@github.com:Org/Repo.git", "github.com/org/repo", true},
		{"git@github.com:org/repo", "github.com/org/repo", true},
		{"ssh://git@github.com:2222/org/repo.git", "github.com/org/repo", true},
		{"https://GitHub.com/org/repo.git", "github.com/org/repo", true},
		{"https://github.com/org/repo/", "github.com/org/repo", true},
		{"", "", false},
		{"/local/path", "", false},
		{"nonsense", "", false},
		{"https://github.com/", "", false},
		{"https://github.com/solo", "", false},
		{"://bad", "", false},
	}
	for _, tt := range tests {
		got, ok := wire.NamespaceFromRemote(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%q: got (%q,%v) want (%q,%v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestHookLinesRoundTrip(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	in := wire.Preamble{SessionID: "s", Kind: wire.KindHook, Client: "codex", AgentPID: 42}
	if err := wire.WritePreamble(&buf, in); err != nil {
		t.Fatal(err)
	}
	req := wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPrompt, ToolUseID: "t1"}
	if err := wire.WriteLine(&buf, req); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(&buf)
	p, err := wire.ReadPreamble(br)
	if err != nil {
		t.Fatal(err)
	}
	in.V = wire.Version
	if p != in {
		t.Fatalf("preamble = %+v, want %+v", p, in)
	}
	var got wire.HookRequest
	if err := wire.ReadLine(br, &got); err != nil {
		t.Fatal(err)
	}
	if got != req {
		t.Fatalf("request = %+v, want %+v", got, req)
	}
}

func TestReadLineWithoutNewline(t *testing.T) {
	t.Parallel()
	var r wire.HookReply
	if err := wire.ReadLine(bufio.NewReader(strings.NewReader(`{"signed_on":true}`)), &r); err != nil || !r.SignedOn {
		t.Fatalf("ReadLine = %+v, %v", r, err)
	}
}

func TestReadLineErrors(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"empty":     "",
		"bad json":  "nope\n",
		"too large": strings.Repeat("a", wire.MaxPreambleBytes+10) + "\n",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var r wire.HookReply
			if err := wire.ReadLine(bufio.NewReaderSize(strings.NewReader(in), 4096), &r); err == nil {
				t.Fatal("want error")
			}
		})
	}
	if err := wire.WriteLine(failWriter{}, 1); err == nil {
		t.Fatal("WriteLine to failing writer: want error")
	}
}

type closeRecorder struct{ closed bool }

func (c *closeRecorder) Close() error { c.closed = true; return nil }

func TestBufferedConnClose(t *testing.T) {
	t.Parallel()
	c := &closeRecorder{}
	rc := wire.BufferedConn(bufio.NewReader(strings.NewReader("x")), c)
	if err := rc.Close(); err != nil || !c.closed {
		t.Fatalf("close: %v closed=%v", err, c.closed)
	}
}
