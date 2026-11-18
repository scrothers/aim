package mcpserver

import (
	"context"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolUseMetaKey is the _meta key carrying Claude Code's tool-use id.
const toolUseMetaKey = "claudecode/toolUseId"

// captureClient records the MCP clientInfo on the presence entry the first
// time a tool is called on the connection, and attributes subagent calls.
func (h *handlers) captureClient(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "tools/call" {
			h.conn.Touch(time.Now())
			h.attributeSubagent(req)
		}
		if method == "tools/call" && !h.clientSeen.Load() {
			if ss, ok := req.GetSession().(*mcp.ServerSession); ok {
				if ip := ss.InitializeParams(); ip != nil && ip.ClientInfo != nil {
					h.conn.SetClient(ip.ClientInfo.Name, ip.ClientInfo.Version)
					h.clientSeen.Store(true)
				}
			}
		}
		return next(ctx, method, req)
	}
}

// attributeSubagent sets the subagent argument of a send, read or mark call
// that left it empty, when the resolver knows the call's subagent.
func (h *handlers) attributeSubagent(req mcp.Request) {
	ctr, ok := req.(*mcp.CallToolRequest)
	if !ok || h.resolveSubagent == nil || ctr.Params == nil {
		return
	}
	switch ctr.Params.Name {
	case "send", "read", "mark":
	default:
		return
	}
	id, _ := ctr.Params.GetMeta()[toolUseMetaKey].(string)
	sub, ok := h.resolveSubagent(id)
	if !ok {
		return
	}
	args := map[string]any{}
	if len(ctr.Params.Arguments) > 0 {
		if err := json.Unmarshal(ctr.Params.Arguments, &args); err != nil || args == nil {
			return // leave malformed arguments for the handler to reject
		}
	}
	if s, _ := args["subagent"].(string); s != "" {
		return
	}
	args["subagent"] = sub
	if b, err := json.Marshal(args); err == nil {
		ctr.Params.Arguments = b
	}
}
