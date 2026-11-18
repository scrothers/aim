package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// AddressInput is the input of tools that take only an address.
type AddressInput struct {
	Address string `json:"address" jsonschema:"mailbox address; covers everything below it"`
}

// RosterOutput is the result of roster.
type RosterOutput struct {
	Identities []mail.RosterEntry `json:"identities"`
}

func (t *toolset) roster(ctx context.Context, _ *mcp.CallToolRequest, in AddressInput) (*mcp.CallToolResult, RosterOutput, error) {
	if err := session.RequireAddress("roster", in.Address); err != nil {
		return nil, RosterOutput{Identities: []mail.RosterEntry{}}, err
	}
	es, err := t.c.Svc.Roster(ctx, in.Address)
	if es == nil {
		es = []mail.RosterEntry{}
	}
	return nil, RosterOutput{Identities: es}, session.ToolErr(err)
}
