package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// SearchInput is the input of search.
type SearchInput struct {
	Address string `json:"address" jsonschema:"mailbox address; covers everything below it"`
	Query   string `json:"query" jsonschema:"FTS5 query over subject and body"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum hits, default 20, max 100"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from a previous result"`
}

func (t *toolset) search(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, mail.SearchResult, error) {
	if err := session.RequireAddress("search", in.Address); err != nil {
		return nil, mail.SearchResult{Hits: []mail.Hit{}}, err
	}
	res, err := t.c.Svc.Search(ctx, mail.SearchInput{Address: in.Address, Query: in.Query, Limit: in.Limit, Cursor: in.Cursor})
	if res.Hits == nil {
		res.Hits = []mail.Hit{}
	}
	return nil, res, session.ToolErr(err)
}
