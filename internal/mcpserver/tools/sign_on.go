package tools

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// SignOnInput is the input of sign_on.
type SignOnInput struct {
	Address string `json:"address" jsonschema:"registered identity address to sign on as"`
	Away    string `json:"away,omitempty" jsonschema:"optional away message shown in the roster"`
}

// SignOnOutput is the result of sign_on.
type SignOnOutput struct {
	SignedOnAs string `json:"signed_on_as"`
}

func (t *toolset) signOn(ctx context.Context, _ *mcp.CallToolRequest, in SignOnInput) (*mcp.CallToolResult, SignOnOutput, error) {
	a, err := t.c.Svc.ResolveIdentity(ctx, in.Address)
	if err != nil {
		return nil, SignOnOutput{}, session.ToolErr(err)
	}
	if err := t.c.Entry.SignOn(a, in.Away, time.Now()); err != nil {
		return nil, SignOnOutput{}, err
	}
	st := t.c.Entry.Status()
	if err := t.c.Svc.SaveSignon(ctx, mail.Signon{
		SessionID: st.SessionID, Identity: a, Away: in.Away, Client: st.Client, Machine: st.Machine,
	}); err != nil {
		return nil, SignOnOutput{}, err
	}
	return nil, SignOnOutput{SignedOnAs: a.String()}, nil
}
