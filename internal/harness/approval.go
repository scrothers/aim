package harness

import (
	"encoding/json"
	"strings"

	"github.com/scrothers/aim/internal/wire"
)

// summaryLimit caps ToolSummary's output.
const summaryLimit = 200

// ApprovalHookTimeout is the runtime timeout, in seconds, installed for
// permission-request hooks: the longest remote approval timeout the console
// accepts (3600 s) plus a margin, so the daemon, not the runtime, ends the
// wait and the prompt falls back to the terminal cleanly.
const ApprovalHookTimeout = 3630

// summaryKeys are the tool-input fields that best describe a call, in
// order of preference: shell commands, file paths, URLs, search patterns.
var summaryKeys = []string{"command", "cmd", "file_path", "filePath", "path", "notebook_path", "url", "pattern", "query", "description"}

// ToolSummary renders a one-line summary of a tool call for the operator,
// such as "Bash: git push" or "Edit: /src/main.go": the tool name and the
// most telling string field of its input, if any.
func ToolSummary(tool string, input json.RawMessage) string {
	var fields map[string]any
	if json.Unmarshal(input, &fields) != nil {
		return tool
	}
	for _, k := range summaryKeys {
		var s string
		switch v := fields[k].(type) {
		case string:
			s = v
		case []any: // argv-style commands, as Codex sends
			parts := make([]string, 0, len(v))
			for _, p := range v {
				if ps, ok := p.(string); ok {
					parts = append(parts, ps)
				}
			}
			s = strings.Join(parts, " ")
		}
		s = strings.Join(strings.Fields(s), " ")
		if s == "" {
			continue
		}
		if tool == "" {
			return clip(s)
		}
		return clip(tool + ": " + s)
	}
	return tool
}

func clip(s string) string {
	if len(s) <= summaryLimit {
		return s
	}
	cut := summaryLimit
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}

// DenyMessage is the reason an agent sees for a denial; the operator's own
// reason, if any, is appended.
func DenyMessage(reason string) string {
	msg := "Denied by the operator in the AIM console."
	if reason != "" {
		msg += " Reason: " + reason
	}
	return msg
}

// permissionDecision is the decision object of a PermissionRequest reply.
type permissionDecision struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}

type permissionOutput struct {
	Event    string             `json:"hookEventName"`
	Decision permissionDecision `json:"decision"`
}

// PermissionDecision renders the hookSpecificOutput a PermissionRequest
// hook prints, shared by Claude Code and Codex:
//
//	{"hookSpecificOutput":{"hookEventName":event,"decision":{"behavior":"allow"}}}
//
// A denial carries a message for the agent. No decision renders nothing,
// which leaves the runtime to prompt as usual.
func PermissionDecision(event string, reply wire.HookReply) []byte {
	var d permissionDecision
	switch reply.Decision {
	case wire.DecisionAllow:
		d.Behavior = "allow"
	case wire.DecisionDeny:
		d.Behavior, d.Message = "deny", DenyMessage(reply.Reason)
	default:
		return nil
	}
	b, err := json.Marshal(map[string]permissionOutput{"hookSpecificOutput": {Event: event, Decision: d}})
	if err != nil {
		return nil
	}
	return append(b, '\n')
}

// ApprovalRequest builds the approve request for a permission prompt.
func ApprovalRequest(tool string, input json.RawMessage) wire.HookRequest {
	return wire.HookRequest{
		Op:        wire.HookOpApprove,
		ToolName:  tool,
		ToolInput: input,
		Detail:    ToolSummary(tool, input),
	}
}
