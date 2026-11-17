package mcpserver

import (
	"fmt"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/mail"
)

// The renderers below turn mail data into Markdown. Resources and prompts
// share them so an agent sees identical text either way. They are pure: no
// I/O, no clock, so golden-file tests can pin their output.

// timeLayout is the timestamp format used in rendered Markdown.
const timeLayout = "2006-01-02 15:04 MST"

func stamp(t time.Time) string { return t.UTC().Format(timeLayout) }

// oneLine collapses all whitespace runs so a value fits on a single line.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// renderTranscript renders a conversation, oldest first.
func renderTranscript(msgs []mail.Message) string {
	if len(msgs) == 0 {
		return "_No messages._\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Thread %s\n\n%d message(s), oldest first.\n", msgs[0].ThreadID, len(msgs))
	for _, m := range msgs {
		b.WriteString("\n---\n\n")
		b.WriteString(renderMessageBody(m, "##"))
	}
	return b.String()
}

// renderMessage renders one message under a top-level heading.
func renderMessage(m mail.Message) string { return renderMessageBody(m, "#") }

func renderMessageBody(m mail.Message, h string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n\n", h, oneLine(m.Subject))
	fmt.Fprintf(&b, "- id: %s\n- thread: %s\n- from: %s\n", m.ID, m.ThreadID, m.From)
	fmt.Fprintf(&b, "- to: %s\n", joinOrNone(m.To))
	if len(m.Cc) > 0 {
		fmt.Fprintf(&b, "- cc: %s\n", strings.Join(m.Cc, ", "))
	}
	if m.InReplyTo != "" {
		fmt.Fprintf(&b, "- in reply to: %s\n", m.InReplyTo)
	}
	fmt.Fprintf(&b, "- sent: %s\n\n%s\n", stamp(m.CreatedAt), strings.TrimSpace(m.Body))
	return b.String()
}

func joinOrNone(s []string) string {
	if len(s) == 0 {
		return "(none)"
	}
	return strings.Join(s, ", ")
}

// renderDigest renders a message list as a digest, newest first as given.
// more reports that the list was truncated.
func renderDigest(title string, items []mail.Header, more bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	if len(items) == 0 {
		b.WriteString("_Nothing here._\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%d message(s), newest first.\n", len(items))
	for _, h := range items {
		fmt.Fprintf(&b, "\n## %s\n\n", oneLine(h.Subject))
		fmt.Fprintf(&b, "- id: %s\n- from: %s\n- to: %s\n- sent: %s\n- thread: %s (%d message(s))\n",
			h.ID, h.From, h.Mailbox, stamp(h.CreatedAt), h.ThreadID, h.ThreadCount)
		if h.Snippet != "" {
			fmt.Fprintf(&b, "\n> %s\n", oneLine(h.Snippet))
		}
	}
	if more {
		b.WriteString("\n_More messages exist; use the `list` tool to page through them._\n")
	}
	return b.String()
}

// renderRoster renders identities with their presence.
func renderRoster(title string, es []mail.RosterEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	if len(es) == 0 {
		b.WriteString("_No identities registered._\n")
		return b.String()
	}
	for _, e := range es {
		fmt.Fprintf(&b, "## %s\n\n- role: %s\n", e.Address, oneLine(e.Role))
		if e.Description != "" {
			fmt.Fprintf(&b, "- description: %s\n", oneLine(e.Description))
		}
		if e.ReportsTo != "" {
			fmt.Fprintf(&b, "- reports to: %s\n", e.ReportsTo)
		}
		if len(e.Subagents) > 0 {
			fmt.Fprintf(&b, "- subagents: %s\n", strings.Join(e.Subagents, ", "))
		}
		if len(e.Online) == 0 {
			b.WriteString("- presence: offline\n")
		}
		for _, o := range e.Online {
			state := o.State
			if state == "" {
				state = "online"
			}
			fmt.Fprintf(&b, "- presence: %s on %s", state, o.Machine)
			if o.Away != "" {
				fmt.Fprintf(&b, " (away: %s)", oneLine(o.Away))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// whoamiInfo is the data behind renderWhoami.
type whoamiInfo struct {
	SessionID, Machine, ProjectDir, GitRemote, Namespace string
	SignedOnAs, Client, ClientVersion                    string
}

// renderWhoami renders this connection's identity.
func renderWhoami(w whoamiInfo) string {
	var b strings.Builder
	b.WriteString("# This session\n\n")
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "- %s: %s\n", k, v)
		}
	}
	row("session", w.SessionID)
	row("machine", w.Machine)
	row("project dir", w.ProjectDir)
	row("git remote", w.GitRemote)
	row("namespace", w.Namespace)
	row("client", strings.TrimSpace(w.Client+" "+w.ClientVersion))
	if w.SignedOnAs == "" {
		b.WriteString("- signed on as: nobody (call `sign_on` with a registered identity; `register` one first if needed)\n")
	} else {
		row("signed on as", w.SignedOnAs)
	}
	return b.String()
}
