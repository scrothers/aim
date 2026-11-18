package gemini_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/gemini"
	"github.com/scrothers/aim/internal/wire"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		event   string
		stdin   string
		env     map[string]string
		wantOK  bool
		wantErr bool
		want    harness.Parsed
	}{
		{
			name:   "session start",
			stdin:  `{"session_id":"g1","cwd":"/p","hook_event_name":"SessionStart","source":"startup"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionStart, Source: "startup"}},
		},
		{
			name:   "before agent is prompt",
			stdin:  `{"session_id":"g1","cwd":"/p","hook_event_name":"BeforeAgent","prompt":"hi"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPrompt}},
		},
		{
			name:   "session end",
			stdin:  `{"session_id":"g1","hook_event_name":"SessionEnd"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionEnd}},
		},
		{
			name:   "env fallback gemini cwd",
			stdin:  `{"hook_event_name":"BeforeAgent"}`,
			env:    map[string]string{"GEMINI_SESSION_ID": "e1", "GEMINI_CWD": "/c", "GEMINI_PROJECT_DIR": "/d"},
			wantOK: true,
			want: harness.Parsed{SessionID: "e1", Cwd: "/c", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPrompt}},
		},
		{
			name:   "env fallback project dir",
			stdin:  `{"hook_event_name":"BeforeAgent"}`,
			env:    map[string]string{"GEMINI_PROJECT_DIR": "/d"},
			wantOK: true,
			want: harness.Parsed{Cwd: "/d", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPrompt}},
		},
		{
			name:   "after tool is post tool",
			stdin:  `{"session_id":"g1","hook_event_name":"AfterTool","tool_name":"x"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPostTool}},
		},
		{
			name:   "tool permission notification",
			stdin:  `{"session_id":"g1","hook_event_name":"Notification","notification_type":"ToolPermission","message":"run ls?"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionBlocked, Detail: "run ls?"}},
		},
		{name: "other notification", stdin: `{"hook_event_name":"Notification","notification_type":"Other"}`},
		{name: "other event", stdin: `{"hook_event_name":"BeforeTool"}`},
		{name: "no watch", event: "watch", stdin: `{}`},
		{name: "bad json", stdin: `x`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok, err := gemini.New().Parse(tt.event, []byte(tt.stdin), func(k string) string { return tt.env[k] })
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
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
		{name: "before agent", event: wire.HookPrompt, reply: wire.HookReply{Context: "mail"}},
		{name: "after tool", event: wire.HookPostTool, reply: wire.HookReply{Context: "mail"}},
		{name: "status", event: wire.HookStatus, reply: wire.HookReply{Context: "ignored"}},
		{name: "empty", event: wire.HookPrompt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := gemini.New().Render(wire.HookRequest{Op: wire.HookOpEvent, Event: tt.event}, tt.reply)
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
	if h := gemini.New(); h.Name() != "gemini" || h.Client() != "gemini-cli" {
		t.Errorf("name/client = %s/%s", h.Name(), h.Client())
	}
}
