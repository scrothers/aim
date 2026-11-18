package opencode

import (
	"encoding/json"
	"fmt"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

var _ harness.Hook = Runtime{}

// Client returns OpenCode's MCP clientInfo name.
func (Runtime) Client() string { return "opencode" }

// payload is the JSON AIM's bundled OpenCode plugin pipes to the hook.
type payload struct {
	SessionID string `json:"sessionID"`
	Directory string `json:"directory"`
	Agent     string `json:"agent"`
}

// Parse normalizes a plugin invocation. The event argument is required: one
// of session-start, prompt, session-end or watch.
func (Runtime) Parse(event string, stdin []byte, _ func(string) string) (harness.Parsed, bool, error) {
	var req wire.HookRequest
	switch event {
	case wire.HookSessionStart, wire.HookPrompt, wire.HookSessionEnd:
		req = wire.HookRequest{Op: wire.HookOpEvent, Event: event}
	case "watch":
		req = wire.HookRequest{Op: wire.HookOpWatch}
	default:
		return harness.Parsed{}, false, nil
	}
	var in payload
	if err := json.Unmarshal(stdin, &in); err != nil {
		return harness.Parsed{}, false, fmt.Errorf("opencode: parse payload: %w", err)
	}
	return harness.Parsed{Request: req, SessionID: in.SessionID, Cwd: in.Directory}, true, nil
}

// Render writes the context as plain Markdown for the plugin to inject. A
// watch prints only when it was woken by mail.
func (Runtime) Render(req wire.HookRequest, reply wire.HookReply) harness.Output {
	if reply.Context == "" || (req.Op == wire.HookOpWatch && !reply.Wake) {
		return harness.Output{}
	}
	return harness.Output{Stdout: []byte(reply.Context + "\n")}
}
