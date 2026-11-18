package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// ListInput is the input of list.
type ListInput struct {
	Address string `json:"address" jsonschema:"Mailbox to list, covering everything below it: your identity (reviewer@github.com/acme/api) for your inbox, a repo or org (github.com/acme/api), or operator. A misspelled address returns no items, not an error."`
	State   string `json:"state,omitempty" jsonschema:"unread (default: not read, not archived), inbox (read or unread, not archived), read, archived or all. archived requires both since and until."`
	From    string `json:"from,omitempty" jsonschema:"Only messages sent by this address, for example coordinator@github.com/acme. An address that never sent mail returns no items."`
	Since   string `json:"since,omitempty" jsonschema:"RFC3339 lower bound on send time, for example 2026-11-17T00:00:00Z. Required with until when state is archived."`
	Until   string `json:"until,omitempty" jsonschema:"RFC3339 upper bound on send time, for example 2026-11-18T00:00:00Z; must not be before since. Required with since when state is archived."`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum items per page; default 50, maximum 200 (larger values are clamped)."`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page; keep the other arguments unchanged. Omit for the first page."`
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
