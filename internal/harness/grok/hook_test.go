package grok_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/grok"
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
			name:   "camel session start",
			stdin:  `{"hookEventName":"session_start","hook_event_name":"SessionStart","sessionId":"k1","cwd":"/p","workspaceRoot":"/w"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "k1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionStart}},
		},
		{
			name:   "snake post tool with subagent",
			stdin:  `{"hook_event_name":"PostToolUse","session_id":"k1","workspace_root":"/w","subagent_type":"explorer","toolName":"bash"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "k1", Cwd: "/w", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPostTool, AgentType: "explorer"}},
		},
		{
			name:   "camel post tool",
			stdin:  `{"hookEventName":"post_tool_use","sessionId":"k1","workspaceRoot":"/w","subagentType":"x"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "k1", Cwd: "/w", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPostTool, AgentType: "x"}},
		},
		{
			name:   "env event and session",
			stdin:  `{}`,
			env:    map[string]string{"GROK_HOOK_EVENT": "session_end", "GROK_SESSION_ID": "e1"},
			wantOK: true,
			want:   harness.Parsed{SessionID: "e1", Request: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookSessionEnd}},
		},
		{
			name:   "camel permission notification",
			stdin:  `{"hookEventName":"Notification","sessionId":"g","notificationType":"permission_prompt","message":"m"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionBlocked, Detail: "m"}},
		},
		{
			name:   "snake idle notification",
			stdin:  `{"hook_event_name":"notification","session_id":"g","notification_type":"idle_prompt"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionWaiting}},
		},
		{name: "other notification", stdin: `{"hookEventName":"Notification","notificationType":"info"}`},
		{
			name:   "stop failure",
			stdin:  `{"hookEventName":"StopFailure","sessionId":"g","errorMessage":"rate limited"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionFailed, Detail: "rate limited"}},
		},
		{
			name:   "stop cancelled",
			stdin:  `{"hook_event_name":"stop_cancelled","session_id":"g"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "g", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionInterrupted}},
		},
		{name: "other event", stdin: `{"hookEventName":"pre_tool_use"}`},
		{name: "no watch", event: "watch", stdin: `{}`},
		{name: "bad json", stdin: `[`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok, err := grok.New().Parse(tt.event, []byte(tt.stdin), func(k string) string { return tt.env[k] })
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
		{name: "post tool", event: wire.HookPostTool, reply: wire.HookReply{Context: "mail"}},
		{name: "status", event: wire.HookStatus, reply: wire.HookReply{Context: "ignored"}},
		{name: "empty", event: wire.HookPostTool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := grok.New().Render(wire.HookRequest{Op: wire.HookOpEvent, Event: tt.event}, tt.reply)
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
	if h := grok.New(); h.Name() != "grok" || h.Client() != "grok" {
		t.Errorf("name/client = %s/%s", h.Name(), h.Client())
	}
}
