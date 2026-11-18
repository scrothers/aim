package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// ReadInput is the input of read.
type ReadInput struct {
	MessageID string `json:"message_id" jsonschema:"message id, msg-..."`
	Peek      bool   `json:"peek,omitempty" jsonschema:"read without marking the delivery read"`
	Subagent  string `json:"subagent,omitempty" jsonschema:"read as this subagent of your identity"`
}

// ReadOutput is the result of read: the message plus whether the read left
// the delivery marked read.
type ReadOutput struct {
	mail.Message
	Marked bool `json:"marked" jsonschema:"true when a delivery of this message is marked read for you; false for peek or when you are not signed on as an owner"`
}

func (t *toolset) read(ctx context.Context, _ *mcp.CallToolRequest, in ReadInput) (*mcp.CallToolResult, ReadOutput, error) {
	m, err := t.c.Svc.Read(ctx, t.c.Caller(), mail.ReadInput{MessageID: in.MessageID, Peek: in.Peek, Subagent: in.Subagent})
	return nil, ReadOutput{Message: m, Marked: m.Marked}, session.ToolErr(err)
}
