package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// MarkInput is the input of mark.
type MarkInput struct {
	MessageIDs []string `json:"message_ids" jsonschema:"ids of the messages to change"`
	State      string   `json:"state" jsonschema:"read, unread, archived or unarchived"`
	Subagent   string   `json:"subagent,omitempty" jsonschema:"act as this subagent of your identity"`
}

// MarkOutput is the result of mark.
type MarkOutput struct {
	Changed int `json:"changed" jsonschema:"number of deliveries that changed"`
}

func (t *toolset) mark(ctx context.Context, _ *mcp.CallToolRequest, in MarkInput) (*mcp.CallToolResult, MarkOutput, error) {
	n, err := t.c.Svc.Mark(ctx, t.c.Caller(), mail.MarkInput{MessageIDs: in.MessageIDs, State: in.State, Subagent: in.Subagent})
	return nil, MarkOutput{Changed: n}, session.ToolErr(err)
}
