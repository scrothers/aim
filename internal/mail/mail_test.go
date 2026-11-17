package mail_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
)

const (
	aliceOrg  = "alice@github.com/acme"
	aliceWeb  = "alice@github.com/acme/web"
	bobWeb    = "bob@github.com/acme/web"
	carolAPI  = "carol@github.com/acme/api"
	orgNS     = "github.com/acme"
	webNS     = "github.com/acme/web"
	otherOrgA = "dave@github.com/other"
)

var templatePath string

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "aim-mail-template-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	templatePath = filepath.Join(dir, "template.db")
	s, err := store.Open(context.Background(), templatePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := s.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

// counterReader yields deterministic, non-repeating bytes.
type counterReader struct{ n atomic.Uint64 }

func (c *counterReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(c.n.Add(1))
	}
	return len(p), nil
}

type fakePresence map[string][]mail.OnlineStatus

func (f fakePresence) Online(a address.Address) []mail.OnlineStatus { return f[a.String()] }

type env struct {
	svc  *mail.Service
	st   *store.Store
	pres fakePresence
	tick atomic.Int64
}

var epoch = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

func newEnv(t *testing.T) *env {
	t.Helper()
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "aim.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &env{st: st, pres: fakePresence{}}
	clock := func() time.Time { return epoch.Add(time.Duration(e.tick.Add(1)) * time.Second) }
	e.svc = mail.New(st, e.pres, mail.WithClock(clock), mail.WithEntropy(&counterReader{}))
	return e
}

func (e *env) register(t *testing.T, addrs ...string) {
	t.Helper()
	for _, a := range addrs {
		if _, err := e.svc.Register(context.Background(), mail.Caller{}, mail.RegisterInput{Address: a, Role: "dev", Description: "d"}); err != nil {
			t.Fatalf("register %s: %v", a, err)
		}
	}
}

func caller(a string) mail.Caller {
	return mail.Caller{SessionID: "sess-" + a, Identity: address.MustParse(a), SignedOn: true}
}

func (e *env) send(t *testing.T, from string, to []string, subject string) mail.Message {
	t.Helper()
	m, err := e.svc.Send(context.Background(), caller(from), mail.SendInput{To: to, Subject: subject, Body: "body of " + subject})
	if err != nil {
		t.Fatalf("send %q: %v", subject, err)
	}
	return m
}

func (e *env) list(t *testing.T, in mail.ListInput) mail.ListResult {
	t.Helper()
	r, err := e.svc.List(context.Background(), in)
	if err != nil {
		t.Fatalf("list %+v: %v", in, err)
	}
	return r
}

func ids(hs []mail.Header) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.ID
	}
	return out
}

func TestRegister(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	id, err := e.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: " Alice@GitHub.com/acme/web ", Role: "lead", Description: "d", ReportsTo: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if id.Address != aliceWeb || id.ReportsTo != "operator" || id.Role != "lead" {
		t.Errorf("identity = %+v", id)
	}
	if _, err := e.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: aliceWeb, Role: "x"}); !errors.Is(err, mail.ErrExists) {
		t.Errorf("duplicate err = %v, want ErrExists", err)
	}
	// Org-level alice is a different identity from repo-level alice.
	r, err := e.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: aliceOrg, Role: "x", ReportsTo: aliceWeb})
	if err != nil || r.ReportsTo != aliceWeb {
		t.Errorf("org alice = %+v, %v", r, err)
	}
	long := strings.Repeat("x", 1001)
	for name, in := range map[string]mail.RegisterInput{
		"namespace":   {Address: orgNS, Role: "x"},
		"subagent":    {Address: "h@" + aliceWeb, Role: "x"},
		"bad address": {Address: "nope", Role: "x"},
		"no role":     {Address: bobWeb},
		"long desc":   {Address: bobWeb, Role: "x", Description: long},
		"bad reports": {Address: bobWeb, Role: "x", ReportsTo: "not an address"},
	} {
		if _, err := e.svc.Register(ctx, mail.Caller{}, in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	_, err = e.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: "bad", Role: "x"})
	if !errors.Is(err, address.ErrInvalid) {
		t.Errorf("bad address should wrap address.ErrInvalid: %v", err)
	}
}

func TestResolveIdentityAndSuggestions(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, carolAPI, otherOrgA, "alicia@github.com/acme/web")
	ctx := context.Background()
	a, err := e.svc.ResolveIdentity(ctx, "ALICE@github.com/acme/web")
	if err != nil || a.String() != aliceWeb {
		t.Fatalf("resolve = %v, %v", a, err)
	}
	_, err = e.svc.ResolveIdentity(ctx, "alise@github.com/acme/web")
	if !errors.Is(err, mail.ErrMailboxNotFound) {
		t.Fatalf("err = %v", err)
	}
	var nf *mail.MailboxNotFoundError
	if !errors.As(err, &nf) {
		t.Fatal("no MailboxNotFoundError")
	}
	if !strings.Contains(err.Error(), "mailbox alise@github.com/acme/web does not exist") || !strings.Contains(err.Error(), "did you mean") {
		t.Errorf("message = %q", err)
	}
	if len(nf.Suggestions) == 0 || nf.Suggestions[0] != aliceWeb {
		t.Errorf("suggestions = %v", nf.Suggestions)
	}
	for _, s := range nf.Suggestions {
		if strings.Contains(s, "other") {
			t.Errorf("suggestion from another org: %s", s)
		}
	}
	// Far-away name: no suggestions, still not found.
	_, err = e.svc.ResolveIdentity(ctx, "zzzzzzzzzzzzzzzz@github.com/acme/web")
	if !errors.As(err, &nf) || len(nf.Suggestions) != 0 {
		t.Errorf("expected no suggestions: %v", err)
	}
	for _, in := range []string{orgNS, "h@" + aliceWeb, "bad"} {
		if _, err := e.svc.ResolveIdentity(ctx, in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("resolve %q err = %v", in, err)
		}
	}
}

func TestSuggestionsCapped(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, "bob1@github.com/acme", "bob2@github.com/acme", "bob3@github.com/acme", "bob4@github.com/acme")
	_, err := e.svc.ResolveIdentity(context.Background(), "bob@github.com/acme")
	var nf *mail.MailboxNotFoundError
	if !errors.As(err, &nf) || len(nf.Suggestions) != 3 {
		t.Errorf("suggestions = %v", nf)
	}
}

func TestSendBasics(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, carolAPI)
	ctx := context.Background()
	m, err := e.svc.Send(ctx, caller(aliceWeb), mail.SendInput{
		To: []string{bobWeb, "BOB@github.com/acme/web"}, Cc: []string{bobWeb, carolAPI},
		Subject: "hello", Body: "# hi\n**md**",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(m.ID, "msg-") || m.ThreadID != m.ID || m.From != aliceWeb {
		t.Errorf("message = %+v", m)
	}
	if !slices.Equal(m.To, []string{bobWeb}) || !slices.Equal(m.Cc, []string{carolAPI}) {
		t.Errorf("to=%v cc=%v (to wins over cc, dedupe)", m.To, m.Cc)
	}
	if m.ID != strings.ToLower(m.ID) {
		t.Errorf("id not lowercase: %s", m.ID)
	}
	got, err := e.svc.Read(ctx, mail.Caller{}, mail.ReadInput{MessageID: m.ID, Peek: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "# hi\n**md**" || got.Subject != "hello" || !got.CreatedAt.Equal(m.CreatedAt) || got.From != aliceWeb {
		t.Errorf("stored = %+v", got)
	}
}

func TestSendValidation(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	ctx := context.Background()
	ok := mail.SendInput{To: []string{bobWeb}, Subject: "s", Body: "b"}
	with := func(f func(*mail.SendInput)) mail.SendInput { in := ok; f(&in); return in }
	cases := map[string]mail.SendInput{
		"no to":          with(func(i *mail.SendInput) { i.To = nil }),
		"empty subject":  with(func(i *mail.SendInput) { i.Subject = "" }),
		"long subject":   with(func(i *mail.SendInput) { i.Subject = strings.Repeat("é", 201) }),
		"empty body":     with(func(i *mail.SendInput) { i.Body = "" }),
		"long body":      with(func(i *mail.SendInput) { i.Body = strings.Repeat("x", 65537) }),
		"bad utf8":       with(func(i *mail.SendInput) { i.Body = "\xff\xfe" }),
		"bad to":         with(func(i *mail.SendInput) { i.To = []string{"zzz"} }),
		"bad cc":         with(func(i *mail.SendInput) { i.Cc = []string{"zzz"} }),
		"reply_all solo": with(func(i *mail.SendInput) { i.ReplyAll = true }),
		"bad subagent":   with(func(i *mail.SendInput) { i.Subagent = "Bad Name!" }),
	}
	for name, in := range cases {
		if _, err := e.svc.Send(ctx, caller(aliceWeb), in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	// Boundary values are accepted.
	b := with(func(i *mail.SendInput) { i.Subject = strings.Repeat("é", 200); i.Body = strings.Repeat("x", 65536) })
	if _, err := e.svc.Send(ctx, caller(aliceWeb), b); err != nil {
		t.Errorf("boundary send: %v", err)
	}
	if _, err := e.svc.Send(ctx, mail.Caller{}, ok); !errors.Is(err, mail.ErrNotSignedOn) {
		t.Errorf("anonymous send err = %v", err)
	}
	ns := mail.Caller{Identity: address.MustParse(orgNS), SignedOn: true}
	if _, err := e.svc.Send(ctx, ns, ok); !errors.Is(err, mail.ErrNotSignedOn) {
		t.Errorf("non-identity caller err = %v", err)
	}
	_, err := e.svc.Send(ctx, caller(aliceWeb), with(func(i *mail.SendInput) { i.InReplyTo = "msg-nope" }))
	if !errors.Is(err, mail.ErrMessageNotFound) {
		t.Errorf("bad in_reply_to err = %v", err)
	}
}

func TestSendUnknownRecipientIsAtomic(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	_, err := e.svc.Send(context.Background(), caller(aliceWeb), mail.SendInput{
		To: []string{"newns@github.com/acme/web", "bobb@github.com/acme/web"}, Cc: []string{"github.com/acme/fresh"}, Subject: "s", Body: "b",
	})
	if !errors.Is(err, mail.ErrMailboxNotFound) || !strings.Contains(err.Error(), "newns@github.com/acme/web") {
		t.Fatalf("err = %v", err)
	}
	// Nothing was created or delivered: the fresh namespace mailbox rolled back.
	r := e.list(t, mail.ListInput{Address: orgNS, State: "all"})
	if len(r.Items) != 0 {
		t.Errorf("rolled-back send delivered %d items", len(r.Items))
	}
	if _, err := e.svc.Send(context.Background(), caller(aliceWeb), mail.SendInput{To: []string{"x@" + bobWeb}, Subject: "s", Body: "b"}); err != nil {
		t.Errorf("subagent send to existing parent: %v", err)
	}
	_, err = e.svc.Send(context.Background(), caller(aliceWeb), mail.SendInput{To: []string{"x@ghost@github.com/acme/web"}, Subject: "s", Body: "b"})
	if !errors.Is(err, mail.ErrMailboxNotFound) {
		t.Errorf("subagent of unknown parent err = %v", err)
	}
}

func TestAutoCreateNamespaceAndSubagent(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	m := e.send(t, aliceWeb, []string{webNS, "helper@" + bobWeb}, "broadcast")
	r := e.list(t, mail.ListInput{Address: webNS, State: "all"})
	if len(r.Items) != 2 {
		t.Fatalf("web subtree items = %d, want namespace + bob subagent deliveries", len(r.Items))
	}
	bare := e.list(t, mail.ListInput{Address: webNS, State: "unread"})
	for _, h := range bare.Items {
		if h.Mailbox == webNS && h.ReadAt != nil {
			t.Error("bare namespace delivery has read state")
		}
	}
	// Subagent mailbox now exists, so its owner can read it.
	sub := caller(bobWeb)
	got, err := e.svc.Read(context.Background(), sub, mail.ReadInput{MessageID: m.ID, Subagent: "helper"})
	if err != nil || got.ID != m.ID {
		t.Fatalf("subagent read = %v, %v", got, err)
	}
	r = e.list(t, mail.ListInput{Address: "helper@" + bobWeb, State: "read"})
	if len(r.Items) != 1 || r.Items[0].ReadAt == nil {
		t.Errorf("subagent delivery not marked read: %+v", r.Items)
	}
	// Bob's own delivery (none) is unaffected.
	if r := e.list(t, mail.ListInput{Address: bobWeb, State: "all"}); len(r.Items) != 1 {
		t.Errorf("bob subtree = %d", len(r.Items))
	}
	// Sending as a subagent creates its mailbox and sets From.
	sm, err := e.svc.Send(context.Background(), caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Subject: "s", Body: "b", Subagent: "scout"})
	if err != nil || sm.From != "scout@"+aliceWeb {
		t.Fatalf("subagent send = %+v, %v", sm, err)
	}
}

func TestThreadingAndReplyAll(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, carolAPI)
	ctx := context.Background()
	root, err := e.svc.Send(ctx, caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Cc: []string{carolAPI}, Subject: "q", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := e.svc.Send(ctx, caller(bobWeb), mail.SendInput{InReplyTo: root.ID, ReplyAll: true, Subject: "Re: q", Body: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if reply.ThreadID != root.ThreadID || reply.InReplyTo != root.ID {
		t.Errorf("reply threading = %+v", reply)
	}
	if !slices.Equal(reply.To, []string{aliceWeb}) || !slices.Equal(reply.Cc, []string{carolAPI}) {
		t.Errorf("reply_all to=%v cc=%v (sender's own mailbox excluded)", reply.To, reply.Cc)
	}
	third, err := e.svc.Send(ctx, caller(carolAPI), mail.SendInput{InReplyTo: reply.ID, To: []string{bobWeb}, Subject: "Re: q", Body: "x"})
	if err != nil || third.ThreadID != root.ID {
		t.Fatalf("third = %+v, %v", third, err)
	}
	th, err := e.svc.Thread(ctx, root.ThreadID)
	if err != nil || len(th) != 3 || th[0].ID != root.ID || th[2].ID != third.ID {
		t.Fatalf("thread = %v, %v", th, err)
	}
	if !slices.Equal(th[0].To, []string{bobWeb}) || !slices.Equal(th[0].Cc, []string{carolAPI}) {
		t.Errorf("thread recipients = %v %v", th[0].To, th[0].Cc)
	}
	if _, err := e.svc.Thread(ctx, "msg-nothere"); !errors.Is(err, mail.ErrMessageNotFound) {
		t.Errorf("unknown thread err = %v", err)
	}
	if _, err := e.svc.Thread(ctx, ""); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("empty thread err = %v", err)
	}
	// Reply-all to your own message with no other recipients left.
	self, _ := e.svc.Send(ctx, caller(aliceWeb), mail.SendInput{To: []string{aliceWeb}, Subject: "note", Body: "b"})
	if _, err := e.svc.Send(ctx, caller(aliceWeb), mail.SendInput{InReplyTo: self.ID, ReplyAll: true, Subject: "s", Body: "b"}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("empty reply_all err = %v", err)
	}
	// Explicit to plus reply_all: explicit entries win and are not duplicated.
	rr, err := e.svc.Send(ctx, caller(carolAPI), mail.SendInput{InReplyTo: root.ID, ReplyAll: true, To: []string{bobWeb}, Subject: "s", Body: "b"})
	if err != nil || !slices.Equal(rr.To, []string{bobWeb, aliceWeb}) || len(rr.Cc) != 0 {
		t.Errorf("reply_all+to = %+v, %v", rr, err)
	}
}

func TestListScopesAndIsolation(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceOrg, aliceWeb, bobWeb, carolAPI)
	toOrgAlice := e.send(t, bobWeb, []string{aliceOrg}, "to org alice")
	toWebAlice := e.send(t, bobWeb, []string{aliceWeb}, "to web alice")
	toCarol := e.send(t, bobWeb, []string{carolAPI}, "to carol")
	toOrgNS := e.send(t, bobWeb, []string{orgNS}, "to org ns")

	has := func(addr string, want ...mail.Message) {
		t.Helper()
		r := e.list(t, mail.ListInput{Address: addr, State: "all"})
		got := ids(r.Items)
		slices.Sort(got)
		w := make([]string, len(want))
		for i, m := range want {
			w[i] = m.ID
		}
		slices.Sort(w)
		if !slices.Equal(got, w) {
			t.Errorf("list %s = %v, want %v", addr, got, w)
		}
	}
	has(aliceOrg, toOrgAlice)
	has(aliceWeb, toWebAlice)
	has(carolAPI, toCarol)
	has("github.com/acme/web", toWebAlice)
	has("github.com/acme/api", toCarol)
	has(orgNS, toOrgAlice, toWebAlice, toCarol, toOrgNS)
	has("github.com/other")
	if r := e.list(t, mail.ListInput{Address: orgNS, State: "all"}); r.Items[0].ID != toOrgNS.ID {
		t.Errorf("not newest first: %v", ids(r.Items))
	}
	// From filter, including an unknown sender.
	r := e.list(t, mail.ListInput{Address: orgNS, State: "all", From: bobWeb})
	if len(r.Items) != 4 {
		t.Errorf("from bob = %d", len(r.Items))
	}
	r = e.list(t, mail.ListInput{Address: orgNS, State: "all", From: carolAPI})
	if len(r.Items) != 0 || r.Items == nil {
		t.Errorf("from carol = %v", r.Items)
	}
	r = e.list(t, mail.ListInput{Address: orgNS, State: "all", From: "ghost@github.com/acme"})
	if len(r.Items) != 0 {
		t.Errorf("from ghost = %v", r.Items)
	}
	h := e.list(t, mail.ListInput{Address: aliceWeb}).Items[0]
	if h.From != bobWeb || h.Mailbox != aliceWeb || h.Kind != "to" || h.Subject != "to web alice" || len(h.To) != 1 {
		t.Errorf("header = %+v", h)
	}
}

func TestListStatesAndArchivedWindow(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	m1 := e.send(t, bobWeb, []string{aliceWeb}, "one")
	m2 := e.send(t, bobWeb, []string{aliceWeb}, "two")
	m3 := e.send(t, bobWeb, []string{aliceWeb}, "three")
	a := caller(aliceWeb)
	if _, err := e.svc.Read(ctx, a, mail.ReadInput{MessageID: m1.ID}); err != nil {
		t.Fatal(err)
	}
	if n, err := e.svc.Mark(ctx, a, mail.MarkInput{MessageIDs: []string{m2.ID}, State: "archived"}); err != nil || n != 1 {
		t.Fatalf("archive = %d, %v", n, err)
	}
	count := func(in mail.ListInput) []string {
		in.Address = aliceWeb
		return ids(e.list(t, in).Items)
	}
	if got := count(mail.ListInput{}); !slices.Equal(got, []string{m3.ID}) {
		t.Errorf("default unread = %v", got)
	}
	if got := count(mail.ListInput{State: "read"}); !slices.Equal(got, []string{m1.ID}) {
		t.Errorf("read = %v", got)
	}
	if got := count(mail.ListInput{State: "all"}); len(got) != 3 {
		t.Errorf("all = %v", got)
	}
	since, until := epoch.Add(-time.Hour), epoch.Add(time.Hour)
	if got := count(mail.ListInput{State: "archived", Since: &since, Until: &until}); !slices.Equal(got, []string{m2.ID}) {
		t.Errorf("archived = %v", got)
	}
	for name, in := range map[string]mail.ListInput{
		"archived no window":  {Address: aliceWeb, State: "archived"},
		"archived only since": {Address: aliceWeb, State: "archived", Since: &since},
		"archived only until": {Address: aliceWeb, State: "archived", Until: &until},
		"bad state":           {Address: aliceWeb, State: "bogus"},
		"until before since":  {Address: aliceWeb, Since: &until, Until: &since},
		"negative limit":      {Address: aliceWeb, Limit: -1},
		"bad cursor":          {Address: aliceWeb, Cursor: "garbage"},
		"bad address":         {Address: "nope"},
		"bad from":            {Address: aliceWeb, From: "nope"},
	} {
		if _, err := e.svc.List(ctx, in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// Since/until narrow by created_at.
	late := epoch.Add(time.Hour)
	if got := count(mail.ListInput{State: "all", Since: &late}); len(got) != 0 {
		t.Errorf("since in future = %v", got)
	}
}

func TestListPagination(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	var want []string
	for i := range 5 {
		want = append([]string{e.send(t, bobWeb, []string{aliceWeb}, fmt.Sprintf("m%d", i)).ID}, want...)
	}
	var got []string
	cursor := ""
	for pages := 0; ; pages++ {
		r := e.list(t, mail.ListInput{Address: aliceWeb, Limit: 2, Cursor: cursor})
		got = append(got, ids(r.Items)...)
		if r.NextCursor == "" {
			if pages != 2 {
				t.Errorf("pages = %d, want 3", pages+1)
			}
			break
		}
		cursor = r.NextCursor
		if pages > 5 {
			t.Fatal("pagination did not terminate")
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("paged = %v, want %v", got, want)
	}
	if r := e.list(t, mail.ListInput{Address: aliceWeb, Limit: 5}); r.NextCursor != "" || len(r.Items) != 5 {
		t.Errorf("exact page: cursor %q items %d", r.NextCursor, len(r.Items))
	}
	if r := e.list(t, mail.ListInput{Address: aliceWeb, Limit: 100000}); len(r.Items) != 5 {
		t.Errorf("clamped limit items = %d", len(r.Items))
	}
}

func TestListPaginationMultiRecipientMessages(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, carolAPI)
	// Each message has 2 deliveries inside the org subtree; no message may be
	// split or lost across pages.
	var want []string
	for i := range 4 {
		m := e.send(t, carolAPI, []string{aliceWeb, bobWeb}, fmt.Sprintf("m%d", i))
		want = append([]string{m.ID, m.ID}, want...)
	}
	var got []string
	cursor := ""
	for range 20 {
		r := e.list(t, mail.ListInput{Address: orgNS, State: "all", Limit: 3, Cursor: cursor})
		got = append(got, ids(r.Items)...)
		if r.NextCursor == "" {
			break
		}
		cursor = r.NextCursor
	}
	if !slices.Equal(got, want) {
		t.Errorf("paged = %v, want %v", got, want)
	}
}

func TestReadOwnershipAndAnonymous(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb, carolAPI)
	m := e.send(t, carolAPI, []string{aliceWeb}, "for alice")
	state := func(addr string) *time.Time {
		t.Helper()
		r := e.list(t, mail.ListInput{Address: addr, State: "all"})
		if len(r.Items) != 1 {
			t.Fatalf("%s items = %d", addr, len(r.Items))
		}
		return r.Items[0].ReadAt
	}
	// Anonymous and peek reads never mark.
	if _, err := e.svc.Read(ctx, mail.Caller{}, mail.ReadInput{MessageID: m.ID, Peek: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{MessageID: m.ID, Peek: true}); err != nil {
		t.Fatal(err)
	}
	if state(aliceWeb) != nil {
		t.Error("peek marked read")
	}
	if got, err := e.svc.Read(ctx, mail.Caller{}, mail.ReadInput{MessageID: m.ID}); err != nil || got.ID != m.ID {
		t.Errorf("anonymous non-peek read = %v, %v; want the message", got.ID, err)
	}
	if state(aliceWeb) != nil {
		t.Error("anonymous read marked read")
	}
	// A subagent mailbox that does not exist yet is not an error: nothing to mark.
	if got, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{MessageID: m.ID, Subagent: "ghost"}); err != nil || got.ID != m.ID {
		t.Errorf("read as unknown subagent = %v, %v", got.ID, err)
	}
	if state(aliceWeb) != nil {
		t.Error("read as unknown subagent marked alice's delivery")
	}
	// A different identity can read but cannot mark alice's delivery.
	if _, err := e.svc.Read(ctx, caller(bobWeb), mail.ReadInput{MessageID: m.ID}); err != nil {
		t.Fatal(err)
	}
	if state(aliceWeb) != nil {
		t.Error("non-owner read marked alice's delivery")
	}
	// The owner marks it, once; a second read keeps the first timestamp.
	if _, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{MessageID: m.ID}); err != nil {
		t.Fatal(err)
	}
	first := state(aliceWeb)
	if first == nil {
		t.Fatal("owner read did not mark")
	}
	if _, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{MessageID: m.ID}); err != nil {
		t.Fatal(err)
	}
	if second := state(aliceWeb); !second.Equal(*first) {
		t.Errorf("read_at changed: %v -> %v", first, second)
	}
	// Errors.
	if _, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("empty id err = %v", err)
	}
	if _, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{MessageID: "msg-zzz"}); !errors.Is(err, mail.ErrMessageNotFound) {
		t.Errorf("unknown id err = %v", err)
	}
	if _, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{MessageID: m.ID, Subagent: "Bad Name"}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("bad subagent err = %v", err)
	}
	// An unregistered caller owns no delivery: the read succeeds, marking nothing.
	if _, err := e.svc.Read(ctx, caller("ghost@github.com/acme/web"), mail.ReadInput{MessageID: m.ID}); err != nil {
		t.Errorf("unregistered caller err = %v", err)
	}
}

func TestBareNamespaceNeverMutated(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	m := e.send(t, bobWeb, []string{webNS}, "ns only")
	if _, err := e.svc.Read(ctx, caller(aliceWeb), mail.ReadInput{MessageID: m.ID}); err != nil {
		t.Fatal(err)
	}
	n, err := e.svc.Mark(ctx, caller(aliceWeb), mail.MarkInput{MessageIDs: []string{m.ID}, State: "read"})
	if err != nil || n != 0 {
		t.Errorf("mark on namespace delivery = %d, %v", n, err)
	}
	r := e.list(t, mail.ListInput{Address: webNS, State: "all"})
	if len(r.Items) != 1 || r.Items[0].ReadAt != nil || r.Items[0].ArchivedAt != nil {
		t.Errorf("namespace delivery mutated: %+v", r.Items)
	}
}

func TestMark(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	m1 := e.send(t, bobWeb, []string{aliceWeb}, "one")
	m2 := e.send(t, bobWeb, []string{aliceWeb}, "two")
	m3 := e.send(t, aliceWeb, []string{bobWeb}, "not alice's")
	a := caller(aliceWeb)
	mark := func(state string, ids ...string) int {
		t.Helper()
		n, err := e.svc.Mark(ctx, a, mail.MarkInput{MessageIDs: ids, State: state})
		if err != nil {
			t.Fatalf("mark %s: %v", state, err)
		}
		return n
	}
	if n := mark("read", m1.ID, m2.ID, m3.ID, m1.ID); n != 2 {
		t.Errorf("read changed %d, want 2 (foreign + duplicate ignored)", n)
	}
	if n := mark("read", m1.ID); n != 0 {
		t.Errorf("re-mark read changed %d", n)
	}
	if n := mark("unread", m1.ID, m2.ID); n != 2 {
		t.Errorf("unread changed %d", n)
	}
	if n := mark("archived", m1.ID); n != 1 {
		t.Errorf("archive changed %d", n)
	}
	if n := mark("unarchived", m1.ID, m2.ID); n != 1 {
		t.Errorf("unarchive changed %d", n)
	}
	// Bob's delivery of m3 is untouched.
	if r := e.list(t, mail.ListInput{Address: bobWeb, State: "unread"}); len(r.Items) != 1 || r.Items[0].ID != m3.ID {
		t.Errorf("bob unread = %+v", r.Items)
	}
	// Subagent scope.
	e.send(t, bobWeb, []string{"helper@" + aliceWeb}, "for helper")
	hm := e.list(t, mail.ListInput{Address: "helper@" + aliceWeb}).Items[0]
	if n, err := e.svc.Mark(ctx, a, mail.MarkInput{MessageIDs: []string{hm.ID}, State: "read"}); err != nil || n != 0 {
		t.Errorf("identity mark of subagent delivery = %d, %v", n, err)
	}
	if n, err := e.svc.Mark(ctx, a, mail.MarkInput{MessageIDs: []string{hm.ID}, State: "read", Subagent: "helper"}); err != nil || n != 1 {
		t.Errorf("subagent mark = %d, %v", n, err)
	}
	if _, err := e.svc.Mark(ctx, a, mail.MarkInput{MessageIDs: []string{hm.ID}, State: "read", Subagent: "ghost"}); !errors.Is(err, mail.ErrMailboxNotFound) {
		t.Errorf("ghost subagent err = %v", err)
	}
	if _, err := e.svc.Mark(ctx, mail.Caller{}, mail.MarkInput{MessageIDs: []string{m1.ID}, State: "read"}); !errors.Is(err, mail.ErrNotSignedOn) {
		t.Errorf("anonymous mark err = %v", err)
	}
	for name, in := range map[string]mail.MarkInput{
		"bad state": {MessageIDs: []string{m1.ID}, State: "x"},
		"no ids":    {State: "read"},
		"too many":  {MessageIDs: make([]string, 201), State: "read"},
	} {
		if _, err := e.svc.Mark(ctx, a, in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("%s err = %v", name, err)
		}
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb, carolAPI)
	for i := range 3 {
		if _, err := e.svc.Send(ctx, caller(bobWeb), mail.SendInput{To: []string{aliceWeb, carolAPI}, Subject: fmt.Sprintf("deploy %d", i), Body: "the pipeline needs a rollout"}); err != nil {
			t.Fatal(err)
		}
	}
	e.send(t, bobWeb, []string{aliceWeb}, "lunch")
	r, err := e.svc.Search(ctx, mail.SearchInput{Address: aliceWeb, Query: "rollout"})
	if err != nil || len(r.Hits) != 3 {
		t.Fatalf("search = %+v, %v", r, err)
	}
	h := r.Hits[0]
	if h.Mailbox != aliceWeb || h.From != bobWeb || !strings.Contains(h.Snippet, "[rollout]") || h.MessageID == "" {
		t.Errorf("hit = %+v", h)
	}
	r, err = e.svc.Search(ctx, mail.SearchInput{Address: orgNS, Query: "rollout"})
	if err != nil || len(r.Hits) != 3 {
		t.Errorf("org search dedupes multi-delivery messages: %d, %v", len(r.Hits), err)
	}
	if r, _ := e.svc.Search(ctx, mail.SearchInput{Address: "github.com/other", Query: "rollout"}); len(r.Hits) != 0 || r.Hits == nil {
		t.Errorf("other org hits = %v", r.Hits)
	}
	// Pagination by offset.
	var got []string
	cursor := ""
	for range 5 {
		p, err := e.svc.Search(ctx, mail.SearchInput{Address: aliceWeb, Query: "rollout", Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range p.Hits {
			got = append(got, h.MessageID)
		}
		if p.NextCursor == "" {
			break
		}
		cursor = p.NextCursor
	}
	if len(got) != 3 || got[0] == got[1] || got[1] == got[2] {
		t.Errorf("paged hits = %v", got)
	}
	// Errors.
	if _, err := e.svc.Search(ctx, mail.SearchInput{Address: aliceWeb, Query: "  "}); !errors.Is(err, mail.ErrBadQuery) || !errors.Is(err, store.ErrBadQuery) {
		t.Errorf("empty query err = %v", err)
	}
	if _, err := e.svc.Search(ctx, mail.SearchInput{Address: aliceWeb, Query: `"unterminated`}); !errors.Is(err, mail.ErrBadQuery) {
		t.Errorf("syntax err = %v", err)
	}
	for name, in := range map[string]mail.SearchInput{
		"bad addr":   {Address: "x", Query: "q"},
		"neg limit":  {Address: aliceWeb, Query: "q", Limit: -1},
		"bad cursor": {Address: aliceWeb, Query: "q", Cursor: "abc"},
		"neg cursor": {Address: aliceWeb, Query: "q", Cursor: "-1"},
	} {
		if _, err := e.svc.Search(ctx, in); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("%s err = %v", name, err)
		}
	}
}

func TestRoster(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceOrg, aliceWeb, bobWeb, carolAPI, otherOrgA)
	e.send(t, bobWeb, []string{"scout@" + aliceWeb}, "hi")
	e.pres[aliceWeb] = []mail.OnlineStatus{{SessionID: "s1", Machine: "devbox", Away: "lunch", SignedOnAt: epoch}}

	r, err := e.svc.Roster(ctx, orgNS)
	if err != nil || len(r) != 4 {
		t.Fatalf("org roster = %v, %v", r, err)
	}
	by := map[string]mail.RosterEntry{}
	for _, en := range r {
		by[en.Address] = en
	}
	if by[aliceWeb].Online[0].Away != "lunch" || !slices.Equal(by[aliceWeb].Subagents, []string{"scout"}) {
		t.Errorf("alice web = %+v", by[aliceWeb])
	}
	if by[aliceOrg].Online == nil || len(by[aliceOrg].Online) != 0 || by[bobWeb].Role != "dev" {
		t.Errorf("alice org / bob = %+v / %+v", by[aliceOrg], by[bobWeb])
	}
	if r, err = e.svc.Roster(ctx, webNS); err != nil || len(r) != 2 {
		t.Errorf("web roster = %v, %v", r, err)
	}
	if r, err = e.svc.Roster(ctx, aliceOrg); err != nil || len(r) != 1 || r[0].Address != aliceOrg {
		t.Errorf("org-identity roster isolates from repo identity: %v, %v", r, err)
	}
	if r, err = e.svc.Roster(ctx, "scout@"+aliceWeb); err != nil || len(r) != 1 || r[0].Address != aliceWeb {
		t.Errorf("subagent roster = %v, %v", r, err)
	}
	if _, err = e.svc.Roster(ctx, "ghost@github.com/acme"); !errors.Is(err, mail.ErrMailboxNotFound) {
		t.Errorf("ghost roster err = %v", err)
	}
	if _, err = e.svc.Roster(ctx, "nope"); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("bad roster err = %v", err)
	}
	if r, err = e.svc.Roster(ctx, "github.com/empty"); err != nil || len(r) != 0 || r == nil {
		t.Errorf("empty roster = %v, %v", r, err)
	}
	// A nil presence is allowed.
	s2 := mail.New(e.st, nil)
	if r, err := s2.Roster(ctx, orgNS); err != nil || len(r) != 4 {
		t.Errorf("nil presence roster = %d, %v", len(r), err)
	}
}

func TestDefaultsAndConcurrentIDs(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	s := mail.New(e.st, nil) // real clock + crypto entropy
	if _, err := s.Register(context.Background(), mail.Caller{}, mail.RegisterInput{Address: aliceWeb, Role: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(context.Background(), mail.Caller{}, mail.RegisterInput{Address: bobWeb, Role: "x"}); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := s.Send(context.Background(), caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Subject: "s", Body: "b"})
			errs <- err
		}()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestListPagesThroughOneMessagesDeliveries(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	m := e.send(t, bobWeb, []string{aliceWeb, orgNS}, "fanout")
	older := e.send(t, bobWeb, []string{aliceWeb}, "single")
	var got []string
	cursor := ""
	for range 10 {
		r := e.list(t, mail.ListInput{Address: orgNS, State: "all", Limit: 1, Cursor: cursor})
		for _, h := range r.Items {
			got = append(got, h.ID+"@"+h.Mailbox)
		}
		if cursor = r.NextCursor; cursor == "" {
			break
		}
	}
	want := []string{older.ID + "@" + aliceWeb, m.ID + "@" + orgNS, m.ID + "@" + aliceWeb}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("paged deliveries = %v, want %v", got, want)
	}
	if _, err := e.svc.List(context.Background(), mail.ListInput{Address: orgNS, Cursor: "msg-x"}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("malformed cursor err = %v", err)
	}
}

func TestReplyAllExcludesIdentityAndKeepsCallerCc(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, carolAPI)
	ctx := context.Background()
	root := e.send(t, aliceWeb, []string{bobWeb}, "q")
	// bob's subagent replies: bob's identity must not be re-added.
	r, err := e.svc.Send(ctx, caller(bobWeb), mail.SendInput{Subagent: "helper", InReplyTo: root.ID, ReplyAll: true, Subject: "Re", Body: "b"})
	if err != nil || !slices.Equal(r.To, []string{aliceWeb}) || len(r.Cc) != 0 {
		t.Errorf("subagent reply_all = %+v, %v", r, err)
	}
	// An address the caller put in cc stays in cc, not promoted by the expansion.
	r, err = e.svc.Send(ctx, caller(carolAPI), mail.SendInput{InReplyTo: root.ID, ReplyAll: true, Cc: []string{aliceWeb}, Subject: "Re", Body: "b"})
	if err != nil || !slices.Equal(r.To, []string{bobWeb}) || !slices.Equal(r.Cc, []string{aliceWeb}) {
		t.Errorf("reply_all with caller cc = %+v, %v", r, err)
	}
}

func TestSendTimestampsFollowCommitOrder(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	var ticks atomic.Int64
	slow, release := make(chan struct{}), make(chan struct{})
	clock := func() time.Time {
		n := ticks.Add(1)
		if n == 1 {
			close(slow)
			select { // wait for the second send to finish, if it can
			case <-release:
			case <-time.After(300 * time.Millisecond):
			}
		}
		return epoch.Add(time.Duration(n) * time.Second)
	}
	svc := mail.New(e.st, e.pres, mail.WithClock(clock), mail.WithEntropy(&counterReader{}))
	type result struct {
		m   mail.Message
		end time.Time
	}
	do := func(subject string, out chan<- result) {
		m, err := svc.Send(context.Background(), caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Subject: subject, Body: "b"})
		if err != nil {
			t.Errorf("send %s: %v", subject, err)
		}
		out <- result{m, time.Now()}
	}
	first, second := make(chan result, 1), make(chan result, 1)
	go do("first", first)
	<-slow
	go func() { do("second", second); close(release) }()
	a, b := <-first, <-second
	if a.end.After(b.end) {
		a, b = b, a
	}
	if a.m.ID >= b.m.ID || a.m.CreatedAt.After(b.m.CreatedAt) {
		t.Errorf("committed %s (%s) before %s (%s): ids/timestamps out of commit order",
			a.m.Subject, a.m.CreatedAt, b.m.Subject, b.m.CreatedAt)
	}
}
