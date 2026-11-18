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
}

var eventNames = map[string]string{
	wire.HookSessionStart:  "SessionStart",
	wire.HookPrompt:        "UserPromptSubmit",
	wire.HookSubagentStart: "SubagentStart",
	wire.HookSessionEnd:    "SessionEnd",
}

// Parse normalizes a Codex hook payload. Events other than session start,
// prompt, subagent start and session end are ignored: Codex fails a Stop hook
// that prints plain text, so AIM never answers those. Codex has no watch.
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
	case "SessionEnd":
		req.Event = wire.HookSessionEnd
	default:
		return harness.Parsed{}, false, nil
	}
	return harness.Parsed{Request: req, SessionID: in.SessionID, Cwd: in.Cwd}, true, nil
}

// Render writes hookSpecificOutput.additionalContext.
func (Runtime) Render(req wire.HookRequest, reply wire.HookReply) harness.Output {
	return harness.Output{Stdout: harness.AdditionalContext(eventNames[req.Event], reply.Context)}
}
