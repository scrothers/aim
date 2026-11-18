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
	addTool(srv, "agent_whoami", t.whoami)
	addTool(srv, "agent_register", t.register)
	addTool(srv, "agent_roster", t.roster)
	addTool(srv, "agent_sign_on", t.signOn)
	addTool(srv, "agent_sign_off", t.signOff)
	addTool(srv, "mail_send", t.send)
	addTool(srv, "mail_list", t.list)
	addTool(srv, "mail_read", t.read)
	addTool(srv, "mail_thread", t.thread)
	addTool(srv, "mail_mark", t.mark)
	addTool(srv, "mail_search", t.search)
	addTool(srv, "mail_wait", t.wait)
	addTool(srv, "task_create", t.taskCreate)
	addTool(srv, "task_list", t.taskList)
	addTool(srv, "task_get", t.taskGet)
	addTool(srv, "task_update", t.taskUpdate)
}

// meta is one tool's human title and behavior hints. Every AIM tool is
// closed-world (messages never leave AIM) and none is destructive, so only
// read-only and idempotent vary.
type meta struct {
	title      string
	readOnly   bool
	idempotent bool
}

// annotations builds the MCP annotations for m. The pointer hints default to
// true in the spec, so they are always set explicitly.
func (m meta) annotations() *mcp.ToolAnnotations {
	no := false
	return &mcp.ToolAnnotations{
		Title:           m.title,
		ReadOnlyHint:    m.readOnly,
		IdempotentHint:  m.idempotent,
		DestructiveHint: &no,
		OpenWorldHint:   &no,
	}
}

// toolMeta maps each tool name to its title and hints. Registration order is
// set by Register, not this table.
var toolMeta = map[string]meta{
	"agent_whoami":   {title: "Who am I", readOnly: true},
	"agent_register": {title: "Register identity", idempotent: true},
	"agent_roster":   {title: "Roster", readOnly: true},
	"agent_sign_on":  {title: "Sign on", idempotent: true},
	"agent_sign_off": {title: "Sign off", idempotent: true},
	"mail_send":      {title: "Send message"},
	"mail_list":      {title: "List mail", readOnly: true},
	// mail_read marks mail read unless peek is set, so it is not read-only.
	"mail_read":   {title: "Read message", idempotent: true},
	"mail_thread": {title: "Read thread", readOnly: true},
	// mail_mark's archive is reversible, so it is not destructive.
	"mail_mark":   {title: "Mark messages", idempotent: true},
	"mail_search": {title: "Search mail", readOnly: true},
	// Creating twice makes two tasks, and update actions such as comment
	// append history, so neither is idempotent.
	"task_create": {title: "Create task"},
	"task_list":   {title: "List tasks", readOnly: true},
	"task_get":    {title: "Get task", readOnly: true},
	"task_update": {title: "Update task"},
	"mail_wait":   {title: "Wait for mail", readOnly: true},
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
