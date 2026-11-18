package mcpserver

import (
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mcpserver/tools"
	"github.com/scrothers/aim/internal/presence"
)

// New returns an MCP server for one connection. Handlers act on behalf of
// conn, whose preamble and sign-on state identify the caller.
func New(svc *mail.Service, conn *presence.Entry, version string, opts ...Option) *mcp.Server {
	h := &handlers{svc: svc, conn: conn, c: &session.Conn{Svc: svc, Entry: conn}}
	h.subs.debounce = defaultSubscribeDebounce
	for _, o := range opts {
		o(h)
	}
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "aim", Version: version},
		&mcp.ServerOptions{
			Instructions:       instructions(),
			SubscribeHandler:   h.subscribe,
			UnsubscribeHandler: h.unsubscribe,
			CompletionHandler:  h.complete,
		},
	)
	h.srv = srv
	h.registerResources(srv)
	h.registerPrompts(srv)
	srv.AddReceivingMiddleware(h.captureClient)
	tools.Register(srv, h.c, tools.Options{ProgressInterval: h.progressInterval})
	return srv
}

// Option configures a per-connection server.
type Option func(*handlers)

// WithProgressInterval sets how often wait reports progress while blocked.
// Non-positive values keep the default.
func WithProgressInterval(d time.Duration) Option {
	return func(h *handlers) { h.progressInterval = d }
}

// SubagentResolver returns the subagent that issued the tool call with the
// given runtime tool-use id, as recorded by a pre-tool hook.
type SubagentResolver func(toolUseID string) (string, bool)

// WithSubagentResolver fills in the `subagent` argument of send, read and
// mark calls that omit it, when the resolver knows which subagent made the
// call. Claude Code sends the tool-use id as _meta["claudecode/toolUseId"].
func WithSubagentResolver(r SubagentResolver) Option {
	return func(h *handlers) { h.resolveSubagent = r }
}

type handlers struct {
	svc              *mail.Service
	conn             *presence.Entry
	c                *session.Conn
	srv              *mcp.Server
	progressInterval time.Duration
	clientSeen       atomic.Bool
	subs             subscriptions
	resolveSubagent  SubagentResolver
}

func (h *handlers) caller() mail.Caller { return h.c.Caller() }
