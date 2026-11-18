package prompts

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mdview"
)

// standupLimit caps how much mail a standup digest embeds.
const standupLimit = mail.MaxListLimit

// standup defaults the address to the session's namespace and attaches
// today's mail under it.
var standup = behavior{
	defaults: func(c *session.Conn, args map[string]string) error {
		if args["address"] != "" {
			return nil
		}
		args["address"] = c.Entry.Preamble().Namespace
		if args["address"] == "" {
			return fmt.Errorf("mcpserver: prompt standup: %w: no address given and this session has no git namespace", mail.ErrInvalidInput)
		}
		return nil
	},
	attach: func(ctx context.Context, c *session.Conn, args map[string]string, now time.Time) ([]*mcp.PromptMessage, error) {
		since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		res, err := c.Svc.List(ctx, mail.ListInput{Address: args["address"], State: mail.StateInbox, Since: &since, Limit: standupLimit})
		if err != nil {
			return nil, err
		}
		text := mdview.Digest("Mail since "+since.Format(time.DateOnly)+" under "+args["address"], res.Items, res.NextCursor != "")
		return []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}}, nil
	},
}
