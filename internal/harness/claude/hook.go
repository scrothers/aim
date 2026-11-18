package claude

import (
	"encoding/json"
	"fmt"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

var _ harness.Hook = Runtime{}

// Client returns Claude Code's MCP clientInfo name.
func (Runtime) Client() string { return "claude-code" }

// payload is the subset of Claude Code's hook stdin AIM reads.
type payload struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Event     string `json:"hook_event_name"`
	Source    string `json:"source"`
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
}

// eventNames maps normalized events to Claude's PascalCase names.
var eventNames = map[string]string{
	wire.HookSessionStart:  "SessionStart",
	wire.HookPrompt:        "UserPromptSubmit",
	wire.HookSubagentStart: "SubagentStart",
	wire.HookPreTool:       "PreToolUse",
	wire.HookSessionEnd:    "SessionEnd",
}

// Parse normalizes a Claude Code hook payload. The event argument "watch"
// selects the asyncRewake mail watch (stdin is the Stop payload).
func (Runtime) Parse(event string, stdin []byte, getenv func(string) string) (harness.Parsed, bool, error) {
	if getenv("GROK_HOOK_EVENT") != "" {
		// Grok Build also reads Claude's settings; it has its own adapter.
		return harness.Parsed{}, false, nil
	}
	var in payload
	if err := json.Unmarshal(stdin, &in); err != nil {
		return harness.Parsed{}, false, fmt.Errorf("claude: parse payload: %w", err)
	}
	p := harness.Parsed{SessionID: in.SessionID, Cwd: in.Cwd}
	if event == "watch" {
		p.Request = wire.HookRequest{Op: wire.HookOpWatch}
		return p, true, nil
	}
	req := wire.HookRequest{Op: wire.HookOpEvent}
	switch in.Event {
	case "SessionStart":
		req.Event, req.Source = wire.HookSessionStart, in.Source
	case "UserPromptSubmit":
		req.Event = wire.HookPrompt
	case "SubagentStart":
		req.Event, req.AgentID, req.AgentType = wire.HookSubagentStart, in.AgentID, in.AgentType
	case "PreToolUse":
		if in.AgentID == "" {
			return harness.Parsed{}, false, nil
		}
		req.Event, req.AgentID, req.AgentType = wire.HookPreTool, in.AgentID, in.AgentType
		req.ToolUseID, req.ToolName = in.ToolUseID, in.ToolName
	case "SessionEnd":
		req.Event = wire.HookSessionEnd
	default:
		return harness.Parsed{}, false, nil
	}
	p.Request = req
	return p, true, nil
}

// Render writes additionalContext for events; for a watch, a wake prints the
// context on stderr and exits 2, which wakes the idle session.
func (Runtime) Render(req wire.HookRequest, reply wire.HookReply) harness.Output {
	if req.Op == wire.HookOpWatch {
		if reply.Wake && reply.Context != "" {
			return harness.Output{Stderr: []byte(reply.Context + "\n"), Exit: 2}
		}
		return harness.Output{}
	}
	return harness.Output{Stdout: harness.AdditionalContext(eventNames[req.Event], reply.Context)}
}
