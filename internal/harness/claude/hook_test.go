package claude_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/claude"
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
			stdin:  `{"session_id":"s1","cwd":"/p","hook_event_name":"SessionStart","source":"resume","extra":1}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionStart, Source: "resume"}},
		},
		{
			name:   "prompt",
			stdin:  `{"session_id":"s1","cwd":"/p","hook_event_name":"UserPromptSubmit","prompt":"hi"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPrompt}},
		},
		{
			name:   "subagent start",
			stdin:  `{"session_id":"s1","hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"reviewer"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSubagentStart, AgentID: "a1", AgentType: "reviewer"}},
		},
		{
			name:   "pre tool in subagent",
			stdin:  `{"session_id":"s1","hook_event_name":"PreToolUse","agent_id":"a1","agent_type":"x","tool_use_id":"tu","tool_name":"Bash"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPreTool, AgentID: "a1", AgentType: "x", ToolUseID: "tu", ToolName: "Bash"}},
		},
		{name: "pre tool in main agent", stdin: `{"hook_event_name":"PreToolUse","tool_name":"Bash"}`},
		{
			name:   "post tool in main agent",
			stdin:  `{"session_id":"s1","hook_event_name":"PostToolUse","tool_use_id":"tu","tool_name":"Bash"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPostTool, ToolUseID: "tu", ToolName: "Bash"}},
		},
		{
			name:   "post tool in subagent",
			stdin:  `{"session_id":"s1","hook_event_name":"PostToolUse","agent_id":"a1","agent_type":"x","tool_use_id":"tu","tool_name":"Bash"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPostTool, AgentID: "a1", AgentType: "x", ToolUseID: "tu", ToolName: "Bash"}},
		},
		{
			name:   "permission prompt",
			stdin:  `{"session_id":"s1","hook_event_name":"Notification","notification_type":"permission_prompt","message":"needs permission"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionBlocked, Detail: "needs permission"}},
		},
		{
			name:   "agent needs input",
			stdin:  `{"session_id":"s1","hook_event_name":"Notification","notification_type":"agent_needs_input","message":"q"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionBlocked, Detail: "q"}},
		},
		{
			name:   "elicitation dialog",
			stdin:  `{"session_id":"s1","hook_event_name":"Notification","notification_type":"elicitation_dialog"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionBlocked}},
		},
		{
			name:   "idle prompt",
			stdin:  `{"session_id":"s1","hook_event_name":"Notification","notification_type":"idle_prompt","message":"idle"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionWaiting, Detail: "idle"}},
		},
		{name: "other notification", stdin: `{"hook_event_name":"Notification","notification_type":"auth_success"}`},
		{
			name:   "stop failure details",
			stdin:  `{"session_id":"s1","hook_event_name":"StopFailure","error":"rate_limit","error_details":"429","last_assistant_message":"API Error"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionFailed, Detail: "429"}},
		},
		{
			name:   "stop failure error only",
			stdin:  `{"session_id":"s1","hook_event_name":"StopFailure","error":"server_error"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookStatus, Attention: wire.AttentionFailed, Detail: "server_error"}},
		},
		{
			name:   "session end",
			stdin:  `{"session_id":"s1","hook_event_name":"SessionEnd"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionEnd}},
		},
		{
			name:   "watch from stop payload",
			event:  "watch",
			stdin:  `{"session_id":"s1","cwd":"/p","hook_event_name":"Stop"}`,
			wantOK: true,
			want:   harness.Parsed{SessionID: "s1", Cwd: "/p", Request: wire.HookRequest{Op: wire.HookOpWatch}},
		},
		{
			name: "permission request",
			stdin: `{"session_id":"s1","cwd":"/p","hook_event_name":"PermissionRequest","permission_mode":"default",` +
				`"tool_name":"Bash","tool_input":{"command":"git push origin main","description":"Push"},"permission_suggestions":[]}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpApprove, ToolName: "Bash", Detail: "Bash: git push origin main",
				ToolInput: json.RawMessage(`{"command":"git push origin main","description":"Push"}`)}},
		},
		{
			name:   "permission request without input",
			stdin:  `{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"mcp__x__y"}`,
			wantOK: true,
			want: harness.Parsed{SessionID: "s1", Request: wire.HookRequest{
				Op: wire.HookOpApprove, ToolName: "mcp__x__y", Detail: "mcp__x__y"}},
		},
		{name: "unhandled event", stdin: `{"hook_event_name":"Stop"}`},
		{name: "grok owns its events", stdin: `{"hook_event_name":"SessionStart"}`, env: map[string]string{"GROK_HOOK_EVENT": "session_start"}},
		{name: "bad json", stdin: `{`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			getenv := func(k string) string { return tt.env[k] }
			got, ok, err := claude.New().Parse(tt.event, []byte(tt.stdin), getenv)
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
		name     string
		req      wire.HookRequest
		reply    wire.HookReply
		wantExit int
		wantErr  string
	}{
		{name: "session start", req: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookSessionStart},
			reply: wire.HookReply{Context: "You are **alice**."}},
		{name: "prompt", req: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPrompt},
			reply: wire.HookReply{Context: "1 new message"}},
		{name: "subagent start", req: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookSubagentStart},
			reply: wire.HookReply{Context: "sub"}},
		{name: "pre tool", req: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPreTool},
			reply: wire.HookReply{Context: "tool"}},
		{name: "post tool", req: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPostTool},
			reply: wire.HookReply{Context: "mail"}},
		{name: "status", req: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookStatus},
			reply: wire.HookReply{Context: "ignored"}},
		{name: "empty context", req: wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPrompt}},
		{name: "watch wake", req: wire.HookRequest{Op: wire.HookOpWatch},
			reply: wire.HookReply{Wake: true, Context: "new mail"}, wantExit: 2, wantErr: "new mail\n"},
		{name: "watch superseded", req: wire.HookRequest{Op: wire.HookOpWatch}, reply: wire.HookReply{Superseded: true}},
		{name: "approve allow", req: wire.HookRequest{Op: wire.HookOpApprove},
			reply: wire.HookReply{Decision: wire.DecisionAllow, Reason: "ok"}},
		{name: "approve deny", req: wire.HookRequest{Op: wire.HookOpApprove},
			reply: wire.HookReply{Decision: wire.DecisionDeny, Reason: "use a branch"}},
		{name: "approve deny without reason", req: wire.HookRequest{Op: wire.HookOpApprove},
			reply: wire.HookReply{Decision: wire.DecisionDeny}},
		{name: "approve no decision", req: wire.HookRequest{Op: wire.HookOpApprove}, reply: wire.HookReply{SignedOn: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := claude.New().Render(tt.req, tt.reply)
			if out.Exit != tt.wantExit {
				t.Errorf("exit = %d, want %d", out.Exit, tt.wantExit)
			}
			if string(out.Stderr) != tt.wantErr {
				t.Errorf("stderr = %q, want %q", out.Stderr, tt.wantErr)
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
	h := claude.New()
	if h.Name() != "claude" || h.Client() != "claude-code" {
		t.Errorf("name/client = %s/%s", h.Name(), h.Client())
	}
}
