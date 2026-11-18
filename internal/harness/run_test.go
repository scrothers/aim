package harness_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

type fakeHook struct {
	parsed   harness.Parsed
	ok       bool
	parseErr error
}

func (fakeHook) Name() string   { return "fake" }
func (fakeHook) Client() string { return "fake-client" }
func (f fakeHook) Parse(string, []byte, func(string) string) (harness.Parsed, bool, error) {
	return f.parsed, f.ok, f.parseErr
}

func (fakeHook) Render(_ wire.HookRequest, r wire.HookReply) harness.Output {
	return harness.Output{Stdout: []byte("ctx:" + r.Context), Exit: map[bool]int{true: 2}[r.Wake]}
}

// serve answers one hook exchange on the server end of a pipe, reporting the
// preamble and request it saw. A nil reply closes without answering.
func serve(t *testing.T, reply *wire.HookReply) (dial func(context.Context) (net.Conn, error), seen chan [2]string) {
	t.Helper()
	seen = make(chan [2]string, 1)
	dial = func(context.Context) (net.Conn, error) {
		c, s := net.Pipe()
		go func() {
			defer func() { _ = s.Close() }()
			br := bufio.NewReader(s)
			pre, err := wire.ReadPreamble(br)
			if err != nil {
				return
			}
			var req wire.HookRequest
			if err := wire.ReadLine(br, &req); err != nil {
				return
			}
			seen <- [2]string{pre.Kind + "|" + pre.Client + "|" + pre.SessionID + "|" + pre.ProjectDir + "|" + pre.Machine + "|" + pre.Namespace,
				req.Op + "|" + req.Event + "|" + req.Runtime}
			if reply != nil {
				_ = wire.WriteLine(s, reply)
			}
		}()
		return c, nil
	}
	return dial, seen
}

func deps(dial func(context.Context) (net.Conn, error), env map[string]string) harness.Deps {
	return harness.Deps{
		Getenv:   func(k string) string { return env[k] },
		Dial:     dial,
		Hostname: func() (string, error) { return "box", nil },
		Getwd:    func() (string, error) { return "/wd", nil },
		AgentPID: func() int { return 42 },
		Git:      func(context.Context, string) (string, error) { return "git@github.com:acme/widgets.git", nil },
	}
}

func TestRunHookExchange(t *testing.T) {
	t.Parallel()
	dial, seen := serve(t, &wire.HookReply{Context: "hello"})
	h := fakeHook{ok: true, parsed: harness.Parsed{
		SessionID: "s1", Cwd: "/proj", Request: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPrompt}}}
	out := harness.RunHook(context.Background(), h, "", nil, deps(dial, nil))
	if string(out.Stdout) != "ctx:hello" || out.Exit != 0 {
		t.Errorf("out = %+v", out)
	}
	got := <-seen
	if !strings.HasPrefix(got[0], "hook|fake-client|s1|/proj|box|") || !strings.Contains(got[0], "widgets") {
		t.Errorf("preamble = %q", got[0])
	}
	if got[1] != "event|prompt|fake" {
		t.Errorf("request = %q", got[1])
	}
}

func TestRunHookFallbacksAndWatch(t *testing.T) {
	t.Parallel()
	dial, seen := serve(t, &wire.HookReply{Wake: true, Context: "mail"})
	h := fakeHook{ok: true, parsed: harness.Parsed{Request: wire.HookRequest{Op: wire.HookOpWatch}}}
	out := harness.RunHook(context.Background(), h, "watch", nil, deps(dial, nil))
	if string(out.Stdout) != "ctx:mail" || out.Exit != 2 {
		t.Errorf("out = %+v", out)
	}
	got := <-seen
	if !strings.Contains(got[0], "|/wd|box|") {
		t.Errorf("preamble should fall back to cwd: %q", got[0])
	}
	if got[1] != "watch||fake" {
		t.Errorf("request = %q", got[1])
	}
}

func TestRunHookFailOpen(t *testing.T) {
	t.Parallel()
	okHook := fakeHook{ok: true, parsed: harness.Parsed{Request: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPrompt}}}
	closeDial, _ := serve(t, nil)
	tests := []struct {
		name      string
		hook      fakeHook
		dial      func(context.Context) (net.Conn, error)
		debug     bool
		wantDebug string
	}{
		{name: "ignored event", hook: fakeHook{}},
		{name: "parse error", hook: fakeHook{parseErr: errors.New("boom")}, debug: true, wantDebug: "aim hook fake: parse: boom\n"},
		{name: "parse error silent", hook: fakeHook{parseErr: errors.New("boom")}},
		{
			name: "daemon down", hook: okHook, debug: true, wantDebug: "dial: nope",
			dial: func(context.Context) (net.Conn, error) { return nil, errors.New("nope") },
		},
		{name: "daemon hangs up", hook: okHook, dial: closeDial, debug: true, wantDebug: "reply:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{}
			if tt.debug {
				env["AIM_HOOK_DEBUG"] = "1"
			}
			out := harness.RunHook(context.Background(), tt.hook, "", nil, deps(tt.dial, env))
			if out.Exit != 0 || len(out.Stdout) != 0 {
				t.Errorf("out = %+v, want empty exit 0", out)
			}
			if tt.wantDebug == "" && len(out.Stderr) != 0 {
				t.Errorf("stderr = %q, want none", out.Stderr)
			}
			if !strings.Contains(string(out.Stderr), tt.wantDebug) {
				t.Errorf("stderr = %q, want %q", out.Stderr, tt.wantDebug)
			}
		})
	}
}

func TestAdditionalContext(t *testing.T) {
	t.Parallel()
	if got := harness.AdditionalContext("X", ""); got != nil {
		t.Errorf("empty context = %q", got)
	}
	want := `{"hookSpecificOutput":{"additionalContext":"a \"b\"","hookEventName":"X"}}` + "\n"
	if got := string(harness.AdditionalContext("X", `a "b"`)); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEnvDeps(t *testing.T) {
	t.Parallel()
	d := harness.EnvDeps(nil, nil, nil)
	if d.Getenv == nil || d.Hostname == nil || d.Getwd == nil {
		t.Error("EnvDeps left real deps unset")
	}
}
