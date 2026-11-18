package grok

import (
	"encoding/json"
	"fmt"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

var _ harness.Hook = Runtime{}

// Client returns Grok Build's MCP clientInfo name.
func (Runtime) Client() string { return "grok" }

// payload accepts both the camelCase and snake_case spellings Grok sends.
type payload struct {
	HookEventName  string `json:"hookEventName"`
	HookEventSnake string `json:"hook_event_name"`
	SessionID      string `json:"sessionId"`
	SessionSnake   string `json:"session_id"`
	Cwd            string `json:"cwd"`
	WorkspaceRoot  string `json:"workspaceRoot"`
	WorkspaceSnake string `json:"workspace_root"`
	SubagentType   string `json:"subagentType"`
	SubagentSnake  string `json:"subagent_type"`
	Source         string `json:"source"`
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

var eventNames = map[string]string{
	wire.HookSessionStart: "SessionStart",
	wire.HookPostTool:     "PostToolUse",
	wire.HookSessionEnd:   "SessionEnd",
}

// canonical maps either spelling of a Grok event name to its PascalCase form.
var canonical = map[string]string{
	"session_start": "SessionStart", "SessionStart": "SessionStart",
	"post_tool_use": "PostToolUse", "PostToolUse": "PostToolUse",
	"session_end": "SessionEnd", "SessionEnd": "SessionEnd",
}

// Parse normalizes a Grok Build hook payload, falling back to GROK_SESSION_ID
// and GROK_HOOK_EVENT. Grok has no watch.
func (Runtime) Parse(event string, stdin []byte, getenv func(string) string) (harness.Parsed, bool, error) {
	if event == "watch" {
		return harness.Parsed{}, false, nil
	}
	var in payload
	if err := json.Unmarshal(stdin, &in); err != nil {
		return harness.Parsed{}, false, fmt.Errorf("grok: parse payload: %w", err)
	}
	name := canonical[first(in.HookEventSnake, in.HookEventName, getenv("GROK_HOOK_EVENT"))]
	req := wire.HookRequest{Op: wire.HookOpEvent}
	switch name {
	case "SessionStart":
		req.Event, req.Source = wire.HookSessionStart, in.Source
	case "PostToolUse":
		req.Event, req.AgentType = wire.HookPostTool, first(in.SubagentType, in.SubagentSnake)
	case "SessionEnd":
		req.Event = wire.HookSessionEnd
	default:
		return harness.Parsed{}, false, nil
	}
	return harness.Parsed{
		Request:   req,
		SessionID: first(in.SessionID, in.SessionSnake, getenv("GROK_SESSION_ID")),
		Cwd:       first(in.Cwd, in.WorkspaceRoot, in.WorkspaceSnake),
	}, true, nil
}

// Render writes hookSpecificOutput.additionalContext.
func (Runtime) Render(req wire.HookRequest, reply wire.HookReply) harness.Output {
	return harness.Output{Stdout: harness.AdditionalContext(eventNames[req.Event], reply.Context)}
}
