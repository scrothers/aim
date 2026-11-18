package codex_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/codex"
	"github.com/scrothers/aim/internal/wire"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		event   string
		stdin   string
		wantOK  bool
		wantErr bool
		want    harness.Parsed
	}{
		{
			name:   "session start",
			stdin:  `{"session_id":"s1","cwd":"/p","hook_event_name":"SessionStart","source":"startup","model":"x"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionStart, Source: "startup"}},
		},
		{
			name:   "prompt",
			stdin:  `{"session_id":"s1","hook_event_name":"UserPromptSubmit"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPrompt}},
		},
		{
			name:   "subagent start",
			stdin:  `{"session_id":"s1","hook_event_name":"SubagentStart","agent_id":"a","agent_type":"t"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSubagentStart, AgentID: "a", AgentType: "t"}},
		},
		{
			name:   "session end",
			stdin:  `{"session_id":"s1","hook_event_name":"SessionEnd"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionEnd}},
		},
		{name: "stop is ignored", stdin: `{"hook_event_name":"Stop"}`},
		{name: "pre tool is ignored", stdin: `{"hook_event_name":"PreToolUse","agent_id":"a"}`},
		{
			name:   "post tool",
			stdin:  `{"session_id":"s1","hook_event_name":"PostToolUse"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPostTool}},
		},
		{
			name:   "post tool in subagent",
			stdin:  `{"session_id":"s1","hook_event_name":"PostToolUse","agent_id":"a","agent_type":"t"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPostTool, AgentID: "a", AgentType: "t"}},
		},
		{
			name:   "interrupt",
			stdin:  `{"session_id":"s1","hook_event_name":"Interrupt"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionInterrupted}},
		},
		{name: "no watch", event: "watch", stdin: `{"hook_event_name":"Stop"}`},
		{name: "bad json", stdin: `nope`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok, err := codex.New().Parse(tt.event, []byte(tt.stdin), func(string) string { return "" })
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parsed = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		event string
		reply wire.HookReply
	}{
		{name: "session start", event: wire.HookSessionStart, reply: wire.HookReply{Context: "hello"}},
		{name: "prompt", event: wire.HookPrompt, reply: wire.HookReply{Context: "mail"}},
		{name: "subagent start", event: wire.HookSubagentStart, reply: wire.HookReply{Context: "sub"}},
		{name: "post tool", event: wire.HookPostTool, reply: wire.HookReply{Context: "mail"}},
		{name: "status", event: wire.HookStatus, reply: wire.HookReply{Context: "ignored"}},
		{name: "empty", event: wire.HookPrompt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := codex.New().Render(wire.HookRequest{Op: wire.HookOpEvent, Event: tt.event}, tt.reply)
			if out.Exit != 0 || len(out.Stderr) != 0 {
				t.Errorf("exit/stderr = %d/%q", out.Exit, out.Stderr)
			}
			golden(t, tt.name, out.Stdout)
		})
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "render_"+strings.ReplaceAll(name, " ", "_")+".golden")
	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_SNAPSHOTS=1): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestIdentity(t *testing.T) {
	t.Parallel()
	if h := codex.New(); h.Name() != "codex" || h.Client() != "codex" {
		t.Errorf("name/client = %s/%s", h.Name(), h.Client())
	}
}
