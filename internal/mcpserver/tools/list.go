package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// ListInput is the input of list.
type ListInput struct {
	Address string `json:"address" jsonschema:"mailbox address; covers everything below it"`
	State   string `json:"state,omitempty" jsonschema:"unread (default), read, archived, inbox (read or unread, not archived) or all; archived requires since and until"`
	From    string `json:"from,omitempty" jsonschema:"only messages from this sender address"`
	Since   string `json:"since,omitempty" jsonschema:"RFC3339 lower time bound"`
	Until   string `json:"until,omitempty" jsonschema:"RFC3339 upper time bound"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum items, default 50, max 200"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from a previous result"`
}

func (t *toolset) list(ctx context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, mail.ListResult, error) {
	if err := session.RequireAddress("list", in.Address); err != nil {
		return nil, mail.ListResult{Items: []mail.Header{}}, err
	}
	since, err := session.ParseTime("since", in.Since)
	if err != nil {
		return nil, mail.ListResult{}, err
	}
	until, err := session.ParseTime("until", in.Until)
	if err != nil {
		return nil, mail.ListResult{}, err
	}
	res, err := t.c.Svc.List(ctx, mail.ListInput{
		Address: in.Address, State: in.State, From: in.From, Since: since, Until: until,
		Limit: in.Limit, Cursor: in.Cursor,
	})
	if res.Items == nil {
		res.Items = []mail.Header{}
	}
	return nil, res, session.ToolErr(err)
}
