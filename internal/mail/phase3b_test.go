package mail_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/mail"
)

func TestApprovalSettings(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	a, err := e.svc.ApprovalSettings(ctx)
	if err != nil || a.Enabled || a.TimeoutSeconds != mail.DefaultApprovalTimeoutSeconds {
		t.Fatalf("default approvals = %+v, %v", a, err)
	}
	for _, secs := range []int{-1, 9, 3601} {
		if _, err := e.svc.SetApprovalSettings(ctx, mail.ApprovalSettings{Enabled: true, TimeoutSeconds: secs}); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("timeout %d err = %v", secs, err)
		}
	}
	got, err := e.svc.SetApprovalSettings(ctx, mail.ApprovalSettings{Enabled: true, TimeoutSeconds: 60})
	if err != nil || got != (mail.ApprovalSettings{Enabled: true, TimeoutSeconds: 60}) {
		t.Fatalf("set = %+v, %v", got, err)
	}
	if a, _ = e.svc.ApprovalSettings(ctx); !a.Enabled || a.TimeoutSeconds != 60 {
		t.Errorf("approvals = %+v", a)
	}
	if got, _ = e.svc.SetApprovalSettings(ctx, mail.ApprovalSettings{}); got.Enabled || got.TimeoutSeconds != mail.DefaultApprovalTimeoutSeconds {
		t.Errorf("zero set = %+v", got)
	}
}

func TestRetentionAndBackupSettings(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	r, err := e.svc.Retention(ctx)
	if err != nil || r.PruneArchivedAfterDays != 0 {
		t.Fatalf("default retention = %+v, %v", r, err)
	}
	if err := e.svc.SetRetention(ctx, mail.RetentionSettings{PruneArchivedAfterDays: 90}); err != nil {
		t.Fatal(err)
	}
	if r, _ = e.svc.Retention(ctx); r.PruneArchivedAfterDays != 90 {
		t.Errorf("retention = %+v", r)
	}
	if err := e.svc.SetRetention(ctx, mail.RetentionSettings{PruneArchivedAfterDays: -1}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("negative days err = %v", err)
	}

	b, _ := e.svc.Backups(ctx)
	if b.EveryHours != 24 || b.Keep != 7 || b.Dir != "" {
		t.Errorf("default backups = %+v", b)
	}
	if err := e.svc.SetBackups(ctx, mail.BackupSettings{Dir: "relative/dir"}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("relative dir err = %v", err)
	}
	if err := e.svc.SetBackups(ctx, mail.BackupSettings{EveryHours: 6, Keep: 3, Dir: "/var/backups/aim/"}); err != nil {
		t.Fatal(err)
	}
	b, _ = e.svc.Backups(ctx)
	if b.EveryHours != 6 || b.Keep != 3 || b.Dir != "/var/backups/aim" {
		t.Errorf("backups = %+v", b)
	}
	if err := e.svc.SetBackups(ctx, mail.BackupSettings{EveryHours: 1, Keep: 1000}); !errors.Is(err, mail.ErrInvalidInput) {
		t.Errorf("keep err = %v", err)
	}

	at := epoch
	if err := e.svc.SetBackupStatus(ctx, mail.BackupStatus{LastAt: &at, LastSize: 42, LastPath: "/x"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.svc.BackupStatus(ctx); st.LastSize != 42 || st.LastAt == nil || !st.LastAt.Equal(epoch) {
		t.Errorf("status = %+v", st)
	}
}

func TestNotifySettings(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	if ok, err := e.svc.NotifyConfigured(ctx); err != nil || ok {
		t.Fatalf("configured = %v, %v", ok, err)
	}
	got, err := e.svc.SetNotify(ctx, mail.NotifySettings{
		PublicURL: "http://devbox:8080/",
		Webhooks:  []mail.Webhook{{Name: "team", URL: "https://discord.com/api/webhooks/1/abc", Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicURL != "http://devbox:8080" || len(got.Webhooks) != 1 || !strings.HasPrefix(got.Webhooks[0].ID, "whk-") {
		t.Fatalf("notify = %+v", got)
	}
	id := got.Webhooks[0].ID
	now := epoch
	if err := e.svc.UpdateNotify(ctx, func(n *mail.NotifySettings) {
		n.Webhooks[0].LastStatus, n.Webhooks[0].LastSentAt = 204, &now
	}); err != nil {
		t.Fatal(err)
	}
	// Resubmitting without a URL keeps it and the delivery status.
	got, err = e.svc.SetNotify(ctx, mail.NotifySettings{Webhooks: []mail.Webhook{{ID: id, Name: "renamed", Enabled: false}}})
	if err != nil {
		t.Fatal(err)
	}
	w := got.Webhooks[0]
	if w.URL != "https://discord.com/api/webhooks/1/abc" || w.Name != "renamed" || w.Enabled || w.LastStatus != 204 {
		t.Errorf("webhook = %+v", w)
	}
	// A new URL resets the history.
	got, _ = e.svc.SetNotify(ctx, mail.NotifySettings{Webhooks: []mail.Webhook{{ID: id, Name: "r", URL: "https://hooks.slack.com/services/x"}}})
	if got.Webhooks[0].LastStatus != 0 || got.Webhooks[0].LastSentAt != nil {
		t.Errorf("history kept: %+v", got.Webhooks[0])
	}
	if ok, _ := e.svc.NotifyConfigured(ctx); !ok {
		t.Error("not configured after set")
	}

	bad := []mail.NotifySettings{
		{PublicURL: "ftp://x"},
		{Webhooks: []mail.Webhook{{Name: "", URL: "https://x.test"}}},
		{Webhooks: []mail.Webhook{{Name: "n", URL: "not a url"}}},
		{Webhooks: []mail.Webhook{{ID: "whk-nope", Name: "n", URL: "https://x.test"}}},
		{Webhooks: []mail.Webhook{{ID: id, Name: "n"}, {ID: id, Name: "m"}}},
		{Webhooks: make([]mail.Webhook, mail.MaxWebhooks+1)},
	}
	for i, n := range bad {
		if _, err := e.svc.SetNotify(ctx, n); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("bad[%d] err = %v", i, err)
		}
	}
}

func TestSavedFilters(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	a, err := e.svc.CreateSavedFilter(ctx, " Urgent ", map[string]string{"state": "unread", "from": "alice@github.com/acme/web"})
	if err != nil || a.Name != "Urgent" || !strings.HasPrefix(a.ID, "flt-") {
		t.Fatalf("create = %+v, %v", a, err)
	}
	b, _ := e.svc.CreateSavedFilter(ctx, "Other", map[string]string{"q": "deploy"})
	name := "Renamed"
	u, err := e.svc.UpdateSavedFilter(ctx, a.ID, &name, nil)
	if err != nil || u.Name != "Renamed" || u.Params["state"] != "unread" {
		t.Fatalf("update = %+v, %v", u, err)
	}
	if u, err = e.svc.UpdateSavedFilter(ctx, a.ID, nil, map[string]string{"q": "x"}); err != nil || len(u.Params) != 1 {
		t.Fatalf("update params = %+v, %v", u, err)
	}
	list, _ := e.svc.SavedFilters(ctx)
	if len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("list = %+v", list)
	}
	if err := e.svc.DeleteSavedFilter(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteSavedFilter(ctx, a.ID); !errors.Is(err, mail.ErrNotFound) {
		t.Errorf("delete missing err = %v", err)
	}
	if _, err := e.svc.UpdateSavedFilter(ctx, "flt-x", &name, nil); !errors.Is(err, mail.ErrNotFound) {
		t.Errorf("update missing err = %v", err)
	}
	for _, c := range []struct {
		name   string
		params map[string]string
	}{{"", map[string]string{"a": "b"}}, {"n", nil}, {"n", map[string]string{"": "v"}}, {"n", map[string]string{"k": strings.Repeat("x", 501)}}} {
		if _, err := e.svc.CreateSavedFilter(ctx, c.name, c.params); !errors.Is(err, mail.ErrInvalidInput) {
			t.Errorf("create %q %v err = %v", c.name, c.params, err)
		}
	}
	empty := newEnv(t)
	if l, err := empty.svc.SavedFilters(ctx); err != nil || l == nil || len(l) != 0 {
		t.Errorf("empty list = %v, %v", l, err)
	}
}

func TestMuteThread(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	m1 := e.send(t, aliceWeb, []string{"operator"}, "muted one")
	m2 := e.send(t, bobWeb, []string{"operator"}, "kept one")
	e.send(t, aliceWeb, []string{bobWeb}, "not operator mail")

	views := func() int {
		v, err := e.svc.Views(ctx, epoch)
		if err != nil {
			t.Fatal(err)
		}
		return v.OperatorUnread
	}
	if n := views(); n != 2 {
		t.Fatalf("unread = %d, want 2", n)
	}
	if err := e.svc.MuteThread(ctx, "msg-nope"); !errors.Is(err, mail.ErrMessageNotFound) {
		t.Errorf("mute unknown err = %v", err)
	}
	for range 2 { // idempotent
		if err := e.svc.MuteThread(ctx, m1.ThreadID); err != nil {
			t.Fatal(err)
		}
	}
	if n := views(); n != 1 {
		t.Errorf("unread after mute = %d, want 1", n)
	}
	if muted, _ := e.svc.ThreadMuted(ctx, m1.ThreadID); !muted {
		t.Error("ThreadMuted = false")
	}
	if ms, _ := e.svc.MutedThreads(ctx); len(ms) != 1 || ms[0].ThreadID != m1.ThreadID {
		t.Errorf("muted = %+v", ms)
	}

	got := e.list(t, mail.ListInput{Address: "operator", State: mail.StateAll})
	if !slices.Equal(ids(got.Items), []string{m2.ID}) {
		t.Errorf("operator inbox = %v, want only %s", ids(got.Items), m2.ID)
	}
	got = e.list(t, mail.ListInput{Address: "operator", State: mail.StateAll, IncludeMuted: true})
	if len(got.Items) != 2 {
		t.Fatalf("with muted = %d items", len(got.Items))
	}
	for _, h := range got.Items {
		if h.Muted != (h.ID == m1.ID) {
			t.Errorf("header %s muted = %v", h.ID, h.Muted)
		}
	}
	// Non-operator scopes are unaffected.
	if n := len(e.list(t, mail.ListInput{State: mail.StateAll}).Items); n != 3 {
		t.Errorf("all mail = %d, want 3", n)
	}
	th, _ := e.svc.Thread(ctx, m1.ThreadID)
	if len(th) != 1 || !th[0].Muted {
		t.Errorf("thread muted = %+v", th)
	}
	rd, _ := e.svc.Read(ctx, mail.Caller{}, mail.ReadInput{MessageID: m1.ID, Peek: true})
	if !rd.Muted {
		t.Error("Read muted = false")
	}

	if err := e.svc.UnmuteThread(ctx, m1.ThreadID); err != nil {
		t.Fatal(err)
	}
	if n := views(); n != 2 {
		t.Errorf("unread after unmute = %d", n)
	}
	if n := len(e.list(t, mail.ListInput{Address: "operator", State: mail.StateAll}).Items); n != 2 {
		t.Errorf("inbox after unmute = %d", n)
	}
}

func TestMutedListPagination(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb)
	var keep []string
	for i := range 6 {
		m := e.send(t, aliceWeb, []string{"operator"}, "m"+string(rune('a'+i)))
		if i%3 != 0 { // mute 1 in 3... keep indexes 1,2,4,5
			keep = append(keep, m.ID)
		} else if err := e.svc.MuteThread(ctx, m.ThreadID); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	cursor := ""
	for range 5 {
		r := e.list(t, mail.ListInput{Address: "operator", State: mail.StateAll, Limit: 1, Cursor: cursor})
		got = append(got, ids(r.Items)...)
		if cursor = r.NextCursor; cursor == "" {
			break
		}
	}
	slices.Reverse(keep)
	if !slices.Equal(got, keep) {
		t.Errorf("paged = %v, want %v", got, keep)
	}
}

func TestPrune(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	archive := func(who string, ids ...string) {
		t.Helper()
		if _, err := e.svc.Mark(ctx, caller(who), mail.MarkInput{MessageIDs: ids, State: mail.MarkArchived}); err != nil {
			t.Fatal(err)
		}
	}
	oldArchived := e.send(t, aliceWeb, []string{bobWeb}, "old archived")
	oldArchived2 := e.send(t, aliceWeb, []string{bobWeb}, "old archived two")
	oldUnread := e.send(t, aliceWeb, []string{bobWeb}, "old unread")
	reply, err := e.svc.Send(ctx, caller(bobWeb), mail.SendInput{InReplyTo: oldArchived.ID, To: []string{aliceWeb}, Subject: "Re", Body: "reply body"})
	if err != nil {
		t.Fatal(err)
	}
	archive(bobWeb, oldArchived.ID, oldArchived2.ID)
	two, err := e.svc.Send(ctx, caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Cc: []string{aliceWeb}, Subject: "two recips", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	archive(bobWeb, two.ID) // alice's copy stays unarchived
	e.tick.Add(10 * 86400)
	recent := e.send(t, aliceWeb, []string{bobWeb}, "recent archived")
	archive(bobWeb, recent.ID)
	now := epoch.Add(time.Duration(e.tick.Load()) * time.Second)

	// Off by default.
	if r, err := e.svc.Prune(ctx, now, false); err != nil || r.Enabled || r.Messages != 0 {
		t.Fatalf("prune while off = %+v, %v", r, err)
	}
	if err := e.svc.SetRetention(ctx, mail.RetentionSettings{PruneArchivedAfterDays: 5}); err != nil {
		t.Fatal(err)
	}
	// Archive the reply's only delivery (alice) too, so it is prunable.
	archive(aliceWeb, reply.ID)

	dry, err := e.svc.Prune(ctx, now, true)
	if err != nil || dry.Messages != 3 || !dry.DryRun {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}
	if n := countMessages(t, e); n != 6 {
		t.Fatalf("dry run deleted rows: %d messages", n)
	}
	res, err := e.svc.Prune(ctx, now, false)
	if err != nil || res.Messages != 3 {
		t.Fatalf("prune = %+v, %v", res, err)
	}
	// Remaining: oldUnread, two (partially archived), recent.
	left, _ := e.svc.List(ctx, mail.ListInput{State: mail.StateAll, Limit: 50})
	subjects := make([]string, 0, len(left.Items))
	for _, h := range left.Items {
		subjects = append(subjects, h.Subject)
	}
	slices.Sort(subjects)
	want := []string{"old unread", "recent archived", "two recips", "two recips"}
	if !slices.Equal(subjects, want) {
		t.Errorf("remaining deliveries = %v, want %v", subjects, want)
	}
	_ = oldUnread
	// FTS no longer finds a pruned message.
	hits, err := e.svc.Search(ctx, mail.SearchInput{Query: "reply"})
	if err != nil || len(hits.Hits) != 0 {
		t.Errorf("search pruned = %+v, %v", hits, err)
	}
	if hits, _ := e.svc.Search(ctx, mail.SearchInput{Query: "unread"}); len(hits.Hits) != 1 {
		t.Errorf("search kept = %+v", hits)
	}
	// Idempotent.
	if res, _ := e.svc.Prune(ctx, now, false); res.Messages != 0 {
		t.Errorf("second prune = %+v", res)
	}
}

func TestPruneNullsDanglingReplies(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, aliceWeb, bobWeb)
	root := e.send(t, aliceWeb, []string{bobWeb}, "root")
	if _, err := e.svc.Mark(ctx, caller(bobWeb), mail.MarkInput{MessageIDs: []string{root.ID}, State: mail.MarkArchived}); err != nil {
		t.Fatal(err)
	}
	e.tick.Add(10 * 86400)
	reply, err := e.svc.Send(ctx, caller(bobWeb), mail.SendInput{InReplyTo: root.ID, To: []string{aliceWeb}, Subject: "Re", Body: "r"})
	if err != nil {
		t.Fatal(err)
	}
	now := epoch.Add(time.Duration(e.tick.Load()) * time.Second)
	res, err := e.svc.PruneAfter(ctx, now, 5, false)
	if err != nil || res.Messages != 1 {
		t.Fatalf("prune = %+v, %v", res, err)
	}
	m, err := e.svc.Read(ctx, mail.Caller{}, mail.ReadInput{MessageID: reply.ID, Peek: true})
	if err != nil || m.InReplyTo != "" || m.ThreadID != root.ThreadID {
		t.Errorf("reply = %+v, %v", m, err)
	}
}

func countMessages(t *testing.T, e *env) int {
	t.Helper()
	s, err := e.svc.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return int(s.Messages)
}
