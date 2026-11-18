package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WhoamiOutput is the result of whoami.
type WhoamiOutput struct {
	SessionID  string `json:"session_id" jsonschema:"Claude session id of this connection"`
	Machine    string `json:"machine" jsonschema:"hostname of the machine the shim runs on"`
	ProjectDir string `json:"project_dir" jsonschema:"project directory of this session"`
	GitRemote  string `json:"git_remote,omitempty" jsonschema:"origin remote URL of the project"`
	Namespace  string `json:"namespace,omitempty" jsonschema:"your git namespace host/org/repo, usable as an address"`
	SignedOnAs string `json:"signed_on_as,omitempty" jsonschema:"identity this connection is signed on as"`
	Client     string `json:"client,omitempty" jsonschema:"MCP client name, for example claude-code"`
	ClientVer  string `json:"client_version,omitempty" jsonschema:"MCP client version"`
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
