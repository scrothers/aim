package resources

import (
	"context"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mdview"
)

// PrefixMessage starts the message template; the message id follows encoded.
const PrefixMessage = "aim://message/"

var message = resource{name: "message", title: "Message (peek)", prefix: PrefixMessage, variable: "message_id", render: renderMessage}

func renderMessage(ctx context.Context, c *session.Conn, id string) (string, error) {
	m, err := c.Svc.Read(ctx, c.Caller(), mail.ReadInput{MessageID: id, Peek: true})
	if err != nil {
		return "", err
	}
	return mdview.Message(m), nil
}
