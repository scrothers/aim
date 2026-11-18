package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WhoamiOutput is the result of whoami.
type WhoamiOutput struct {
	SessionID  string `json:"session_id" jsonschema:"Agent session id behind this connection; sign-ons are restored per session id after a reconnect."`
	Machine    string `json:"machine" jsonschema:"Hostname of the machine the session runs on."`
	ProjectDir string `json:"project_dir" jsonschema:"Project directory of this session."`
	GitRemote  string `json:"git_remote,omitempty" jsonschema:"origin remote URL of the project; absent without a remote."`
	Namespace  string `json:"namespace,omitempty" jsonschema:"Your git namespace as an address, for example github.com/acme/api. Pass it to agent_roster, mail_list, mail_search and mail_wait; agent_register identities under it."`
	SignedOnAs string `json:"signed_on_as,omitempty" jsonschema:"Identity this connection acts as; absent until agent_sign_on. mail_send and mail_mark need it."`
	Client     string `json:"client,omitempty" jsonschema:"MCP client name, for example claude-code."`
	ClientVer  string `json:"client_version,omitempty" jsonschema:"MCP client version."`
}

func (t *toolset) whoami(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, WhoamiOutput, error) {
	p := t.c.Entry.Preamble()
	out := WhoamiOutput{
		SessionID: p.SessionID, Machine: p.Machine, ProjectDir: p.ProjectDir,
		GitRemote: p.GitRemote, Namespace: p.Namespace,
	}
	if id, ok := t.c.Entry.Identity(); ok {
		out.SignedOnAs = id.String()
	}
	st := t.c.Entry.Status()
	out.Client, out.ClientVer = st.Client, st.ClientVersion
	return nil, out, nil
}
