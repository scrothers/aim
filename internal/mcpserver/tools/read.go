package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// ReadInput is the input of read.
type ReadInput struct {
	MessageID string `json:"message_id" jsonschema:"Id of the message to read, for example msg-01jcz6x2k3, from mail_list, mail_wait, mail_search or mail_thread."`
	Peek      bool   `json:"peek,omitempty" jsonschema:"true reads without marking anything read; default false. Use it to inspect mail you are not handling now."`
	Subagent  string `json:"subagent,omitempty" jsonschema:"Mark the copy delivered to this subagent of your identity, for example tests. An unknown subagent is not an error; nothing is marked. Filled in automatically for Claude Code subagents when AIM hooks are installed."`
}

// ReadOutput is the result of read: the message plus whether the read left
// the delivery marked read.
type ReadOutput struct {
	mail.Message
	Marked bool `json:"marked" jsonschema:"true when your copy is now, or already was, marked read; false for peek, when you are not signed on, or when the message was not delivered to you."`
}

func (t *toolset) read(ctx context.Context, _ *mcp.CallToolRequest, in ReadInput) (*mcp.CallToolResult, ReadOutput, error) {
	m, err := t.c.Svc.Read(ctx, t.c.Caller(), mail.ReadInput{MessageID: in.MessageID, Peek: in.Peek, Subagent: in.Subagent})
	return nil, ReadOutput{Message: m, Marked: m.Marked}, session.ToolErr(err)
}
