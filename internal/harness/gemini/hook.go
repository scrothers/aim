package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

var _ harness.Hook = Runtime{}

// Client returns Gemini CLI's MCP clientInfo name.
func (Runtime) Client() string { return "gemini-cli" }

type payload struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Event     string `json:"hook_event_name"`
	Source    string `json:"source"`

	NotificationType string `json:"notification_type"`
	Message          string `json:"message"`
}

var eventNames = map[string]string{
	wire.HookSessionStart: "SessionStart",
	wire.HookPrompt:       "BeforeAgent",
	wire.HookPostTool:     "AfterTool",
	wire.HookSessionEnd:   "SessionEnd",
}

// Parse normalizes a Gemini CLI hook payload, falling back to the
// GEMINI_SESSION_ID and GEMINI_CWD/GEMINI_PROJECT_DIR environment. Gemini has
// no watch.
func (Runtime) Parse(event string, stdin []byte, getenv func(string) string) (harness.Parsed, bool, error) {
	if event == "watch" {
		return harness.Parsed{}, false, nil
	}
	var in payload
	if err := json.Unmarshal(stdin, &in); err != nil {
		return harness.Parsed{}, false, fmt.Errorf("gemini: parse payload: %w", err)
	}
	if in.SessionID == "" {
		in.SessionID = getenv("GEMINI_SESSION_ID")
	}
	if in.Cwd == "" {
		in.Cwd = getenv("GEMINI_CWD")
	}
	if in.Cwd == "" {
		in.Cwd = getenv("GEMINI_PROJECT_DIR")
	}
	req := wire.HookRequest{Op: wire.HookOpEvent}
	switch in.Event {
	case "SessionStart":
		req.Event, req.Source = wire.HookSessionStart, in.Source
	case "BeforeAgent":
		req.Event = wire.HookPrompt
	case "AfterTool":
		req.Event = wire.HookPostTool
	case "Notification":
		if in.NotificationType != "ToolPermission" {
			return harness.Parsed{}, false, nil
		}
		req.Event, req.Attention, req.Detail = wire.HookStatus, wire.AttentionBlocked, in.Message
	case "SessionEnd":
		req.Event = wire.HookSessionEnd
	default:
		return harness.Parsed{}, false, nil
	}
	return harness.Parsed{Request: req, SessionID: in.SessionID, Cwd: in.Cwd}, true, nil
}

// Render writes one hookSpecificOutput JSON object, or nothing: Gemini
// requires stdout to hold only that object.
func (Runtime) Render(req wire.HookRequest, reply wire.HookReply) harness.Output {
	if req.Event == wire.HookStatus {
		return harness.Output{}
	}
	return harness.Output{Stdout: harness.AdditionalContext(eventNames[req.Event], reply.Context)}
}
