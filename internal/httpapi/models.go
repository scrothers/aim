package httpapi

import (
	"time"

	"github.com/scrothers/aim/internal/mail"
)

// MessageView is a message with its Markdown body rendered to sanitized HTML.
type MessageView struct {
	mail.Message
	BodyHTML string `json:"body_html" doc:"Body rendered from Markdown to sanitized HTML; safe to inject."`
	Snippet  string `json:"snippet" doc:"Plain-text preview of the body with Markdown syntax removed."`
}

// snippetRunes is the length of a plain-text preview.
const snippetRunes = 140

// Buddy is one signed-on session (or, in the offline group, one identity).
type Buddy struct {
	Address       string    `json:"address" doc:"Identity address."`
	Name          string    `json:"name" doc:"Identity name, the part before the first @."`
	Role          string    `json:"role" doc:"Registered role."`
	Online        bool      `json:"online" doc:"True when this entry is a signed-on session."`
	Away          string    `json:"away,omitempty" doc:"Away message, empty when present."`
	Client        string    `json:"client,omitempty" doc:"MCP client name, for example claude-code."`
	ClientVersion string    `json:"client_version,omitempty" doc:"MCP client version."`
	Machine       string    `json:"machine,omitempty" doc:"Hostname the session runs on."`
	Subagents     []string  `json:"subagents" doc:"Subagent names that have mailboxes under this identity."`
	SignedOnAt    time.Time `json:"signed_on_at,omitzero" doc:"When the session signed on."`
}

// BuddyGroup is a repo or org namespace and its buddies.
type BuddyGroup struct {
	Name    string  `json:"name" doc:"Namespace (host/org[/repo]) or Offline."`
	Offline bool    `json:"offline" doc:"True for the group of identities with no signed-on session."`
	Online  int     `json:"online" doc:"Identities in the namespace with at least one session."`
	Total   int     `json:"total" doc:"Active identities in the namespace."`
	Buddies []Buddy `json:"buddies"`
}

// PresenceEvent is one entry of the sign-on feed.
type PresenceEvent struct {
	Kind     string    `json:"kind" enum:"signed_on,signed_off,connected,disconnected" doc:"What happened."`
	Identity string    `json:"identity,omitempty" doc:"Identity address, when signed on."`
	Machine  string    `json:"machine,omitempty"`
	Client   string    `json:"client,omitempty"`
	At       time.Time `json:"at"`
}

// PresenceBody is the buddy list.
type PresenceBody struct {
	Groups []BuddyGroup    `json:"groups" doc:"Online groups by namespace, then the offline group."`
	Events []PresenceEvent `json:"events" doc:"Recent presence events, newest first."`
}

// StatsBody is daemon-wide totals.
type StatsBody struct {
	mail.Stats
	Version string `json:"version" doc:"Daemon version."`
}

// RosterBody lists identities.
type RosterBody struct {
	Items []mail.RosterEntry `json:"items"`
}

// ThreadBody is a thread, oldest message first.
type ThreadBody struct {
	Messages []MessageView `json:"messages"`
}

// SendRequest is the body of POST /api/messages.
type SendRequest struct {
	To        []string `json:"to,omitempty" doc:"Recipient addresses. May be empty with reply_all."`
	Cc        []string `json:"cc,omitempty" doc:"Carbon-copy addresses."`
	Subject   string   `json:"subject" doc:"Subject line."`
	Body      string   `json:"body" doc:"Markdown body."`
	InReplyTo string   `json:"in_reply_to,omitempty" doc:"Message id being replied to."`
	ReplyAll  bool     `json:"reply_all,omitempty" doc:"Address the reply to everyone on in_reply_to."`
}

// MarkRequest is the body of POST /api/marks.
type MarkRequest struct {
	MessageIDs []string `json:"message_ids" doc:"Messages delivered to the operator mailbox."`
	State      string   `json:"state" enum:"read,unread,archived,unarchived" doc:"State to apply."`
}

// MarkBody reports how many deliveries changed.
type MarkBody struct {
	Updated int `json:"updated"`
}

// UpdateRequest is the body of PATCH /api/identities. Omitted fields are left
// unchanged; an empty string clears description or reports_to.
type UpdateRequest struct {
	Role        *string `json:"role,omitempty" doc:"New role."`
	Description *string `json:"description,omitempty" doc:"New description."`
	ReportsTo   *string `json:"reports_to,omitempty" doc:"New reports_to address, or operator."`
}

// RenderRequest is the body of POST /api/render.
type RenderRequest struct {
	Markdown string `json:"markdown" doc:"Markdown source to preview."`
}

// RenderBody is the rendered preview.
type RenderBody struct {
	HTML string `json:"html" doc:"Sanitized HTML."`
}
