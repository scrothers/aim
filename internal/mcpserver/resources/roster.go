package resources

import (
	"context"

	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mdview"
)

// Roster URIs: the session namespace's roster, and the template for any
// address's roster.
const (
	URIRoster      = "aim://roster"
	PrefixRosterOf = "aim://roster/"
)

const noNamespaceHint = "This session has no git namespace (no `origin` remote), so there is no default roster. Read `aim://roster/{address}` or call the `roster` tool with an org or repo address."

var (
	namespaceRoster = resource{name: "roster", title: "Namespace roster", uri: URIRoster, render: renderNamespaceRoster}
	rosterOf        = resource{name: "roster-of", title: "Roster of an address", prefix: PrefixRosterOf, variable: "address", render: renderRoster}
)

func renderNamespaceRoster(ctx context.Context, c *session.Conn, _ string) (string, error) {
	ns := c.Entry.Preamble().Namespace
	if ns == "" {
		return "# Roster\n\n" + noNamespaceHint + "\n", nil
	}
	return renderRoster(ctx, c, ns)
}

func renderRoster(ctx context.Context, c *session.Conn, addr string) (string, error) {
	es, err := c.Svc.Roster(ctx, addr)
	if err != nil {
		return "", err
	}
	return mdview.Roster("Roster of "+addr, es), nil
}
