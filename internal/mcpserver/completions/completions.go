package completions

import (
	"context"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/resources"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// maxCompletions is the protocol's cap on values per completion response.
const maxCompletions = 100

// kind is what a completable argument holds.
type kind int

const (
	kindNone kind = iota
	kindIdentity
	kindAddress
	kindRosterAddress
	kindMessageID
	kindThreadID
	kindTaskID
)

// promptKind returns the kind of value a prompt argument holds.
func promptKind(prompt, arg string) kind {
	switch prompt + "." + arg {
	case "handoff.to":
		return kindIdentity
	case "reply.message_id":
		return kindMessageID
	case "escalate.thread_id":
		return kindThreadID
	case "standup.address":
		return kindAddress
	}
	return kindNone
}

// templateKind returns the kind of value a resource template variable holds.
func templateKind(uriTemplate, variable string) kind {
	switch uriTemplate {
	case resources.PrefixThread + "{thread_id}":
		if variable == "thread_id" {
			return kindThreadID
		}
	case resources.PrefixMessage + "{message_id}":
		if variable == "message_id" {
			return kindMessageID
		}
	case resources.PrefixMailbox + "{address}":
		if variable == "address" {
			return kindAddress
		}
	case resources.PrefixRosterOf + "{address}":
		if variable == "address" {
			return kindRosterAddress
		}
	case resources.PrefixTask + "{task_id}":
		if variable == "task_id" {
			return kindTaskID
		}
	}
	return kindNone
}

// Handler returns the completion/complete handler for c: it completes prompt
// arguments and resource template variables. Unknown refs and arguments yield
// an empty result.
func Handler(c *session.Conn) func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	cp := completer{c: c}
	return cp.complete
}

// completer looks up candidates for one connection.
type completer struct{ c *session.Conn }

func (cp completer) complete(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	p := req.Params
	k := kindNone
	encode := false // resource template values must be percent-encoded
	switch {
	case p.Ref == nil:
	case p.Ref.Type == "ref/prompt":
		k = promptKind(p.Ref.Name, p.Argument.Name)
	case p.Ref.Type == "ref/resource":
		k = templateKind(p.Ref.URI, p.Argument.Name)
		encode = true
	}
	var candidates []string
	var err error
	switch k {
	case kindIdentity:
		candidates, err = cp.identities(ctx, true)
	case kindAddress:
		candidates, err = cp.addresses(ctx, true)
	case kindRosterAddress:
		candidates, err = cp.addresses(ctx, false)
	case kindMessageID:
		candidates, err = cp.recentIDs(ctx, func(m mail.Header) string { return m.ID })
	case kindThreadID:
		candidates, err = cp.recentIDs(ctx, func(m mail.Header) string { return m.ThreadID })
	case kindTaskID:
		candidates, err = cp.taskIDs(ctx)
	case kindNone:
	}
	if err != nil {
		// Suggestions are best effort; a failing lookup must not break typing.
		candidates = nil
	}
	return completion(candidates, p.Argument.Value, encode), nil
}

// completion filters candidates by prefix (case-insensitive, against the
// decoded typed value), caps them and optionally percent-encodes them.
func completion(candidates []string, typed string, encode bool) *mcp.CompleteResult {
	if v, err := url.PathUnescape(typed); err == nil {
		typed = v
	}
	typed = strings.ToLower(typed)
	values := []string{}
	total := 0
	for _, c := range candidates {
		if !strings.HasPrefix(strings.ToLower(c), typed) {
			continue
		}
		total++
		if len(values) < maxCompletions {
			if encode {
				c = resources.EncodeSegment(c)
			}
			values = append(values, c)
		}
	}
	return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: values, Total: total, HasMore: total > len(values)}}
}
