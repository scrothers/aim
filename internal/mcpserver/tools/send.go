package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// SendInput is the input of send.
type SendInput struct {
	To        []string `json:"to" jsonschema:"Recipient addresses, for example [reviewer@github.com/acme/api] or [operator]. Identities must be registered; org, repo and subagent mailboxes are created on first send. Required unless reply_all is set."`
	Cc        []string `json:"cc,omitempty" jsonschema:"Carbon-copy addresses, same rules as to. An address in both to and cc is delivered once, as to."`
	Subject   string   `json:"subject" jsonschema:"Subject line, 1 to 200 characters. In replies keep the original, for example Re: Release plan."`
	Body      string   `json:"body" jsonschema:"Markdown body, up to 64 KiB. Put the ask first, then concrete references such as PR numbers, file paths and error text."`
	InReplyTo string   `json:"in_reply_to,omitempty" jsonschema:"Id of the message you are answering (msg-...); the reply joins its thread. Omit to start a new thread."`
	ReplyAll  bool     `json:"reply_all,omitempty" jsonschema:"Requires in_reply_to. Also address the original sender and its to and cc (cc stays cc), minus yourself and retired identities. Default false."`
	Subagent  string   `json:"subagent,omitempty" jsonschema:"Send as this subagent of your identity, for example tests sends from tests@reviewer@github.com/acme/api. Filled in automatically for Claude Code subagents when AIM hooks are installed."`
	TaskID    string   `json:"task_id,omitempty" jsonschema:"Link the sent message to this task (task-...), so it appears in the task's history and the thread shows the task. The task must exist."`
}

func (t *toolset) send(ctx context.Context, _ *mcp.CallToolRequest, in SendInput) (*mcp.CallToolResult, mail.Message, error) {
	if in.TaskID != "" {
		board, err := t.c.TaskBoard()
		if err != nil {
			return nil, mail.Message{}, err
		}
		if err := board.Exists(ctx, in.TaskID); err != nil {
			return nil, mail.Message{}, fmt.Errorf("%w; nothing was sent", err)
		}
	}
	c := t.c.Caller()
	m, err := t.c.Svc.Send(ctx, c, mail.SendInput{
		To: in.To, Cc: in.Cc, Subject: in.Subject, Body: in.Body,
		InReplyTo: in.InReplyTo, Subagent: in.Subagent, ReplyAll: in.ReplyAll,
	})
	if err != nil || in.TaskID == "" {
		return nil, m, session.ToolErr(err)
	}
	return nil, m, t.linkSent(ctx, c, in.TaskID, m)
}
