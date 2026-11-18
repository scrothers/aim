package prompts

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver/resources"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// checkInbox attaches the caller's unread digest.
var checkInbox = behavior{
	attach: func(ctx context.Context, c *session.Conn, _ map[string]string, _ time.Time) ([]*mcp.PromptMessage, error) {
		text, err := resources.Inbox(ctx, c)
		if err != nil {
			return nil, err
		}
		return []*mcp.PromptMessage{embedded(resources.URIInbox, text)}, nil
	},
}
