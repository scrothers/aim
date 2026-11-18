package prompts

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver/session"
)

// escalate attaches the thread being escalated, when one is named.
var escalate = behavior{
	attach: func(ctx context.Context, c *session.Conn, args map[string]string, _ time.Time) ([]*mcp.PromptMessage, error) {
		if args["thread_id"] == "" {
			return nil, nil
		}
		return threadAttachment(ctx, c, args["thread_id"])
	},
}
