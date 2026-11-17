package mail_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store/db"
)

func ptr(s string) *string { return &s }

func operatorCaller() mail.Caller { return mail.OperatorCaller("ui-1", "aim-web", "devbox") }

func TestOperatorMailbox(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, aliceWeb)
	for range 2 { // idempotent
		if err := e.svc.EnsureOperator(ctx); err != nil {
			t.Fatal(err)
		}
	}

	m := e.send(t, aliceWeb, []string{"operator"}, "need approval")
	if len(m.To) != 1 || m.To[0] != "operator" {
		t.Fatalf("To = %v", m.To)
	}
	got := e.list(t, mail.ListInput{Address: "operator", State: mail.StateAll})
	if len(got.Items) != 1 || got.Items[0].Mailbox != "operator" || got.Items[0].From != aliceWeb {
		t.Fatalf("operator inbox = %+v", got.Items)
	}

	// The operator reads (marks read) and replies; the agent receives it.
	if _, err := e.svc.Read(ctx, operatorCaller(), mail.ReadInput{MessageID: m.ID}); err != nil {
		t.Fatal(err)
	}
	if n := len(e.list(t, mail.ListInput{Address: "operator"}).Items); n != 0 {
		t.Errorf("unread operator mail after read = %d", n)
	}
	reply, err := e.svc.Send(ctx, operatorCaller(), mail.SendInput{InReplyTo: m.ID, ReplyAll: true, Subject: "Re: need approval", Body: "approved"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.From != "operator" || reply.FromClient != "aim-web" || reply.FromMachine != "devbox" || reply.ThreadID != m.ThreadID {
		t.Errorf("reply = %+v", reply)
	}
	if !slices.Equal(reply.To, []string{aliceWeb}) {
		t.Errorf("reply.To = %v", reply.To)
	}
	if n := len(e.list(t, mail.ListInput{Address: aliceWeb}).Items); n != 1 {
		t.Errorf("alice unread = %d, want 1", n)
	}
	got2, err := e.svc.Read(ctx, mail.Caller{}, mail.ReadInput{MessageID: reply.ID, Peek: true})
	if err != nil || got2.FromClient != "aim-web" {
		t.Errorf("read = %+v, %v", got2, err)
	}
	th, err := e.svc.Thread(ctx, m.ThreadID)
	if err != nil || len(th) != 2 || th[1].FromMachine != "devbox" {
		t.Errorf("thread = %+v, %v", th, err)
	}

	if _, err := e.svc.Send(ctx, operatorCaller(), mail.SendInput{To: []string{aliceWeb}, Subject: "s", Body: "b", Subagent: "x"}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("operator subagent err = %v", err)
	}
	// A caller that merely claims the operator address is not the operator.
	fake := mail.Caller{Identity: address.Operator(), SignedOn: true}
	if _, err := e.svc.Send(ctx, fake, mail.SendInput{To: []string{aliceWeb}, Subject: "s", Body: "b"}); !errors.Is(err, mail.ErrNotSignedOn) {
		t.Errorf("fake operator err = %v", err)
	}
}

func TestOperatorReserved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	if _, err := e.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: "operator", Role: "r"}); !errors.Is(err, mail.ErrReserved) {
		t.Errorf("register err = %v", err)
	}
	_, err := e.svc.ResolveIdentity(ctx, "operator")
	if !errors.Is(err, mail.ErrReserved) || !strings.Contains(err.Error(), "agents escalate by sending to operator") {
		t.Errorf("resolve err = %v", err)
	}
	if _, err := e.svc.Retire(ctx, "operator"); !errors.Is(err, mail.ErrReserved) {
		t.Errorf("retire err = %v", err)
	}
	if _, err := e.svc.RosterWith(ctx, "operator", mail.RosterOptions{}); !errors.Is(err, mail.ErrReserved) {
		t.Errorf("roster err = %v", err)
	}
	if _, err := e.svc.UpdateIdentity(ctx, orgNS, mail.UpdateInput{}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("update namespace err = %v", err)
	}
}

func TestReportsToCycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	reg := func(a, reports string) error {
		_, err := e.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: a, Role: "dev", ReportsTo: reports})
		return err
	}
	if err := reg(aliceWeb, aliceWeb); !errors.Is(err, mail.ErrCycle) || !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("self-loop err = %v", err)
	}
	if err := reg(aliceWeb, "operator"); err != nil {
		t.Fatal(err)
	}
	if err := reg(bobWeb, aliceWeb); err != nil {
		t.Fatal(err)
	}
	if err := reg(carolAPI, bobWeb); err != nil {
		t.Fatal(err)
	}
	// alice -> carol would close alice -> carol -> bob -> alice.
	_, err := e.svc.UpdateIdentity(ctx, aliceWeb, mail.UpdateInput{ReportsTo: ptr(carolAPI)})
	if !errors.Is(err, mail.ErrCycle) {
		t.Errorf("cycle err = %v", err)
	}
	// Forward reference closes a loop on register.
	e2 := newEnv(t)
	if _, err := e2.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: bobWeb, Role: "r", ReportsTo: aliceWeb}); err != nil {
		t.Fatal(err)
	}
	if _, err := e2.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: aliceWeb, Role: "r", ReportsTo: bobWeb}); !errors.Is(err, mail.ErrCycle) {
		t.Errorf("forward cycle err = %v", err)
	}
	// A chain deeper than the limit is rejected rather than walked forever.
	e3 := newEnv(t)
	prev := ""
	err = e3.st.WithTx(ctx, func(q *db.Queries) error {
		for i := range 70 {
			name := fmt.Sprintf("n%d", i)
			err := q.InsertMailbox(ctx, db.InsertMailboxParams{
				ID: "mbx-" + name, Host: "github.com", Org: "acme", Repo: "deep", Identity: name,
				Kind: "identity", Role: "r", ReportsTo: prev, CreatedAt: 1,
			})
			if err != nil {
				return err
			}
			prev = name + "@github.com/acme/deep"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e3.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: "top@github.com/acme/deep", Role: "r", ReportsTo: prev})
	if !errors.Is(err, mail.ErrCycle) {
		t.Errorf("deep chain err = %v", err)
	}
}

func TestUpdateIdentity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, aliceWeb)
	got, err := e.svc.UpdateIdentity(ctx, aliceWeb, mail.UpdateInput{Role: ptr(" lead "), ReportsTo: ptr("operator")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != "lead" || got.Description != "d" || got.ReportsTo != "operator" {
		t.Errorf("update = %+v", got)
	}
	got, err = e.svc.UpdateIdentity(ctx, aliceWeb, mail.UpdateInput{Description: ptr(""), ReportsTo: ptr("")})
	if err != nil || got.Description != "" || got.ReportsTo != "" || got.Role != "lead" {
		t.Errorf("clear = %+v, %v", got, err)
	}
	bad := []mail.UpdateInput{
		{Role: ptr("")},
		{Role: ptr(strings.Repeat("r", 101))},
		{Description: ptr(strings.Repeat("d", 1001))},
		{ReportsTo: ptr("not an address")},
	}
	for _, in := range bad {
		if _, err := e.svc.UpdateIdentity(ctx, aliceWeb, in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("update %+v err = %v", in, err)
		}
	}
	if _, err := e.svc.UpdateIdentity(ctx, bobWeb, mail.UpdateInput{}); !errors.Is(err, mail.ErrMailboxNotFound) {
		t.Errorf("missing err = %v", err)
	}
	if _, err := e.svc.UpdateIdentity(ctx, "", mail.UpdateInput{}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("empty addr err = %v", err)
	}
}

func TestRetireSemantics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	e.send(t, aliceWeb, []string{bobWeb}, "before")

	r, err := e.svc.Retire(ctx, bobWeb)
	if err != nil || r.RetiredAt == nil {
		t.Fatalf("retire = %+v, %v", r, err)
	}
	again, err := e.svc.Retire(ctx, bobWeb)
	if err != nil || again.RetiredAt == nil || !again.RetiredAt.Equal(*r.RetiredAt) {
		t.Errorf("second retire = %+v, %v", again, err)
	}

	if _, err := e.svc.ResolveIdentity(ctx, bobWeb); !errors.Is(err, mail.ErrRetired) {
		t.Errorf("resolve err = %v", err)
	}
	_, err = e.svc.Send(ctx, caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Subject: "s", Body: "b"})
	if !errors.Is(err, mail.ErrRetired) {
		t.Errorf("send to retired err = %v", err)
	}
	_, err = e.svc.Send(ctx, caller(bobWeb), mail.SendInput{To: []string{aliceWeb}, Subject: "s", Body: "b"})
	if !errors.Is(err, mail.ErrRetired) {
		t.Errorf("send from retired err = %v", err)
	}

	ros, err := e.svc.Roster(ctx, orgNS)
	if err != nil || len(ros) != 1 || ros[0].Address != aliceWeb {
		t.Errorf("roster = %+v, %v", ros, err)
	}
	ros, err = e.svc.RosterWith(ctx, orgNS, mail.RosterOptions{IncludeRetired: true})
	if err != nil || len(ros) != 2 || ros[1].RetiredAt == nil {
		t.Errorf("roster incl retired = %+v, %v", ros, err)
	}
	ros, err = e.svc.RosterWith(ctx, "", mail.RosterOptions{})
	if err != nil || len(ros) != 1 {
		t.Errorf("global roster = %+v, %v", ros, err)
	}
	ros, err = e.svc.RosterWith(ctx, "", mail.RosterOptions{IncludeRetired: true})
	if err != nil || len(ros) != 2 {
		t.Errorf("global roster incl retired = %+v, %v", ros, err)
	}
	// Existing mail stays listable; the retired identity can still be edited.
	if n := len(e.list(t, mail.ListInput{Address: bobWeb, State: mail.StateAll}).Items); n != 1 {
		t.Errorf("retired inbox = %d", n)
	}
	if _, err := e.svc.UpdateIdentity(ctx, bobWeb, mail.UpdateInput{Role: ptr("x")}); err != nil {
		t.Errorf("update retired: %v", err)
	}

	u, err := e.svc.Unretire(ctx, bobWeb)
	if err != nil || u.RetiredAt != nil {
		t.Fatalf("unretire = %+v, %v", u, err)
	}
	if u, err = e.svc.Unretire(ctx, bobWeb); err != nil || u.RetiredAt != nil {
		t.Errorf("second unretire = %+v, %v", u, err)
	}
	if _, err := e.svc.ResolveIdentity(ctx, bobWeb); err != nil {
		t.Errorf("resolve after unretire: %v", err)
	}
	if _, err := e.svc.Retire(ctx, carolAPI); !errors.Is(err, mail.ErrMailboxNotFound) {
		t.Errorf("retire missing err = %v", err)
	}
	if _, err := e.svc.Retire(ctx, ""); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("retire empty err = %v", err)
	}
}

func TestGlobalListAndSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, otherOrgA)
	e.send(t, aliceWeb, []string{bobWeb}, "findme one")
	e.send(t, bobWeb, []string{otherOrgA, "operator"}, "findme two")

	all := e.list(t, mail.ListInput{State: mail.StateAll})
	if len(all.Items) != 3 {
		t.Fatalf("global list = %d, want 3", len(all.Items))
	}
	page := e.list(t, mail.ListInput{State: mail.StateAll, Limit: 2})
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("page 1 = %+v", page)
	}
	page2 := e.list(t, mail.ListInput{State: mail.StateAll, Limit: 2, Cursor: page.NextCursor})
	if len(page2.Items) != 1 || page2.Items[0].ID != all.Items[2].ID {
		t.Errorf("page 2 = %+v", page2)
	}
	res, err := e.svc.Search(ctx, mail.SearchInput{Query: "findme"})
	if err != nil || len(res.Hits) != 2 {
		t.Errorf("global search = %+v, %v", res, err)
	}
	res, err = e.svc.Search(ctx, mail.SearchInput{Address: "operator", Query: "findme"})
	if err != nil || len(res.Hits) != 1 || res.Hits[0].Mailbox != "operator" {
		t.Errorf("operator search = %+v, %v", res, err)
	}
}

func TestPresenceClientInRoster(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb)
	e.pres[aliceWeb] = []mail.OnlineStatus{{SessionID: "s", Machine: "m", Client: "codex"}}
	ros, err := e.svc.Roster(context.Background(), aliceWeb)
	if err != nil || len(ros[0].Online) != 1 || ros[0].Online[0].Client != "codex" {
		t.Errorf("roster = %+v, %v", ros, err)
	}
}
