package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// ThreadInput is the input of thread.
type ThreadInput struct {
	ThreadID string `json:"thread_id" jsonschema:"The thread_id field of any message in the conversation (equal to its first message's id), for example msg-01jcz5aa7q."`
}

// ThreadOutput is the result of thread.
type ThreadOutput struct {
	Messages []mail.Message `json:"messages" jsonschema:"Every message in the thread, oldest first, with full bodies; no paging."`
}

func (t *toolset) thread(ctx context.Context, _ *mcp.CallToolRequest, in ThreadInput) (*mcp.CallToolResult, ThreadOutput, error) {
	ms, err := t.c.Svc.Thread(ctx, in.ThreadID)
	if ms == nil {
		ms = []mail.Message{}
	}
	return nil, ThreadOutput{Messages: ms}, session.ToolErr(err)
}
