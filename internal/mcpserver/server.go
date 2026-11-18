package mcpserver

import (
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/completions"
	"github.com/scrothers/aim/internal/mcpserver/prompts"
	"github.com/scrothers/aim/internal/mcpserver/resources"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mcpserver/tools"
	"github.com/scrothers/aim/internal/presence"
)

// New returns an MCP server for one connection. Handlers act on behalf of
// conn, whose preamble and sign-on state identify the caller.
func New(svc *mail.Service, conn *presence.Entry, version string, opts ...Option) *mcp.Server {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	c := &session.Conn{Svc: svc, Entry: conn}
	subs := resources.NewSubscriptions(c, cfg.subscribeDebounce, cfg.watcherExit)
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "aim", Version: version},
		&mcp.ServerOptions{
			Instructions:       instructions(),
			SubscribeHandler:   subs.Subscribe,
			UnsubscribeHandler: subs.Unsubscribe,
			CompletionHandler:  completions.Handler(c),
		},
	)
	subs.Bind(srv)
	resources.Register(srv, c)
	prompts.Register(srv, c)
	mw := &middleware{conn: conn, resolveSubagent: cfg.resolveSubagent}
	srv.AddReceivingMiddleware(mw.captureClient)
	tools.Register(srv, c, tools.Options{ProgressInterval: cfg.progressInterval})
	return srv
}

// config is what Options set.
type config struct {
	progressInterval  time.Duration
	subscribeDebounce time.Duration
	watcherExit       chan<- struct{}
	resolveSubagent   SubagentResolver
}

// Option configures a per-connection server.
type Option func(*config)

// WithProgressInterval sets how often wait reports progress while blocked.
// Non-positive values keep the default.
func WithProgressInterval(d time.Duration) Option {
	return func(c *config) { c.progressInterval = d }
}

// WithSubscribeDebounce sets how long the subscription watcher waits after a
// mail change before re-checking subscribed resources. Non-positive values
// keep the default.
func WithSubscribeDebounce(d time.Duration) Option {
	return func(c *config) { c.subscribeDebounce = d }
}

// WithWatcherExit makes the subscription watcher close ch once its goroutines
// have returned, so tests can prove it ends with the session.
func WithWatcherExit(ch chan<- struct{}) Option {
	return func(c *config) { c.watcherExit = ch }
}

// SubagentResolver returns the subagent that issued the tool call with the
// given runtime tool-use id, as recorded by a pre-tool hook.
type SubagentResolver func(toolUseID string) (string, bool)

// WithSubagentResolver fills in the `subagent` argument of send, read and
// mark calls that omit it, when the resolver knows which subagent made the
// call. Claude Code sends the tool-use id as _meta["claudecode/toolUseId"].
func WithSubagentResolver(r SubagentResolver) Option {
	return func(c *config) { c.resolveSubagent = r }
}

// middleware is the per-connection state of the receiving middleware.
type middleware struct {
	conn            *presence.Entry
	resolveSubagent SubagentResolver
	clientSeen      atomic.Bool
}
