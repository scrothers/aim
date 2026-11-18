package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// SendInput is the input of send.
type SendInput struct {
	To        []string `json:"to" jsonschema:"recipient addresses"`
	Cc        []string `json:"cc,omitempty" jsonschema:"carbon-copy addresses"`
	Subject   string   `json:"subject" jsonschema:"subject, at most 200 characters"`
	Body      string   `json:"body" jsonschema:"message body, at most 64 KiB"`
	InReplyTo string   `json:"in_reply_to,omitempty" jsonschema:"id of the message being replied to"`
	ReplyAll  bool     `json:"reply_all,omitempty" jsonschema:"also address the original sender, to and cc minus yourself"`
	Subagent  string   `json:"subagent,omitempty" jsonschema:"send as this subagent of your identity"`
}

func (t *toolset) send(ctx context.Context, _ *mcp.CallToolRequest, in SendInput) (*mcp.CallToolResult, mail.Message, error) {
	m, err := t.c.Svc.Send(ctx, t.c.Caller(), mail.SendInput{
		To: in.To, Cc: in.Cc, Subject: in.Subject, Body: in.Body,
		InReplyTo: in.InReplyTo, Subagent: in.Subagent, ReplyAll: in.ReplyAll,
	})
	return nil, m, session.ToolErr(err)
}
