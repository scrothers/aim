package resources

import (
	"context"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mdview"
)

// PrefixMailbox starts the mailbox template; the address follows encoded.
const PrefixMailbox = "aim://mailbox/"

const (
	unreadTitle = "Unread mail"
	digestLimit = mail.DefaultListLimit
)

var mailbox = resource{name: "mailbox", title: "Mailbox digest", prefix: PrefixMailbox, variable: "address", render: renderMailbox}

func renderMailbox(ctx context.Context, c *session.Conn, addr string) (string, error) {
	res, err := c.Svc.List(ctx, mail.ListInput{Address: addr, State: mail.StateUnread, Limit: digestLimit})
	if err != nil {
		return "", err
	}
	return mdview.Digest(unreadTitle+" for "+addr, res.Items, res.NextCursor != ""), nil
}
