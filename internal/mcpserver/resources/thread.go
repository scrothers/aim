package resources

import (
	"context"

	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mdview"
)

// PrefixThread starts the thread template; the thread id follows encoded.
const PrefixThread = "aim://thread/"

var thread = resource{name: "thread", title: "Thread transcript", prefix: PrefixThread, variable: "thread_id", render: Thread}

// ThreadURI returns the resource URI of a thread.
func ThreadURI(id string) string { return PrefixThread + EncodeSegment(id) }

// Thread renders a thread's transcript, oldest first.
func Thread(ctx context.Context, c *session.Conn, id string) (string, error) {
	ms, err := c.Svc.Thread(ctx, id)
	if err != nil {
		return "", err
	}
	return mdview.Transcript(ms), nil
}
