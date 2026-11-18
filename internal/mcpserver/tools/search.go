package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// SearchInput is the input of search.
type SearchInput struct {
	Address string `json:"address" jsonschema:"Scope to search, covering everything below it, for example github.com/acme or reviewer@github.com/acme/api. A misspelled address returns no hits, not an error."`
	Query   string `json:"query" jsonschema:"SQLite FTS5 query over subject and body: words (all must match), quoted phrases, OR, NOT, prefix*. Quote terms with punctuation, for example session.go. Must not be empty."`
	Limit   int    `json:"limit,omitempty" jsonschema:"Maximum hits per page; default 20, maximum 100 (larger values are clamped)."`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page, with the same address and query. Best-effort offset: hits can shift if mail arrives between pages."`
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
