package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

// fakeDaemon returns a Dial that answers one hook exchange with reply.
func fakeDaemon(reply wire.HookReply) func(context.Context) (net.Conn, error) {
	return func(context.Context) (net.Conn, error) {
		c, s := net.Pipe()
		go func() {
			defer func() { _ = s.Close() }()
			br := bufio.NewReader(s)
			if _, err := wire.ReadPreamble(br); err != nil {
				return
			}
			var req wire.HookRequest
			if err := wire.ReadLine(br, &req); err != nil {
				return
			}
			_ = wire.WriteLine(s, reply)
		}()
		return c, nil
	}
}

func testDeps(dial func(context.Context) (net.Conn, error)) harness.Deps {
	return harness.Deps{Getenv: func(string) string { return "" }, Dial: dial}
}

func TestRunHookCmd(t *testing.T) {
	t.Parallel()
	prompt := `{"session_id":"s","cwd":"/p","hook_event_name":"UserPromptSubmit"}`
	stop := `{"session_id":"s","cwd":"/p","hook_event_name":"Stop"}`
	down := func(context.Context) (net.Conn, error) { return nil, errors.New("down") }
	tests := []struct {
		name       string
		args       []string
		stdin      string
		dial       func(context.Context) (net.Conn, error)
		wantStdout string
		wantStderr string
		wantCode   int
	}{
		{
			name: "context on stdout", args: []string{"claude"}, stdin: prompt,
			dial:       fakeDaemon(wire.HookReply{Context: "hi"}),
			wantStdout: `{"hookSpecificOutput":{"additionalContext":"hi","hookEventName":"UserPromptSubmit"}}` + "\n",
		},
		{
			name: "watch wake exits 2", args: []string{"claude", "watch"}, stdin: stop,
			dial:       fakeDaemon(wire.HookReply{Wake: true, Context: "mail"}),
			wantStderr: "mail\n", wantCode: 2,
		},
		{name: "daemon down fails open", args: []string{"claude"}, stdin: prompt, dial: down},
		{name: "unknown runtime fails open", args: []string{"nope"}, stdin: prompt, dial: down},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			err := runHookCmd(context.Background(), tt.args, strings.NewReader(tt.stdin), &stdout, &stderr, testDeps(tt.dial))
			code := 0
			var ee exitError
			if errors.As(err, &ee) {
				code = ee.code
			} else if err != nil {
				t.Fatalf("err = %v", err)
			}
			if code != tt.wantCode || stdout.String() != tt.wantStdout || stderr.String() != tt.wantStderr {
				t.Errorf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestHookCmdRegistered(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SetIn(strings.NewReader(`{}`))
	root.SetArgs([]string{"hook"})
	if err := root.Execute(); err == nil {
		t.Error("hook without a runtime should be an argument error")
	}
	if msg := (exitError{code: 2}).Error(); msg != "exit status 2" {
		t.Errorf("exit error text = %q", msg)
	}
	if d := hookDeps(); d.Getenv == nil || d.Dial == nil {
		t.Error("hookDeps unset")
	}
}
