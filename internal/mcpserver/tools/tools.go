package tools

import (
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver/session"
)

// DefaultProgressInterval is how often wait reports progress while blocked.
const DefaultProgressInterval = 15 * time.Second

// Options configures the tools of one connection.
type Options struct {
	// ProgressInterval is how often wait reports progress while blocked;
	// zero or less means DefaultProgressInterval.
	ProgressInterval time.Duration
}

// toolset holds what every tool handler of one connection needs.
type toolset struct {
	c                *session.Conn
	progressInterval time.Duration
}

// Register adds AIM's tools to srv, acting for the connection c. Each tool's
// types and handler live in <name>.go and its description in
// markdown/<name>.md.
func Register(srv *mcp.Server, c *session.Conn, o Options) {
	t := &toolset{c: c, progressInterval: o.ProgressInterval}
	if t.progressInterval <= 0 {
		t.progressInterval = DefaultProgressInterval
	}
	addTool(srv, "whoami", t.whoami)
	addTool(srv, "register", t.register)
	addTool(srv, "roster", t.roster)
	addTool(srv, "sign_on", t.signOn)
	addTool(srv, "sign_off", t.signOff)
	addTool(srv, "send", t.send)
	addTool(srv, "list", t.list)
	addTool(srv, "read", t.read)
	addTool(srv, "thread", t.thread)
	addTool(srv, "mark", t.mark)
	addTool(srv, "search", t.search)
	addTool(srv, "wait", t.wait)
}

// markdownDir holds one <tool name>.md description per tool. Editing these
// files changes what agents are told without touching Go code.
const markdownDir = "markdown"

//go:embed markdown/*.md
var descriptionFS embed.FS

// description returns the trimmed description of the named tool. The files
// are compiled in and covered by tests, so a missing one is a programming
// error.
func description(name string) string {
	b, err := descriptionFS.ReadFile(markdownDir + "/" + name + ".md")
	if err != nil {
		panic(fmt.Sprintf("tools: missing description %s/%s.md: %v", markdownDir, name, err))
	}
	return strings.TrimSpace(string(b))
}
