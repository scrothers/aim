package prompts

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// reply attaches the thread of the message being replied to, without marking
// it read.
var reply = behavior{
	attach: func(ctx context.Context, c *session.Conn, args map[string]string, _ time.Time) ([]*mcp.PromptMessage, error) {
		m, err := c.Svc.Read(ctx, c.Caller(), mail.ReadInput{MessageID: args["message_id"], Peek: true})
		if err != nil {
			return nil, err
		}
		return threadAttachment(ctx, c, m.ThreadID)
	},
}
