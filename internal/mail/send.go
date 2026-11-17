package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/store/db"
)

const (
	kindTo = "to"
	kindCc = "cc"
)

type recipient struct {
	addr address.Address
	kind string
}

func validateContent(op, subject, body string) error {
	n := utf8.RuneCountInString(subject)
	if strings.TrimSpace(subject) == "" || n > MaxSubjectRunes {
		return invalidf(op, "subject must be 1-%d characters (got %d)", MaxSubjectRunes, n)
	}
	if strings.TrimSpace(body) == "" || len(body) > MaxBodyBytes {
		return invalidf(op, "body must be 1-%d bytes of Markdown (got %d)", MaxBodyBytes, len(body))
	}
	if !utf8.ValidString(subject) || !utf8.ValidString(body) {
		return invalidf(op, "subject and body must be valid UTF-8")
	}
	return nil
}

// dedupe merges the recipient lists preserving order. The caller's own to
// and cc win over the expanded ones (an address the caller put in cc is never
// promoted to to), and within each pair an address in both stays in to.
func dedupe(to, cc, expTo, expCc []address.Address) []recipient {
	seen := make(map[string]bool, len(to)+len(cc)+len(expTo)+len(expCc))
	out := make([]recipient, 0, len(to)+len(cc)+len(expTo)+len(expCc))
	add := func(list []address.Address, kind string) {
		for _, a := range list {
			if k := a.String(); !seen[k] {
				seen[k] = true
				out = append(out, recipient{a, kind})
			}
		}
	}
	add(to, kindTo)
	add(cc, kindCc)
	add(expTo, kindTo)
	add(expCc, kindCc)
	return out
}

func rowAddr(host, org, repo, identity, subagent string) address.Address {
	return address.Address{Host: host, Org: org, Repo: repo, Identity: identity, Subagent: subagent}
}

// Send delivers a message from the caller's identity (or one of its
// subagents) to the to and cc recipients.
func (s *Service) Send(ctx context.Context, c Caller, in SendInput) (Message, error) {
	const op = "send"
	if err := requireSignedOn(op, c); err != nil {
		return Message{}, err
	}
	if err := validateContent(op, in.Subject, in.Body); err != nil {
		return Message{}, err
	}
	if len(in.To) == 0 && !in.ReplyAll {
		return Message{}, invalidf(op, "to must list at least one recipient address (or set reply_all with in_reply_to)")
	}
	if in.ReplyAll && in.InReplyTo == "" {
		return Message{}, invalidf(op, "reply_all requires in_reply_to (the message id you are replying to)")
	}
	to, err := parseAll(op, in.To)
	if err != nil {
		return Message{}, err
	}
	cc, err := parseAll(op, in.Cc)
	if err != nil {
		return Message{}, err
	}
	from := c.Identity.IdentityAddress()
	if in.Subagent != "" {
		if from, err = from.WithSubagent(in.Subagent); err != nil {
			return Message{}, invalidAddr(op, err)
		}
	}

	var out Message
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		// The clock and the id are read inside the transaction: the writer is
		// serialized, so created_at and ids rise in commit order and a
		// since-poller can never miss a later-committed message.
		out, err = s.send(ctx, q, c, in, from, to, cc, s.now())
		return err
	})
	if err != nil {
		return Message{}, err
	}
	return out, nil
}

func (s *Service) send(ctx context.Context, q *db.Queries, c Caller, in SendInput,
	from address.Address, to, cc []address.Address, now time.Time) (Message, error) {
	const op = "send"
	sender, err := s.ensureMailbox(ctx, q, from, now)
	if err != nil {
		return Message{}, fmt.Errorf("mail: %s: sender: %w", op, err)
	}
	msgID, err := s.newID(prefixMessage, now)
	if err != nil {
		return Message{}, err
	}
	threadID := msgID
	var expTo, expCc []address.Address
	if in.InReplyTo != "" {
		parent, err := q.GetMessageByID(ctx, in.InReplyTo)
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, fmt.Errorf("mail: %s: %w: in_reply_to %q does not exist; use an id from list or read", op, ErrMessageNotFound, in.InReplyTo)
		}
		if err != nil {
			return Message{}, fmt.Errorf("mail: %s: parent: %w", op, err)
		}
		threadID = parent.ThreadID
		if in.ReplyAll {
			expTo, expCc, err = s.expandReplyAll(ctx, q, parent, from, nil, nil)
			if err != nil {
				return Message{}, err
			}
		}
	}
	recips := dedupe(to, cc, expTo, expCc)
	if len(recips) == 0 {
		return Message{}, invalidf(op, "reply_all left no recipients (you were the only other party); add explicit to addresses")
	}
	ids := make([]string, len(recips))
	for i, r := range recips {
		m, err := s.ensureMailbox(ctx, q, r.addr, now)
		if err != nil {
			return Message{}, fmt.Errorf("mail: %s: recipient: %w", op, err)
		}
		ids[i] = m.ID
	}
	err = q.InsertMessage(ctx, db.InsertMessageParams{
		ID: msgID, ThreadID: threadID, InReplyTo: sql.NullString{String: in.InReplyTo, Valid: in.InReplyTo != ""},
		FromMailbox: sender.ID, FromSession: c.SessionID, Subject: in.Subject, Body: in.Body, CreatedAt: now.UnixMilli(),
	})
	if err != nil {
		return Message{}, fmt.Errorf("mail: %s: insert message: %w", op, err)
	}
	msg := Message{
		ID: msgID, ThreadID: threadID, InReplyTo: in.InReplyTo, From: from.String(),
		Subject: in.Subject, Body: in.Body, CreatedAt: fromMS(now.UnixMilli()),
		To: []string{}, Cc: []string{},
	}
	for i, r := range recips {
		if err := q.InsertDelivery(ctx, db.InsertDeliveryParams{MessageID: msgID, MailboxID: ids[i], Kind: r.kind}); err != nil {
			return Message{}, fmt.Errorf("mail: %s: insert delivery: %w", op, err)
		}
		if r.kind == kindTo {
			msg.To = append(msg.To, r.addr.String())
		} else {
			msg.Cc = append(msg.Cc, r.addr.String())
		}
	}
	return msg, nil
}

// expandReplyAll appends the parent's sender and recipients to to and cc,
// minus the sender's own addresses (the sending mailbox and its identity).
func (s *Service) expandReplyAll(ctx context.Context, q *db.Queries, parent db.GetMessageByIDRow,
	self address.Address, to, cc []address.Address) ([]address.Address, []address.Address, error) {
	rows, err := q.ListDeliveriesForMessage(ctx, parent.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("mail: send: parent recipients: %w", err)
	}
	notSelf := func(a address.Address) bool { return a != self && a != self.IdentityAddress() }
	if a := rowAddr(parent.FromHost, parent.FromOrg, parent.FromRepo, parent.FromIdentity, parent.FromSubagent); notSelf(a) {
		to = append(to, a)
	}
	for _, r := range rows {
		a := rowAddr(r.Host, r.Org, r.Repo, r.Identity, r.Subagent)
		if !notSelf(a) {
			continue
		}
		if r.Kind == kindTo {
			to = append(to, a)
		} else {
			cc = append(cc, a)
		}
	}
	return to, cc, nil
}
