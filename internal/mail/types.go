package mail

import (
	"time"

	"github.com/scrothers/aim/internal/address"
)

// Presence reports which sessions are currently signed on as an identity.
type Presence interface {
	Online(identity address.Address) []OnlineStatus
}

// OnlineStatus describes one signed-on session of an identity.
type OnlineStatus struct {
	SessionID  string    `json:"session_id"`
	Machine    string    `json:"machine"`
	Away       string    `json:"away,omitempty"`
	SignedOnAt time.Time `json:"signed_on_at"`
	Client     string    `json:"client,omitempty"`
	// State is "online" or "idle" (no tool call within the idle threshold).
	State string `json:"state,omitempty"`
	// LastActive is the time of the session's latest tool call.
	LastActive time.Time `json:"last_active,omitzero"`
}

// Identity is a registered identity mailbox.
type Identity struct {
	Address     string    `json:"address"`
	Role        string    `json:"role"`
	Description string    `json:"description"`
	ReportsTo   string    `json:"reports_to,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	// RetiredAt is nil for active identities.
	RetiredAt *time.Time `json:"retired_at,omitempty"`
}

// UpdateInput is the argument of Service.UpdateIdentity. A nil field is left
// unchanged; a pointer to the empty string clears reports_to or description.
type UpdateInput struct {
	Role        *string
	Description *string
	ReportsTo   *string
}

// RegisterInput is the argument of Service.Register.
type RegisterInput struct {
	Address     string
	Role        string
	Description string
	ReportsTo   string
}

// RosterEntry is one identity in a roster listing.
type RosterEntry struct {
	Address     string         `json:"address"`
	Role        string         `json:"role"`
	Description string         `json:"description"`
	ReportsTo   string         `json:"reports_to,omitempty"`
	Online      []OnlineStatus `json:"online"`
	Subagents   []string       `json:"subagents"`
	RetiredAt   *time.Time     `json:"retired_at,omitempty"`
}

// SendInput is the argument of Service.Send.
type SendInput struct {
	To        []string
	Cc        []string
	Subject   string
	Body      string
	InReplyTo string
	Subagent  string
	ReplyAll  bool
}

// Message is a full message with its recipients.
type Message struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id"`
	InReplyTo string    `json:"in_reply_to,omitempty"`
	From      string    `json:"from"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	To        []string  `json:"to"`
	Cc        []string  `json:"cc"`
	// FromClient and FromMachine record the sender's MCP client name and
	// machine at send time; they are empty for mail sent before they were
	// tracked.
	FromClient  string `json:"from_client,omitempty"`
	FromMachine string `json:"from_machine,omitempty"`
	// Marked reports whether a non-peek Read left a delivery of this message
	// marked read for the caller. It is false for peeks and for callers who
	// are not signed on as an owner of the message. It is set only by Read
	// and is not part of the JSON form.
	Marked bool `json:"-"`
	// Muted reports whether the thread is muted for the operator. It is set
	// by Read and Thread.
	Muted bool `json:"muted"`
}

// Header is a message summary as delivered to one mailbox.
type Header struct {
	ID         string     `json:"id"`
	ThreadID   string     `json:"thread_id"`
	InReplyTo  string     `json:"in_reply_to,omitempty"`
	From       string     `json:"from"`
	Subject    string     `json:"subject"`
	CreatedAt  time.Time  `json:"created_at"`
	To         []string   `json:"to"`
	Cc         []string   `json:"cc"`
	Mailbox    string     `json:"mailbox"`
	Kind       string     `json:"kind"`
	ReadAt     *time.Time `json:"read_at,omitempty"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	// FromClient and FromMachine describe the sender's MCP client and host at
	// send time; empty for mail sent before they were tracked.
	FromClient  string `json:"from_client,omitempty"`
	FromMachine string `json:"from_machine,omitempty"`
	// Snippet is a plain-text preview of the body.
	Snippet string `json:"snippet"`
	// ThreadCount is the number of messages in the thread.
	ThreadCount int `json:"thread_count"`
	// Muted reports whether the thread is muted; set only on operator
	// mailbox deliveries.
	Muted bool `json:"muted"`
}

// List states.
const (
	StateUnread   = "unread"
	StateRead     = "read"
	StateArchived = "archived"
	// StateInbox matches every delivery that is not archived, read or unread.
	StateInbox = "inbox"
	StateAll   = "all"
)

// snippetRunes is the length of a plain-text preview.
const snippetRunes = 140

// Limits.
const (
	DefaultListLimit = 50
	MaxListLimit     = 200
	MaxSubjectRunes  = 200
	MaxBodyBytes     = 65536
)

// ListInput is the argument of Service.List.
type ListInput struct {
	Address string
	State   string
	From    string
	Since   *time.Time
	Until   *time.Time
	Limit   int
	Cursor  string
	// IncludeMuted lists operator mail in muted threads, which are hidden
	// from the operator inbox by default.
	IncludeMuted bool
}

// ListResult is one page of headers, newest first.
type ListResult struct {
	Items      []Header `json:"items"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

// ReadInput is the argument of Service.Read.
type ReadInput struct {
	MessageID string
	Subagent  string
	Peek      bool
}

// Mark states.
const (
	MarkRead       = "read"
	MarkUnread     = "unread"
	MarkArchived   = "archived"
	MarkUnarchived = "unarchived"
)

// MarkInput is the argument of Service.Mark.
type MarkInput struct {
	MessageIDs []string
	State      string
	Subagent   string
}

// Search limits.
const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 100
)

// SearchInput is the argument of Service.Search. Cursor is a best-effort
// offset into the ranked results: it is not a stable snapshot, so results may
// shift, repeat or be skipped if new mail arrives between pages.
type SearchInput struct {
	Address string
	Query   string
	Limit   int
	Cursor  string
}

// Hit is one ranked search result.
type Hit struct {
	MessageID string    `json:"message_id"`
	ThreadID  string    `json:"thread_id"`
	Subject   string    `json:"subject"`
	Snippet   string    `json:"snippet"`
	From      string    `json:"from"`
	Mailbox   string    `json:"mailbox"`
	CreatedAt time.Time `json:"created_at"`
	Rank      float64   `json:"rank"`
}

// SearchResult is one page of hits, best first.
type SearchResult struct {
	Hits       []Hit  `json:"hits"`
	NextCursor string `json:"next_cursor,omitempty"`
}
