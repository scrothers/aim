package codex

import (
	"encoding/json"
	"fmt"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

var _ harness.Hook = Runtime{}

// Client returns Codex's MCP clientInfo name.
func (Runtime) Client() string { return "codex" }

type payload struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Event     string `json:"hook_event_name"`
	Source    string `json:"source"`
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
	ToolName  string `json:"tool_name"`
	// ToolInput is the PermissionRequest tool's arguments; Bash and
	// apply_patch carry tool_input.command.
	ToolInput json.RawMessage `json:"tool_input"`
}

var eventNames = map[string]string{
	wire.HookSessionStart:  "SessionStart",
	wire.HookPrompt:        "UserPromptSubmit",
	wire.HookSubagentStart: "SubagentStart",
	wire.HookPostTool:      "PostToolUse",
	wire.HookSessionEnd:    "SessionEnd",
}

// Parse normalizes a Codex hook payload. Events other than session start,
// prompt, subagent start, post-tool, permission request, interrupt and
// session end are ignored: Codex fails a Stop hook that prints plain text,
// so AIM never answers those. Codex has no watch.
func (Runtime) Parse(event string, stdin []byte, _ func(string) string) (harness.Parsed, bool, error) {
	if event == "watch" {
		return harness.Parsed{}, false, nil
	}
	var in payload
	if err := json.Unmarshal(stdin, &in); err != nil {
		return harness.Parsed{}, false, fmt.Errorf("codex: parse payload: %w", err)
	}
	req := wire.HookRequest{Op: wire.HookOpEvent}
	switch in.Event {
	case "SessionStart":
		req.Event, req.Source = wire.HookSessionStart, in.Source
	case "UserPromptSubmit":
		req.Event = wire.HookPrompt
	case "SubagentStart":
		req.Event, req.AgentID, req.AgentType = wire.HookSubagentStart, in.AgentID, in.AgentType
	case "PostToolUse":
		req.Event, req.AgentID, req.AgentType = wire.HookPostTool, in.AgentID, in.AgentType
	case "PermissionRequest":
		req = harness.ApprovalRequest(in.ToolName, in.ToolInput)
	case "Interrupt":
		req.Event, req.Attention = wire.HookStatus, wire.AttentionInterrupted
	case "SessionEnd":
		req.Event = wire.HookSessionEnd
	default:
		return harness.Parsed{}, false, nil
	}
	return harness.Parsed{Request: req, SessionID: in.SessionID, Cwd: in.Cwd}, true, nil
}

// Render writes hookSpecificOutput.additionalContext, or for a permission
// request its decision (nothing without one: Codex then uses its normal
// approval flow). Codex fails closed on updatedInput, updatedPermissions and
// interrupt for PermissionRequest, so the decision carries only behavior and
// a denial message. See https://developers.openai.com/codex/hooks.
func (Runtime) Render(req wire.HookRequest, reply wire.HookReply) harness.Output {
	if req.Op == wire.HookOpApprove {
		return harness.Output{Stdout: harness.PermissionDecision("PermissionRequest", reply)}
	}
	if req.Event == wire.HookStatus {
		// Codex fails Interrupt hooks that print plain text.
		return harness.Output{}
	}
	return harness.Output{Stdout: harness.AdditionalContext(eventNames[req.Event], reply.Context)}
}
