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
	SessionID  string    `json:"session_id" jsonschema:"Session id of the signed-on connection."`
	Machine    string    `json:"machine" jsonschema:"Hostname of the machine the session runs on."`
	Away       string    `json:"away,omitempty" jsonschema:"Status the session set with sign_on, often its current task, for example reviewing PR 212."`
	SignedOnAt time.Time `json:"signed_on_at" jsonschema:"When the session signed on, RFC3339."`
	Client     string    `json:"client,omitempty" jsonschema:"MCP client of the session, for example claude-code."`
	// State is "online" or "idle" (no tool call within the idle threshold).
	State SessionState `json:"state,omitempty" jsonschema:"online, or idle when the session has made no tool call within the idle threshold (10 minutes by default)."`
	// LastActive is the time of the session's latest tool call.
	LastActive time.Time `json:"last_active,omitzero" jsonschema:"Time of the session's latest tool call, RFC3339."`
	// Attention is the state the agent's hooks reported (blocked, waiting,
	// failed or interrupted), or empty when it needs none.
	Attention Attention `json:"attention,omitempty" jsonschema:"blocked, waiting, failed or interrupted when the agent's hooks report it needs a human; absent otherwise. Such a session is unlikely to answer soon."`
	// AttentionDetail is the runtime's message for the attention state.
	AttentionDetail string `json:"attention_detail,omitempty" jsonschema:"The runtime's message for the attention state, such as the pending permission prompt."`
	// AttentionSince is when the attention state began.
	AttentionSince time.Time `json:"attention_since,omitzero" jsonschema:"When the attention state began, RFC3339."`
}

// Identity is a registered identity mailbox.
type Identity struct {
	Address     string    `json:"address" jsonschema:"Canonical lower-case identity address, for example reviewer@github.com/acme/api. Pass it to sign_on."`
	Role        string    `json:"role" jsonschema:"Short role title shown in rosters."`
	Description string    `json:"description" jsonschema:"What the identity does and what to send it."`
	ReportsTo   string    `json:"reports_to,omitempty" jsonschema:"Address this identity escalates to, or operator; absent when unset."`
	CreatedAt   time.Time `json:"created_at" jsonschema:"When the identity was registered, RFC3339."`
	// RetiredAt is nil for active identities.
	RetiredAt *time.Time `json:"retired_at,omitempty" jsonschema:"When the operator retired the identity; absent while active. Retired identities cannot sign on or receive mail."`
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
	Address     string         `json:"address" jsonschema:"Identity address, for example reviewer@github.com/acme/api. Use it as a send recipient or sign_on address."`
	Role        string         `json:"role" jsonschema:"Short role title."`
	Description string         `json:"description" jsonschema:"What the identity does and what to send it; match your request against it."`
	ReportsTo   string         `json:"reports_to,omitempty" jsonschema:"Address this identity escalates to, or operator; follow it upward for unclear or cross-repo work."`
	Online      []OnlineStatus `json:"online" jsonschema:"Sessions signed on as this identity now. Empty means nobody is online; mail still waits in its inbox."`
	Subagents   []string       `json:"subagents" jsonschema:"Names of subagent mailboxes under this identity, for example tests for tests@reviewer@github.com/acme/api."`
	RetiredAt   *time.Time     `json:"retired_at,omitempty" jsonschema:"When the identity was retired; absent for active identities, which are the only ones roster lists."`
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
	ID        string    `json:"id" jsonschema:"Message id (msg-...). Pass it to read, to send as in_reply_to, or to wait as since_id."`
	ThreadID  string    `json:"thread_id" jsonschema:"Conversation id, equal to the id of the thread's first message. Pass it to thread or aim://thread/{thread_id}."`
	InReplyTo string    `json:"in_reply_to,omitempty" jsonschema:"Id of the message this one answers; absent for the first message of a thread."`
	From      string    `json:"from" jsonschema:"Sender address; a subagent address such as tests@reviewer@github.com/acme/api when sent as a subagent."`
	Subject   string    `json:"subject" jsonschema:"Subject line."`
	Body      string    `json:"body" jsonschema:"Markdown body, exactly as sent."`
	CreatedAt time.Time `json:"created_at" jsonschema:"When the message was sent, RFC3339 UTC."`
	To        []string  `json:"to" jsonschema:"Addresses the message was delivered to as to."`
	Cc        []string  `json:"cc" jsonschema:"Addresses the message was delivered to as cc."`
	// FromClient and FromMachine record the sender's MCP client name and
	// machine at send time; they are empty for mail sent before they were
	// tracked.
	FromClient  string `json:"from_client,omitempty" jsonschema:"Sender's MCP client at send time, for example claude-code; absent for older mail."`
	FromMachine string `json:"from_machine,omitempty" jsonschema:"Hostname of the sender's machine at send time; absent for older mail."`
	// Marked reports whether a non-peek Read left a delivery of this message
	// marked read for the caller. It is false for peeks and for callers who
	// are not signed on as an owner of the message. It is set only by Read
	// and is not part of the JSON form.
	Marked bool `json:"-"`
	// Muted reports whether the thread is muted for the operator. It is set
	// by Read and Thread.
	Muted bool `json:"muted" jsonschema:"true when the human operator muted this thread; it does not affect agents."`
}

// Header is a message summary as delivered to one mailbox.
type Header struct {
	ID         string       `json:"id" jsonschema:"Message id (msg-...). Pass it to read or mark, to send as in_reply_to, or the newest one to wait as since_id."`
	ThreadID   string       `json:"thread_id" jsonschema:"Conversation id, equal to the id of the thread's first message. Pass it to thread."`
	InReplyTo  string       `json:"in_reply_to,omitempty" jsonschema:"Id of the message this one answers; absent for the first message of a thread."`
	From       string       `json:"from" jsonschema:"Sender address."`
	Subject    string       `json:"subject" jsonschema:"Subject line."`
	CreatedAt  time.Time    `json:"created_at" jsonschema:"When the message was sent, RFC3339 UTC."`
	To         []string     `json:"to" jsonschema:"Addresses the message was delivered to as to."`
	Cc         []string     `json:"cc" jsonschema:"Addresses the message was delivered to as cc."`
	Mailbox    string       `json:"mailbox" jsonschema:"Address this copy was delivered to. A message delivered to several mailboxes in scope appears once per mailbox."`
	Kind       DeliveryKind `json:"kind" jsonschema:"to or cc: how this mailbox was addressed."`
	ReadAt     *time.Time   `json:"read_at,omitempty" jsonschema:"When this copy was marked read; absent while unread. Org and repo mailbox copies are never marked."`
	ArchivedAt *time.Time   `json:"archived_at,omitempty" jsonschema:"When this copy was archived; absent unless archived."`
	// FromClient and FromMachine describe the sender's MCP client and host at
	// send time; empty for mail sent before they were tracked.
	FromClient  string `json:"from_client,omitempty" jsonschema:"Sender's MCP client at send time, for example claude-code; absent for older mail."`
	FromMachine string `json:"from_machine,omitempty" jsonschema:"Hostname of the sender's machine at send time; absent for older mail."`
	// Snippet is a plain-text preview of the body.
	Snippet string `json:"snippet" jsonschema:"Plain-text preview of the body, up to 140 characters. Call read for the full body."`
	// ThreadCount is the number of messages in the thread.
	ThreadCount int `json:"thread_count" jsonschema:"Number of messages in the thread; above 1 means there is history to read with thread."`
	// Muted reports whether the thread is muted; set only on operator
	// mailbox deliveries.
	Muted bool `json:"muted" jsonschema:"true when the human operator muted this thread; set only on operator mailbox copies."`
}

// DeliveryKind is how a mailbox was addressed on a message: DeliveryTo or
// DeliveryCc.
type DeliveryKind string

// Delivery kinds.
const (
	DeliveryTo DeliveryKind = kindTo
	DeliveryCc DeliveryKind = kindCc
)

// SessionState is whether a signed-on session is active: "online", or "idle"
// after no tool call within the idle threshold. The presence package defines
// the values.
type SessionState string

// Attention is the state an agent's hooks reported when it needs a human
// ("blocked", "waiting", "failed" or "interrupted"), or empty. The wire
// package defines the values.
type Attention string

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
	Items      []Header `json:"items" jsonschema:"Deliveries, newest first, one per mailbox a message reached in scope."`
	NextCursor string   `json:"next_cursor,omitempty" jsonschema:"Pass as cursor, with the same other arguments, for the next page; absent on the last page."`
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
	// From restricts hits to messages sent by this address.
	From string
	// Since and Until bound the message creation time (inclusive).
	Since, Until *time.Time
	// State restricts hits by delivery state: unread, read, archived, inbox
	// or all; empty means all.
	State string
}

// Hit is one ranked search result.
type Hit struct {
	MessageID string `json:"message_id" jsonschema:"Id of the matching message; pass it to read."`
	ThreadID  string `json:"thread_id" jsonschema:"Conversation id of the matching message; pass it to thread for the full discussion."`
	Subject   string `json:"subject" jsonschema:"Subject line."`
	// Snippet is the matching excerpt with matches wrapped in [brackets].
	Snippet string `json:"snippet" jsonschema:"Matching passage with matched terms in [square brackets]."`
	// Marked is Snippet with matches wrapped in store.MarkOpen and
	// store.MarkClose instead, for callers that render highlights.
	Marked    string    `json:"-"`
	From      string    `json:"from" jsonschema:"Sender address."`
	Mailbox   string    `json:"mailbox" jsonschema:"Mailbox in scope the message was delivered to."`
	CreatedAt time.Time `json:"created_at" jsonschema:"When the message was sent, RFC3339 UTC."`
	Rank      float64   `json:"rank" jsonschema:"bm25 relevance score; lower is a better match."`
}

// SearchResult is one page of hits, best first.
type SearchResult struct {
	Hits       []Hit  `json:"hits" jsonschema:"Matches, best first; a message delivered to several mailboxes in scope appears once."`
	NextCursor string `json:"next_cursor,omitempty" jsonschema:"Pass as cursor, with the same address and query, for the next page; absent on the last page. Best-effort: hits can shift as mail arrives."`
}
