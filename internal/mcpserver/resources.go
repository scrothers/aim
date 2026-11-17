package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
)

// Resource URIs. Static resources describe this connection; templates take a
// single percent-encoded path segment (see encodeSegment).
const (
	uriMe            = "aim://me"
	uriInbox         = "aim://inbox"
	uriRoster        = "aim://roster"
	prefixThread     = "aim://thread/"
	prefixMessage    = "aim://message/"
	prefixMailbox    = "aim://mailbox/"
	prefixRosterOf   = "aim://roster/"
	markdownMIME     = "text/markdown"
	digestLimit      = mail.DefaultListLimit
	noNamespaceHint  = "This session has no git namespace (no `origin` remote), so there is no default roster. Read `aim://roster/{address}` or call the `roster` tool with an org or repo address."
	notSignedOnHint  = "Not signed on, so there is no inbox. Call `register` (once) and `sign_on` with your identity, then read this resource again."
	unreadInboxTitle = "Unread mail"
)

// encodeSegment percent-encodes everything but RFC 3986 unreserved characters,
// so an address (which contains '@' and '/') survives as one path segment and
// matches the RFC 6570 simple expansion {var} in the resource templates.
func encodeSegment(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// decodeSegment reverses encodeSegment. It rejects empty values and any
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

// registerResources adds the aim:// resources and templates.
func (h *handlers) registerResources(srv *mcp.Server) {
	static := []*mcp.Resource{
		{URI: uriMe, Name: "me", Title: "This session", MIMEType: markdownMIME,
			Description: "Who this connection is: session, machine, namespace and signed-on identity."},
		{URI: uriInbox, Name: "inbox", Title: "Unread mail", MIMEType: markdownMIME,
			Description: "Unread digest for your signed-on identity and its subagents, newest first."},
		{URI: uriRoster, Name: "roster", Title: "Namespace roster", MIMEType: markdownMIME,
			Description: "Identities registered in this session's git namespace, with presence."},
	}
	for _, r := range static {
		srv.AddResource(r, h.readResource)
	}
	templates := []*mcp.ResourceTemplate{
		{URITemplate: prefixThread + "{thread_id}", Name: "thread", Title: "Thread transcript", MIMEType: markdownMIME,
			Description: "Full conversation, oldest first."},
		{URITemplate: prefixMessage + "{message_id}", Name: "message", Title: "Message", MIMEType: markdownMIME,
			Description: "One message. Reading it never marks it read."},
		{URITemplate: prefixMailbox + "{address}", Name: "mailbox", Title: "Mailbox digest", MIMEType: markdownMIME,
			Description: "Unread digest for any address subtree. Percent-encode the address."},
		{URITemplate: prefixRosterOf + "{address}", Name: "roster-of", Title: "Roster of an address", MIMEType: markdownMIME,
			Description: "Roster for any org, repo or identity. Percent-encode the address."},
	}
	for _, t := range templates {
		srv.AddResourceTemplate(t, h.readResource)
	}
}

// readResource serves every aim:// resource. It never marks mail read.
func (h *handlers) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := req.Params.URI
	text, err := h.renderURI(ctx, uri)
	if err != nil {
		return nil, h.resourceErr(uri, err)
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: markdownMIME, Text: text}}}, nil
}

// resourceErr maps unknown ids and addresses to the protocol's not-found error.
func (h *handlers) resourceErr(uri string, err error) error {
	if errors.Is(err, mail.ErrMessageNotFound) || errors.Is(err, mail.ErrMailboxNotFound) {
		return mcp.ResourceNotFoundError(uri)
	}
	return toolErr(err)
}

// errNoResource marks a URI that names no resource.
var errNoResource = fmt.Errorf("mcpserver: unknown resource: %w", mail.ErrMessageNotFound)

// renderURI renders the Markdown for an aim:// URI.
func (h *handlers) renderURI(ctx context.Context, uri string) (string, error) {
	switch uri {
	case uriMe:
		return h.meMarkdown(), nil
	case uriInbox:
		return h.inboxMarkdown(ctx)
	case uriRoster:
		return h.namespaceRosterMarkdown(ctx)
	}
	kinds := []struct {
		prefix string
		render func(context.Context, string) (string, error)
	}{
		{prefixThread, h.threadMarkdown},
		{prefixMessage, h.messageMarkdown},
		{prefixMailbox, h.mailboxMarkdown},
		{prefixRosterOf, h.rosterMarkdown},
	}
	for _, k := range kinds {
		rest, ok := strings.CutPrefix(uri, k.prefix)
		if !ok {
			continue
		}
		v, ok := decodeSegment(rest)
		if !ok {
			return "", errNoResource
		}
		return k.render(ctx, v)
	}
	return "", errNoResource
}

func (h *handlers) meMarkdown() string {
	p := h.conn.Preamble()
	st := h.conn.Status()
	w := whoamiInfo{
		SessionID: p.SessionID, Machine: p.Machine, ProjectDir: p.ProjectDir, GitRemote: p.GitRemote,
		Namespace: p.Namespace, Client: st.Client, ClientVersion: st.ClientVersion,
	}
	if id, ok := h.conn.Identity(); ok {
		w.SignedOnAs = id.String()
	}
	return renderWhoami(w)
}

func (h *handlers) inboxMarkdown(ctx context.Context) (string, error) {
	id, ok := h.conn.Identity()
	if !ok {
		return "# " + unreadInboxTitle + "\n\n" + notSignedOnHint + "\n", nil
	}
	return h.mailboxMarkdown(ctx, id.String())
}

func (h *handlers) mailboxMarkdown(ctx context.Context, addr string) (string, error) {
	res, err := h.svc.List(ctx, mail.ListInput{Address: addr, State: mail.StateUnread, Limit: digestLimit})
	if err != nil {
		return "", err
	}
	return renderDigest(unreadInboxTitle+" for "+addr, res.Items, res.NextCursor != ""), nil
}

func (h *handlers) namespaceRosterMarkdown(ctx context.Context) (string, error) {
	ns := h.conn.Preamble().Namespace
	if ns == "" {
		return "# Roster\n\n" + noNamespaceHint + "\n", nil
	}
	return h.rosterMarkdown(ctx, ns)
}

func (h *handlers) rosterMarkdown(ctx context.Context, addr string) (string, error) {
	es, err := h.svc.Roster(ctx, addr)
	if err != nil {
		return "", err
	}
	return renderRoster("Roster of "+addr, es), nil
}

func (h *handlers) threadMarkdown(ctx context.Context, id string) (string, error) {
	ms, err := h.svc.Thread(ctx, id)
	if err != nil {
		return "", err
	}
	return renderTranscript(ms), nil
}

func (h *handlers) messageMarkdown(ctx context.Context, id string) (string, error) {
	m, err := h.svc.Read(ctx, h.caller(), mail.ReadInput{MessageID: id, Peek: true})
	if err != nil {
		return "", err
	}
	return renderMessage(m), nil
}
