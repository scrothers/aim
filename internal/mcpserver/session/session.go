package session

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/tasks"
)

// Conn is one MCP connection: the mail service it acts through, the task
// board, and the presence entry whose preamble and sign-on identify the
// caller.
type Conn struct {
	Svc   *mail.Service
	Entry *presence.Entry
	// Tasks is the task board; nil when the server was built without one,
	// in which case task tools and resources report it unavailable.
	Tasks *tasks.Service
}

// ErrNoTasks is returned by task tools on a server without a task board.
var ErrNoTasks = errors.New("the task board is not available on this server")

// TaskBoard returns the task board or ErrNoTasks.
func (c *Conn) TaskBoard() (*tasks.Service, error) {
	if c.Tasks == nil {
		return nil, ErrNoTasks
	}
	return c.Tasks, nil
}

// Caller returns the mail caller for the connection's current sign-on.
func (c *Conn) Caller() mail.Caller {
	id, ok := c.Entry.Identity()
	st := c.Entry.Status()
	return mail.Caller{
		SessionID: st.SessionID, Identity: id, SignedOn: ok,
		Client: st.Client, Machine: st.Machine,
	}
}

// AddressHint is appended to address errors to remind agents of the grammar.
const AddressHint = " (address grammar: [subagent@][identity@]host/org[/repo])"

// RequireAddress rejects an empty address: unlike the operator console, agent
// tools never read "everything".
func RequireAddress(op, addr string) error {
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("mcpserver: %s: %w: address is required%s", op, mail.ErrInvalidInput, AddressHint)
	}
	return nil
}

// ToolErr decorates service errors with guidance for agents.
func ToolErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, address.ErrInvalid) {
		return fmt.Errorf("%w%s", err, AddressHint)
	}
	return err
}

// ParseTime parses an optional RFC3339 timestamp; empty yields a nil pointer.
func ParseTime(name, s string) (*time.Time, error) {
	var out *time.Time
	if s == "" {
		return out, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return out, fmt.Errorf("%s must be an RFC3339 timestamp like 2026-01-02T15:04:05Z: %w", name, err)
	}
	return &t, nil
}
