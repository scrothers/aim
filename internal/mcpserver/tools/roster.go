package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// AddressInput is the input of tools that take only an address.
type AddressInput struct {
	Address string `json:"address" jsonschema:"Org (github.com/acme, including its repos), repo (github.com/acme/api) or identity (reviewer@github.com/acme/api) to list. An unknown identity fails with suggestions; an empty org or repo returns no identities."`
}

// RosterOutput is the result of roster.
type RosterOutput struct {
	Identities []mail.RosterEntry `json:"identities" jsonschema:"Active registered identities in scope, with role, description, reports_to and online sessions."`
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
