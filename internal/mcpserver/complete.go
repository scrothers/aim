package mcpserver

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
)

// maxCompletions is the protocol's cap on values per completion response.
const maxCompletions = 100

// completeKind is what a completable argument holds.
type completeKind int

const (
	completeNone completeKind = iota
	completeIdentity
	completeAddress
	completeRosterAddress
	completeMessageID
	completeThreadID
)

// promptCompletion returns the kind of value a prompt argument holds.
func promptCompletion(prompt, arg string) completeKind {
	switch prompt + "." + arg {
	case "handoff.to":
		return completeIdentity
	case "reply.message_id":
		return completeMessageID
	case "escalate.thread_id":
		return completeThreadID
	case "standup.address":
		return completeAddress
	}
	return completeNone
}

// templateCompletion returns the kind of value a resource template variable
// holds.
func templateCompletion(uriTemplate, variable string) completeKind {
	switch uriTemplate {
	case prefixThread + "{thread_id}":
		if variable == "thread_id" {
			return completeThreadID
		}
	case prefixMessage + "{message_id}":
		if variable == "message_id" {
			return completeMessageID
		}
	case prefixMailbox + "{address}":
		if variable == "address" {
			return completeAddress
		}
	case prefixRosterOf + "{address}":
		if variable == "address" {
			return completeRosterAddress
		}
	}
	return completeNone
}

// recentLimit bounds the mail scanned for id completions.
const recentLimit = mail.MaxListLimit

// complete answers completion/complete for prompt arguments and resource
// template variables. Unknown refs and arguments yield an empty result.
func (h *handlers) complete(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	p := req.Params
	kind := completeNone
	encode := false // resource template values must be percent-encoded
	switch {
	case p.Ref == nil:
	case p.Ref.Type == "ref/prompt":
		kind = promptCompletion(p.Ref.Name, p.Argument.Name)
	case p.Ref.Type == "ref/resource":
		kind = templateCompletion(p.Ref.URI, p.Argument.Name)
		encode = true
	}
	var candidates []string
	var err error
	switch kind {
	case completeIdentity:
		candidates, err = h.identityCandidates(ctx, true)
	case completeAddress:
		candidates, err = h.addressCandidates(ctx, true)
	case completeRosterAddress:
		candidates, err = h.addressCandidates(ctx, false)
	case completeMessageID:
		candidates, err = h.recentIDs(ctx, func(m mail.Header) string { return m.ID })
	case completeThreadID:
		candidates, err = h.recentIDs(ctx, func(m mail.Header) string { return m.ThreadID })
	case completeNone:
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
				c = encodeSegment(c)
			}
			values = append(values, c)
		}
	}
	return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: values, Total: total, HasMore: total > len(values)}}
}

// scopeAddress is the address whose roster feeds suggestions: the caller's org
// when its namespace is known, otherwise everything.
func (h *handlers) scopeAddress() string {
	parts := strings.Split(h.conn.Preamble().Namespace, "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[:2], "/")
}

// rosterEntries lists identities in scope, online ones first.
func (h *handlers) rosterEntries(ctx context.Context) ([]mail.RosterEntry, error) {
	es, err := h.svc.Roster(ctx, h.scopeAddress())
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(es, func(a, b mail.RosterEntry) int {
		return onlineRank(a) - onlineRank(b)
	})
	return es, nil
}

func onlineRank(e mail.RosterEntry) int {
	if len(e.Online) > 0 {
		return 0
	}
	return 1
}

// identityCandidates suggests identity addresses, optionally with operator.
func (h *handlers) identityCandidates(ctx context.Context, withOperator bool) ([]string, error) {
	es, err := h.rosterEntries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(es)+1)
	for _, e := range es {
		out = append(out, e.Address)
	}
	if withOperator {
		out = append(out, address.Operator().String())
	}
	return out, nil
}

// addressCandidates suggests scopes (the caller's namespace, orgs, repos) and
// identities for address arguments.
func (h *handlers) addressCandidates(ctx context.Context, withOperator bool) ([]string, error) {
	ids, err := h.identityCandidates(ctx, withOperator)
	if err != nil {
		return nil, err
	}
	var scopes []string
	add := func(s string) {
		if s != "" && !slices.Contains(scopes, s) {
			scopes = append(scopes, s)
		}
	}
	add(h.conn.Preamble().Namespace)
	add(h.scopeAddress())
	for _, id := range ids {
		if _, scope, ok := strings.Cut(id, "@"); ok {
			add(scope)
			if i := strings.LastIndex(scope, "/"); i > strings.Index(scope, "/") {
				add(scope[:i])
			}
		}
	}
	return append(scopes, ids...), nil
}

// recentIDs extracts values from the caller's recent non-archived mail (their
// identity when signed on, else their namespace), newest first, deduplicated.
func (h *handlers) recentIDs(ctx context.Context, pick func(mail.Header) string) ([]string, error) {
	addr := h.conn.Preamble().Namespace
	if id, ok := h.conn.Identity(); ok {
		addr = id.String()
	}
	if addr == "" {
		return nil, nil
	}
	res, err := h.svc.List(ctx, mail.ListInput{Address: addr, State: mail.StateInbox, Limit: recentLimit})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range res.Items {
		if v := pick(m); !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out, nil
}
