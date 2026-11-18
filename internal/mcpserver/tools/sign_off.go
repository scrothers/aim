package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SignOffOutput is the result of sign_off.
type SignOffOutput struct {
	WasSignedOn bool `json:"was_signed_on"`
}

func (t *toolset) signOff(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, SignOffOutput, error) {
	_, was := t.c.Entry.Identity()
	t.c.Entry.SignOff()
	if err := t.c.Svc.ClearSignon(ctx, t.c.Entry.Status().SessionID); err != nil {
		return nil, SignOffOutput{}, err
	}
	return nil, SignOffOutput{WasSignedOn: was}, nil
}
