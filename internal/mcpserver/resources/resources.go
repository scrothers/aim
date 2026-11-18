package resources

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/tasks"
)

// MIMEMarkdown is the MIME type of every aim:// resource.
const MIMEMarkdown = "text/markdown"

// resource describes one aim:// resource: a static URI, or a template of a
// prefix followed by a single percent-encoded {variable} segment (see
// EncodeSegment). render receives the decoded variable, or "" for a static
// resource. Each resource is declared in its own <name>.go file and described
// in markdown/<name>.md.
type resource struct {
	name, title string
	uri         string // static resources
	prefix      string // templates
	variable    string // templates
	render      func(ctx context.Context, c *session.Conn, arg string) (string, error)
}

// template returns a template resource's URI template, such as
// aim://thread/{thread_id}.
func (r resource) template() string { return r.prefix + "{" + r.variable + "}" }

// all lists every resource in registration order: statics, then templates.
var all = []resource{me, inbox, namespaceRoster, myTasks, thread, message, mailbox, rosterOf, task}

// Register adds the aim:// resources and templates to srv, acting for c.
func Register(srv *mcp.Server, c *session.Conn) {
	read := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		text, err := render(ctx, c, uri)
		if err != nil {
			return nil, notFound(uri, err)
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: MIMEMarkdown, Text: text}}}, nil
	}
	for _, r := range all {
		if r.uri != "" {
			srv.AddResource(&mcp.Resource{URI: r.uri, Name: r.name, Title: r.title, MIMEType: MIMEMarkdown,
				Description: description(r.name)}, read)
		}
	}
	for _, r := range all {
		if r.uri == "" {
			srv.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: r.template(), Name: r.name, Title: r.title,
				MIMEType: MIMEMarkdown, Description: description(r.name)}, read)
		}
	}
}

// errNoResource marks a URI that names no resource.
var errNoResource = fmt.Errorf("resources: unknown resource: %w", mail.ErrMessageNotFound)

// render renders the Markdown for an aim:// URI. It never marks mail read.
func render(ctx context.Context, c *session.Conn, uri string) (string, error) {
	for _, r := range all {
		if r.uri != "" {
			if uri == r.uri {
				return r.render(ctx, c, "")
			}
			continue
		}
		rest, ok := strings.CutPrefix(uri, r.prefix)
		if !ok {
			continue
		}
		v, ok := decodeSegment(rest)
		if !ok {
			return "", errNoResource
		}
		return r.render(ctx, c, v)
	}
	return "", errNoResource
}

// notFound maps unknown ids and addresses to the protocol's not-found error.
func notFound(uri string, err error) error {
	if errors.Is(err, mail.ErrMessageNotFound) || errors.Is(err, mail.ErrMailboxNotFound) || errors.Is(err, tasks.ErrNotFound) {
		return mcp.ResourceNotFoundError(uri)
	}
	return session.ToolErr(err)
}

// EncodeSegment percent-encodes everything but RFC 3986 unreserved
// characters, so an address (which contains '@' and '/') survives as one path
// segment and matches the RFC 6570 simple expansion {var} in the templates.
func EncodeSegment(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := range len(s) {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '-', ch == '.', ch == '_', ch == '~':
			b.WriteByte(ch)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[ch>>4])
			b.WriteByte(hex[ch&15])
		}
	}
	return b.String()
}

// decodeSegment reverses EncodeSegment. It rejects empty values and any
// segment that still contains a path separator.
func decodeSegment(s string) (string, bool) {
	if s == "" || strings.Contains(s, "/") {
		return "", false
	}
	v, err := url.PathUnescape(s)
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// markdownDir holds one <resource name>.md description per resource.
const markdownDir = "markdown"

//go:embed markdown/*.md
var descriptionFS embed.FS

// description returns the trimmed description of the named resource. The
// files are compiled in and covered by tests, so a missing one is a
// programming error.
func description(name string) string {
	b, err := descriptionFS.ReadFile(markdownDir + "/" + name + ".md")
	if err != nil {
		panic(fmt.Sprintf("resources: missing description %s/%s.md: %v", markdownDir, name, err))
	}
	return strings.TrimSpace(string(b))
}
