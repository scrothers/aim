package opencode

import (
	"encoding/json"
	"fmt"
	"strings"

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
	Attention string `json:"attention"`
	Detail    string `json:"detail"`
	// Approve payloads: the permission (tool) asked for, the prompt's title
	// and its patterns and metadata as one JSON object.
	Tool  string          `json:"tool"`
	Title string          `json:"title"`
	Input json.RawMessage `json:"input"`
}

// approveEvent is the plugin's event argument for a permission prompt.
const approveEvent = "approve"

// Parse normalizes a plugin invocation. The event argument is required: one
// of session-start, prompt, session-end, status, watch or approve.
func (Runtime) Parse(event string, stdin []byte, _ func(string) string) (harness.Parsed, bool, error) {
	var req wire.HookRequest
	switch event {
	case wire.HookSessionStart, wire.HookPrompt, wire.HookSessionEnd, wire.HookStatus:
		req = wire.HookRequest{Op: wire.HookOpEvent, Event: event}
	case "watch":
		req = wire.HookRequest{Op: wire.HookOpWatch}
	case approveEvent:
		req = wire.HookRequest{Op: wire.HookOpApprove}
	default:
		return harness.Parsed{}, false, nil
	}
	var in payload
	if err := json.Unmarshal(stdin, &in); err != nil {
		return harness.Parsed{}, false, fmt.Errorf("opencode: parse payload: %w", err)
	}
	if event == approveEvent {
		req = harness.ApprovalRequest(in.Tool, in.Input)
		if t := strings.Join(strings.Fields(in.Title), " "); t != "" {
			req.Detail = in.Tool + ": " + t
		}
	}
	if event == wire.HookStatus {
		switch in.Attention {
		case "", wire.AttentionBlocked, wire.AttentionWaiting, wire.AttentionFailed, wire.AttentionInterrupted:
			req.Attention, req.Detail = in.Attention, in.Detail
		default:
			return harness.Parsed{}, false, nil
		}
	}
	return harness.Parsed{Request: req, SessionID: in.SessionID, Cwd: in.Directory}, true, nil
}

// approveOutput is the JSON line an approve hook prints for the plugin.
type approveOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// Render writes the context as plain Markdown for the plugin to inject. A
// watch prints only when it was woken by mail. An approval prints
// {"decision":"allow"|"deny","reason":...} for the plugin to apply, or
// nothing without a decision, which leaves OpenCode to prompt as usual.
func (Runtime) Render(req wire.HookRequest, reply wire.HookReply) harness.Output {
	if req.Op == wire.HookOpApprove {
		out := approveOutput{Decision: reply.Decision}
		switch reply.Decision {
		case wire.DecisionAllow:
		case wire.DecisionDeny:
			out.Reason = harness.DenyMessage(reply.Reason)
		default:
			return harness.Output{}
		}
		b, err := json.Marshal(out)
		if err != nil {
			return harness.Output{}
		}
		return harness.Output{Stdout: append(b, '\n')}
	}
	if req.Event == wire.HookStatus || reply.Context == "" || (req.Op == wire.HookOpWatch && !reply.Wake) {
		return harness.Output{}
	}
	return harness.Output{Stdout: []byte(reply.Context + "\n")}
}
