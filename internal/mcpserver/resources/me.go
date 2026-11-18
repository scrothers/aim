package resources

import (
	"context"

	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mdview"
)

// URIMe is this connection's identity resource.
const URIMe = "aim://me"

var me = resource{name: "me", title: "This session", uri: URIMe, render: renderMe}

func renderMe(_ context.Context, c *session.Conn, _ string) (string, error) {
	p := c.Entry.Preamble()
	st := c.Entry.Status()
	w := mdview.WhoamiInfo{
		SessionID: p.SessionID, Machine: p.Machine, ProjectDir: p.ProjectDir, GitRemote: p.GitRemote,
		Namespace: p.Namespace, Client: st.Client, ClientVersion: st.ClientVersion,
	}
	if id, ok := c.Entry.Identity(); ok {
		w.SignedOnAs = id.String()
	}
	return mdview.Whoami(w), nil
}
