package mail_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/mail"
)

func TestInboxStateExcludesArchived(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	read := e.send(t, bobWeb, []string{aliceWeb}, "read one")
	archived := e.send(t, bobWeb, []string{aliceWeb}, "archived one")
	fresh := e.send(t, bobWeb, []string{aliceWeb}, "fresh one")
	if _, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{MessageID: read.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Mark(ctx, caller(aliceWeb), mail.MarkInput{MessageIDs: []string{archived.ID}, State: mail.MarkArchived}); err != nil {
		t.Fatal(err)
	}
	got := ids(e.list(t, mail.ListInput{Address: aliceWeb, State: mail.StateInbox}).Items)
	if want := []string{fresh.ID, read.ID}; !slices.Equal(got, want) {
		t.Errorf("inbox = %v, want %v", got, want)
	}
}

func TestTodayCountsDeliveries(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, carolAPI)
	e.send(t, aliceWeb, []string{bobWeb, carolAPI}, "fan out")
	v, err := e.svc.Views(context.Background(), epoch)
	if err != nil {
		t.Fatal(err)
	}
	if v.Today != 2 {
		t.Errorf("Today = %d, want 2 deliveries for one message", v.Today)
	}
}

func TestReplyAllSkipsRetired(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb, carolAPI)
	root, err := e.svc.Send(ctx, caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Cc: []string{carolAPI}, Subject: "q", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Retire(ctx, carolAPI); err != nil {
		t.Fatal(err)
	}
	reply, err := e.svc.Send(ctx, caller(bobWeb), mail.SendInput{InReplyTo: root.ID, ReplyAll: true, Subject: "Re: q", Body: "r"})
	if err != nil {
		t.Fatalf("reply_all with a retired participant: %v", err)
	}
	if !slices.Equal(reply.To, []string{aliceWeb}) || len(reply.Cc) != 0 {
		t.Errorf("to=%v cc=%v, want only alice", reply.To, reply.Cc)
	}
}

func TestReadReportsMarked(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	m := e.send(t, bobWeb, []string{aliceWeb}, "mark me")
	cases := []struct {
		name string
		c    mail.Caller
		in   mail.ReadInput
		want bool
	}{
		{"unsigned", mail.Caller{}, mail.ReadInput{MessageID: m.ID}, false},
		{"peek", caller(aliceWeb), mail.ReadInput{MessageID: m.ID, Peek: true}, false},
		{"non-owner", caller(bobWeb), mail.ReadInput{MessageID: m.ID}, false},
		{"owner", caller(aliceWeb), mail.ReadInput{MessageID: m.ID}, true},
		{"owner again", caller(aliceWeb), mail.ReadInput{MessageID: m.ID}, true},
	}
	for _, tt := range cases {
		got, err := e.svc.Read(ctx, tt.c, tt.in)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got.Marked != tt.want {
			t.Errorf("%s: Marked = %v, want %v", tt.name, got.Marked, tt.want)
		}
	}
}

func TestListHeadersCarrySnippet(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	if _, err := e.svc.Send(ctx, caller(bobWeb), mail.SendInput{To: []string{aliceWeb}, Subject: "s", Body: "## Plan\n\n| a | b |\n|---|---|\n| **1** | 2 |"}); err != nil {
		t.Fatal(err)
	}
	items := e.list(t, mail.ListInput{Address: aliceWeb, State: mail.StateAll}).Items
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	if got := items[0].Snippet; got != "Plan a b 1 2" || strings.ContainsAny(got, "#|*") {
		t.Errorf("Snippet = %q", got)
	}
}
