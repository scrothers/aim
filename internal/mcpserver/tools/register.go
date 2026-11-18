package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// RegisterInput is the input of register.
type RegisterInput struct {
	Address     string `json:"address" jsonschema:"identity address name@host/org[/repo]"`
	Role        string `json:"role" jsonschema:"short role title, for example reviewer"`
	Description string `json:"description" jsonschema:"what this identity does"`
	ReportsTo   string `json:"reports_to,omitempty" jsonschema:"address of the identity this one reports to"`
}

func (t *toolset) register(ctx context.Context, _ *mcp.CallToolRequest, in RegisterInput) (*mcp.CallToolResult, mail.Identity, error) {
	id, err := t.c.Svc.Register(ctx, t.c.Caller(), mail.RegisterInput{
		Address: in.Address, Role: in.Role, Description: in.Description, ReportsTo: in.ReportsTo,
	})
	return nil, id, session.ToolErr(err)
}
