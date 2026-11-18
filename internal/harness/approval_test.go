package harness_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

func TestToolSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, tool, input, want string
	}{
		{"bash", "Bash", `{"command":"git  push\n origin","description":"Push"}`, "Bash: git push origin"},
		{"edit", "Edit", `{"file_path":"/src/main.go","old_string":"a","new_string":"b"}`, "Edit: /src/main.go"},
		{"argv command", "shell", `{"command":["bash","-lc","ls -la"]}`, "shell: bash -lc ls -la"},
		{"url", "WebFetch", `{"url":"https://example.com","prompt":"x"}`, "WebFetch: https://example.com"},
		{"no telling field", "mcp__x__y", `{"n":1}`, "mcp__x__y"},
		{"no input", "Task", ``, "Task"},
		{"not an object", "Bash", `"ls"`, "Bash"},
		{"no tool", "", `{"command":"ls"}`, "ls"},
		{"long", "Bash", `{"command":"` + strings.Repeat("é", 200) + `"}`, "Bash: " + strings.Repeat("é", 97) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := harness.ToolSummary(tt.tool, json.RawMessage(tt.input)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPermissionDecision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		reply wire.HookReply
		want  string
	}{
		{"allow", wire.HookReply{Decision: wire.DecisionAllow, Reason: "fine"},
			`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}` + "\n"},
		{"deny", wire.HookReply{Decision: wire.DecisionDeny, Reason: "no"},
			`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"Denied by the operator in the AIM console. Reason: no"}}}` + "\n"},
		{"none", wire.HookReply{SignedOn: true}, ""},
		{"unknown", wire.HookReply{Decision: "maybe"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := string(harness.PermissionDecision("PermissionRequest", tt.reply)); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRunHookApprove(t *testing.T) {
	t.Parallel()
	dial, seen := serve(t, &wire.HookReply{Context: "decided"})
	big := json.RawMessage(`{"content":"` + strings.Repeat("x", wire.MaxToolInputBytes) + `"}`)
	h := fakeHook{ok: true, parsed: harness.Parsed{Request: harness.ApprovalRequest("Write", big)}}
	out := harness.RunHook(context.Background(), h, "", nil, deps(dial, nil))
	if string(out.Stdout) != "ctx:decided" {
		t.Errorf("out = %+v", out)
	}
	if got := <-seen; got[1] != "approve||fake" {
		t.Errorf("request = %q", got[1])
	}
}
