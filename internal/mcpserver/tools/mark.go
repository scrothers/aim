package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// MarkInput is the input of mark.
type MarkInput struct {
	MessageIDs []string `json:"message_ids" jsonschema:"Ids of the messages to change, 1 to 200, for example [msg-01jcz6x2k3, msg-01jcz6y8pp]. Ids not delivered to you are skipped."`
	State      string   `json:"state" jsonschema:"read, unread, archived (handled: hidden from the unread and inbox lists, never deleted) or unarchived."`
	Subagent   string   `json:"subagent,omitempty" jsonschema:"Act on deliveries to this subagent of your identity, for example tests for tests@reviewer@github.com/acme/api; its mailbox must exist. Filled in automatically for Claude Code subagents when AIM hooks are installed."`
}

// MarkOutput is the result of mark.
type MarkOutput struct {
	Changed int `json:"changed" jsonschema:"Number of deliveries that changed state. Ids you do not own and deliveries already in that state are skipped, so it can be lower than the ids passed."`
}

func (t *toolset) mark(ctx context.Context, _ *mcp.CallToolRequest, in MarkInput) (*mcp.CallToolResult, MarkOutput, error) {
	n, err := t.c.Svc.Mark(ctx, t.c.Caller(), mail.MarkInput{MessageIDs: in.MessageIDs, State: in.State, Subagent: in.Subagent})
	return nil, MarkOutput{Changed: n}, session.ToolErr(err)
}
