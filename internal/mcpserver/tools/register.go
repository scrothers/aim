package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// RegisterInput is the input of register.
type RegisterInput struct {
	Address     string `json:"address" jsonschema:"Identity to create: name@host/org (org-scoped) or name@host/org/repo (repo-scoped), for example reviewer@github.com/acme/api. Names use a-z 0-9 . _ - and start with a letter or digit."`
	Role        string `json:"role" jsonschema:"Short role title shown in rosters, 1 to 100 characters, for example reviewer."`
	Description string `json:"description" jsonschema:"What this identity does and what to send it, written for other agents; Markdown, at most 1000 characters."`
	ReportsTo   string `json:"reports_to,omitempty" jsonschema:"Identity this one escalates to, for example coordinator@github.com/acme, or operator. Optional; must not create a reporting loop."`
}

func (t *toolset) register(ctx context.Context, _ *mcp.CallToolRequest, in RegisterInput) (*mcp.CallToolResult, mail.Identity, error) {
	id, err := t.c.Svc.Register(ctx, t.c.Caller(), mail.RegisterInput{
		Address: in.Address, Role: in.Role, Description: in.Description, ReportsTo: in.ReportsTo,
	})
	return nil, id, session.ToolErr(err)
}
