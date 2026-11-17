package mail_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store/db"
)

type countingPresence struct{ fakePresence }

func (countingPresence) Sessions() (int, int) { return 3, 2 }

// seedTraffic registers identities and sends mail so the aggregates have
// something to count. The env clock advances one second per call.
func seedTraffic(t *testing.T, e *env) {
	t.Helper()
	e.register(t, aliceOrg, aliceWeb, bobWeb, carolAPI)
	e.send(t, aliceWeb, []string{bobWeb, "operator"}, "one")
	e.send(t, bobWeb, []string{aliceWeb, webNS}, "two")
	e.send(t, carolAPI, []string{orgNS}, "three")
	if _, err := e.svc.Send(context.Background(), caller(aliceWeb), mail.SendInput{To: []string{bobWeb}, Subject: "sub", Body: "b", Subagent: "helper"}); err != nil {
		t.Fatal(err)
	}
}

func TestTree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	seedTraffic(t, e)
	e.pres[aliceWeb] = []mail.OnlineStatus{{SessionID: "s"}}
	if _, err := e.svc.Retire(ctx, carolAPI); err != nil {
		t.Fatal(err)
	}
	tree, err := e.svc.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Orgs) != 1 || tree.Orgs[0].Address != "github.com/acme" {
		t.Fatalf("orgs = %+v", tree.Orgs)
	}
	org := tree.Orgs[0]
	if len(org.Identities) != 1 || org.Identities[0].Address != aliceOrg || org.Identities[0].Unread != 0 {
		t.Errorf("org identities = %+v", org.Identities)
	}
	if len(org.Repos) != 2 || org.Repos[0].Name != "api" || org.Repos[1].Name != "web" {
		t.Fatalf("repos = %+v", org.Repos)
	}
	api, web := org.Repos[0], org.Repos[1]
	// carol (retired) sent to the org namespace; nothing was delivered to her.
	if len(api.Identities) != 1 || !api.Identities[0].Retired || api.Unread != 0 {
		t.Errorf("api = %+v", api)
	}
	if api.Identities[0].LastActivity == nil {
		t.Error("carol's send must count as activity")
	}
	// web: alice receives "two" (1), bob receives "one" + "sub" (2), web
	// namespace receives "two" (1).
	if web.Unread != 4 || web.Address != "github.com/acme/web" {
		t.Errorf("web unread = %d (%+v)", web.Unread, web)
	}
	var alice, bob mail.MailboxNode
	for _, n := range web.Identities {
		switch n.Address {
		case aliceWeb:
			alice = n
		case bobWeb:
			bob = n
		}
	}
	if alice.Unread != 1 || alice.Online != 1 || !slices.Equal(alice.Subagents, []string{"helper"}) {
		t.Errorf("alice = %+v", alice)
	}
	if bob.Unread != 2 || bob.Role != "dev" {
		t.Errorf("bob = %+v", bob)
	}
	// The org rolls up repos, org identities and the org namespace (carol's
	// "three"); the operator mailbox lies outside the tree.
	if org.Unread != 5 {
		t.Errorf("org unread = %d, want 5", org.Unread)
	}
	if len(org.Activity.Activity) != mail.ActivityDays {
		t.Fatalf("activity len = %d", len(org.Activity.Activity))
	}
	sum := 0
	for _, n := range org.Activity.Activity {
		sum += n
	}
	if sum != 5 || org.Activity.Activity[mail.ActivityDays-1] != 5 {
		t.Errorf("org activity = %v, want 5 today", org.Activity.Activity)
	}
	if org.LastActivity == nil || !org.LastActivity.After(epoch) {
		t.Errorf("org last activity = %v", org.LastActivity)
	}
}

func TestTreeDailyActivity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	e.send(t, aliceWeb, []string{bobWeb}, "recent")
	tree, err := e.svc.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bob := tree.Orgs[0].Repos[0].Identities[1]
	if bob.Address != bobWeb || bob.Activity.Activity[6] != 1 || bob.Activity.Activity[5] != 0 {
		t.Errorf("bob activity = %+v", bob)
	}
}

func TestTreeEmpty(t *testing.T) {
	t.Parallel()
	tree, err := newEnv(t).svc.Tree(context.Background())
	if err != nil || len(tree.Orgs) != 0 || tree.Orgs == nil {
		t.Errorf("tree = %+v, %v", tree, err)
	}
}

func TestViews(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	v, err := e.svc.Views(ctx, epoch)
	if err != nil || v != (mail.ViewCounts{}) {
		t.Fatalf("empty views = %+v, %v", v, err)
	}
	seedTraffic(t, e)
	v, err = e.svc.Views(ctx, epoch)
	if err != nil {
		t.Fatal(err)
	}
	// Deliveries: one->bob,operator; two->alice,web ns; three->org ns; sub->bob.
	if v.OperatorUnread != 1 || v.UnreadEverywhere != 6 || v.Today != 4 {
		t.Errorf("views = %+v", v)
	}
	later, err := e.svc.Views(ctx, epoch.Add(time.Hour))
	if err != nil || later.Today != 0 {
		t.Errorf("later today = %+v, %v", later, err)
	}
}

func TestStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	seedTraffic(t, e)
	st, err := e.svc.Stats(ctx)
	if err != nil || st.Messages != 4 || st.DBSizeBytes <= 0 || st.Sessions != 0 {
		t.Errorf("stats = %+v, %v", st, err)
	}
	svc := mail.New(e.st, countingPresence{e.pres}, mail.WithClock(func() time.Time { return epoch }))
	st, err = svc.Stats(ctx)
	if err != nil || st.Sessions != 3 || st.Machines != 2 {
		t.Errorf("stats with sessions = %+v, %v", st, err)
	}
}

func TestOrgChart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb, carolAPI)
	set := func(a, r string) {
		if _, err := e.svc.UpdateIdentity(ctx, a, mail.UpdateInput{ReportsTo: ptr(r)}); err != nil {
			t.Fatal(err)
		}
	}
	set(aliceWeb, "operator")
	set(bobWeb, aliceWeb)
	set(carolAPI, "ghost@github.com/acme/web") // dangling target
	e.pres[bobWeb] = []mail.OnlineStatus{{SessionID: "s"}}
	chart, err := e.svc.OrgChart(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(chart.Nodes) != 4 || !chart.Nodes[0].Operator || chart.Nodes[0].Address != "operator" {
		t.Fatalf("nodes = %+v", chart.Nodes)
	}
	want := []mail.OrgChartEdge{{From: aliceWeb, To: "operator"}, {From: bobWeb, To: aliceWeb}}
	if !slices.Equal(chart.Edges, want) {
		t.Errorf("edges = %+v, want %+v", chart.Edges, want)
	}
	if len(chart.Nodes[3].Online) != 1 {
		t.Errorf("bob presence = %+v", chart.Nodes[3])
	}
	if _, err := e.svc.Retire(ctx, bobWeb); err != nil {
		t.Fatal(err)
	}
	chart, _ = e.svc.OrgChart(ctx)
	if len(chart.Nodes) != 3 {
		t.Errorf("nodes after retire = %d", len(chart.Nodes))
	}
}

func TestTreeActivityBuckets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, aliceWeb, bobWeb)
	alice, _ := e.st.Read().GetMailboxByKey(ctx, keyFor("web", "alice"))
	bob, _ := e.st.Read().GetMailboxByKey(ctx, keyFor("web", "bob"))
	at := func(d time.Duration) int64 { return epoch.Add(d).UnixMilli() }
	err := e.st.WithTx(ctx, func(q *db.Queries) error {
		for i, ts := range []int64{at(-48 * time.Hour), at(-10 * 24 * time.Hour), at(48 * time.Hour)} {
			id := fmt.Sprintf("msg-old%d", i)
			if err := q.InsertMessage(ctx, db.InsertMessageParams{ID: id, ThreadID: id, FromMailbox: alice.ID, Subject: "s", Body: "b", CreatedAt: ts}); err != nil {
				return err
			}
			if err := q.InsertDelivery(ctx, db.InsertDeliveryParams{MessageID: id, MailboxID: bob.ID, Kind: "to"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := e.svc.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := tree.Orgs[0].Activity.Activity
	// Only the message two days ago is inside the window; ten days ago and the
	// future are ignored.
	if !slices.Equal(got, []int{0, 0, 0, 0, 1, 0, 0}) {
		t.Errorf("activity = %v", got)
	}
}

func keyFor(repo, identity string) db.GetMailboxByKeyParams {
	return db.GetMailboxByKeyParams{Host: "github.com", Org: "acme", Repo: repo, Identity: identity}
}
