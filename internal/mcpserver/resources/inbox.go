package resources

import (
	"context"

	"github.com/scrothers/aim/internal/mcpserver/session"
)

// URIInbox is the unread digest of the signed-on identity.
const URIInbox = "aim://inbox"

const notSignedOnHint = "Not signed on, so there is no inbox. Call `agent_register` (once) and `agent_sign_on` with your identity, then read this resource again."

var inbox = resource{name: "inbox", title: unreadTitle, uri: URIInbox, render: renderInbox}

// Inbox renders the inbox resource for c: the unread digest of its signed-on
// identity and subagents, or a sign-on hint.
func Inbox(ctx context.Context, c *session.Conn) (string, error) {
	return renderInbox(ctx, c, "")
}

func renderInbox(ctx context.Context, c *session.Conn, _ string) (string, error) {
	id, ok := c.Entry.Identity()
	if !ok {
		return "# " + unreadTitle + "\n\n" + notSignedOnHint + "\n", nil
	}
	return renderMailbox(ctx, c, id.String())
}
