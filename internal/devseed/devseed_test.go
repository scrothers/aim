package devseed_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/daemon"
	"github.com/scrothers/aim/internal/devseed"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/tasks"
)

var fixedNow = time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC)

func histCfg(t *testing.T, seed uint64) devseed.HistoryConfig {
	t.Helper()
	return devseed.HistoryConfig{
		DB: filepath.Join(t.TempDir(), "aim.db"), Seed: seed, Days: 3, Messages: 50,
		Now: func() time.Time { return fixedNow },
	}
}

func TestHistoryCounts(t *testing.T) {
	t.Parallel()
	sum, err := devseed.SeedHistory(context.Background(), histCfg(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Identities != 22 || sum.Retired != 2 {
		t.Errorf("identities = %d retired = %d, want 22 and 2", sum.Identities, sum.Retired)
	}
	if sum.Messages != 50 || len(sum.Subjects) != 50 {
		t.Errorf("messages = %d subjects = %d, want 50", sum.Messages, len(sum.Subjects))
	}
	if sum.Threads < 10 || sum.Threads > sum.Messages {
		t.Errorf("threads = %d", sum.Threads)
	}
	if sum.OperatorUnread == 0 {
		t.Error("operator has no unread mail")
	}
}

func TestHistoryDeterministic(t *testing.T) {
	t.Parallel()
	a, err := devseed.SeedHistory(context.Background(), histCfg(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	b, err := devseed.SeedHistory(context.Background(), histCfg(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	c, err := devseed.SeedHistory(context.Background(), histCfg(t, 8))
	if err != nil {
		t.Fatal(err)
	}
	if a.Identities != b.Identities || a.Messages != b.Messages || a.Threads != b.Threads || !slices.Equal(a.Subjects, b.Subjects) {
		t.Error("same seed produced different data")
	}
	if slices.Equal(a.Subjects, c.Subjects) {
		t.Error("different seeds produced identical subjects")
	}
}

func TestHistoryBackdated(t *testing.T) {
	t.Parallel()
	sum, err := devseed.SeedHistory(context.Background(), histCfg(t, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Oldest.Before(fixedNow.Add(-2 * 24 * time.Hour)) {
		t.Errorf("oldest %v is not spread over the window", sum.Oldest)
	}
	if sum.Oldest.Before(fixedNow.Add(-4 * 24 * time.Hour)) {
		t.Errorf("oldest %v is outside the window", sum.Oldest)
	}
	if sum.Newest.After(fixedNow) {
		t.Errorf("newest %v is in the future", sum.Newest)
	}
}

func TestHistoryRefusesTwiceUnlessForced(t *testing.T) {
	t.Parallel()
	cfg := histCfg(t, 1)
	if _, err := devseed.SeedHistory(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := devseed.SeedHistory(context.Background(), cfg); !errors.Is(err, devseed.ErrHistoryExists) {
		t.Fatalf("second run error = %v, want ErrHistoryExists", err)
	}
	cfg.Force = true
	sum, err := devseed.SeedHistory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("forced run: %v", err)
	}
	if sum.Identities != 0 || sum.Existing != 22 || sum.Messages != 50 {
		t.Errorf("forced run = %+v", sum)
	}
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test against a local listener
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func TestLive(t *testing.T) {
	dir := t.TempDir()
	cfg := daemon.Config{DBPath: filepath.Join(dir, "aim.db"), Socket: filepath.Join(dir, "s"), HTTP: "127.0.0.1:0"}
	ready := make(chan daemon.Addrs, 1)
	cfg.Ready = func(a daemon.Addrs) { ready <- a }
	dctx, dcancel := context.WithCancel(context.Background())
	ddone := make(chan error, 1)
	go func() { ddone <- daemon.Run(dctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	t.Cleanup(func() { dcancel(); <-ddone })
	var addrs daemon.Addrs
	select {
	case addrs = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("daemon not ready")
	}
	base := "http://" + addrs.HTTP

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	online := make(chan []devseed.Online, 1)
	done := make(chan error, 1)
	go func() {
		done <- devseed.RunLive(ctx, devseed.LiveConfig{
			Addr: addrs.Socket, Seed: 1, Chatter: 20 * time.Millisecond,
			OnReady: func(o []devseed.Online) { online <- o },
		})
	}()
	select {
	case o := <-online:
		if len(o) != 12 {
			t.Fatalf("online = %d, want 12", len(o))
		}
	case err := <-done:
		t.Fatalf("RunLive exited early: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("agents not online")
	}

	var pres struct {
		Groups []struct {
			Offline bool `json:"offline"`
			Buddies []struct {
				Client  string `json:"client"`
				Machine string `json:"machine"`
				Online  bool   `json:"online"`
			} `json:"buddies"`
		} `json:"groups"`
	}
	getJSON(t, base+"/api/presence", &pres)
	n := 0
	for _, g := range pres.Groups {
		for _, b := range g.Buddies {
			if b.Online {
				n++
				if b.Client == "" || b.Machine == "" {
					t.Errorf("online buddy lacks client or machine: %+v", b)
				}
			}
		}
	}
	if n < 8 { // chatter may sign a few off already
		t.Errorf("online buddies = %d, want about 12", n)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		var stats struct {
			Messages int `json:"messages"`
		}
		getJSON(t, base+"/api/stats", &stats)
		if stats.Messages > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no chatter message was sent")
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunLive: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("RunLive did not stop")
	}
}

func boardCfg(t *testing.T, seed uint64) devseed.HistoryConfig {
	t.Helper()
	cfg := histCfg(t, seed)
	cfg.Days, cfg.Messages = 7, 250
	return cfg
}

// readBoard loads every task with its events and links from a seeded database.
func readBoard(t *testing.T, dbPath string, now time.Time) []tasks.Detail {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	svc := tasks.New(st, mail.New(st, nil), tasks.WithClock(func() time.Time { return now }))
	res, err := svc.List(ctx, tasks.ListInput{Limit: tasks.MaxListLimit})
	if err != nil {
		t.Fatal(err)
	}
	var out []tasks.Detail
	for _, tk := range res.Items {
		d, err := svc.Get(ctx, tk.ID)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b tasks.Detail) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func TestHistoryTaskBoard(t *testing.T) {
	t.Parallel()
	cfg := boardCfg(t, 1)
	sum, err := devseed.SeedHistory(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	b := sum.Board
	if b.Tasks < 25 || b.Overdue < 2 || b.Links == 0 {
		t.Errorf("board = %+v, want 25+ tasks, 2+ overdue and some links", b)
	}
	for _, st := range tasks.Statuses {
		if b.Status[st] == 0 {
			t.Errorf("no task is %s", st)
		}
	}
	for _, h := range tasks.Healths {
		if b.Health[h] == 0 {
			t.Errorf("no held task is %s", h)
		}
	}

	details := readBoard(t, cfg.DB, fixedNow)
	if len(details) != b.Tasks {
		t.Fatalf("read %d tasks, summary says %d", len(details), b.Tasks)
	}
	var assigned, due int
	for _, d := range details {
		if !strings.HasPrefix(d.ID, "task-") || d.Title == "" || d.Body == "" {
			t.Errorf("malformed task %+v", d.Task)
		}
		if d.Status == tasks.StatusNotStarted && d.Assignee != "" {
			assigned++
		}
		if d.DueAt != nil {
			due++
		}
		if len(d.Events) == 0 || d.Events[0].Kind != tasks.EventCreated {
			t.Errorf("%s: events do not start with created: %+v", d.ID, d.Events)
		}
		for i := 1; i < len(d.Events); i++ {
			if d.Events[i].At.Before(d.Events[i-1].At) {
				t.Errorf("%s: event %d (%s %s) precedes event %d (%s %s)", d.ID, i, d.Events[i].Kind, d.Events[i].At, i-1, d.Events[i-1].Kind, d.Events[i-1].At)
			}
		}
		if d.Status == tasks.StatusAbandoned && !slices.ContainsFunc(d.Events, func(e tasks.Event) bool { return e.Kind == tasks.EventDecayed }) {
			t.Errorf("%s: abandoned without a decayed event", d.ID)
		}
		if d.Status == tasks.StatusFinished && (d.FinishedAt == nil || d.Events[len(d.Events)-1].Kind != string(tasks.ActionFinish)) {
			t.Errorf("%s: finished task does not end with finish", d.ID)
		}
		if (d.Status == tasks.StatusClaimed || d.Status == tasks.StatusStarted) && (d.Claimant == "" || d.LastActivityAt == nil || d.Health == "") {
			t.Errorf("%s: held task lacks claimant, activity or health", d.ID)
		}
		for _, l := range d.Links {
			if l.MessageID == "" || l.ThreadID == "" || l.Subject == "" || l.LinkedAt.Before(d.CreatedAt) {
				t.Errorf("%s: bad link %+v", d.ID, l)
			}
		}
		if d.CreatedAt.After(fixedNow) || d.UpdatedAt.After(fixedNow) {
			t.Errorf("%s is dated in the future", d.ID)
		}
	}
	if assigned < 3 || due < 5 {
		t.Errorf("assigned-not-claimed = %d, with due dates = %d", assigned, due)
	}
}

func TestHistoryTaskBoardDeterministic(t *testing.T) {
	t.Parallel()
	a, b := boardCfg(t, 5), boardCfg(t, 5)
	for _, cfg := range []devseed.HistoryConfig{a, b} {
		if _, err := devseed.SeedHistory(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
	}
	shape := func(cfg devseed.HistoryConfig) []string {
		var out []string
		for _, d := range readBoard(t, cfg.DB, fixedNow) {
			out = append(out, fmt.Sprintf("%s|%s|%s|%s|%d|%d", d.ID, d.Title, d.Status, d.Priority, len(d.Events), len(d.Links)))
		}
		return out
	}
	if sa, sb := shape(a), shape(b); !slices.Equal(sa, sb) {
		t.Errorf("same seed produced different boards:\n%v\n%v", sa, sb)
	}
}

func TestHistoryTaskBoardNotDuplicated(t *testing.T) {
	t.Parallel()
	cfg := boardCfg(t, 1)
	first, err := devseed.SeedHistory(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Force = true
	again, err := devseed.SeedHistory(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if again.Board.Tasks != 0 || again.Board.Existing != first.Board.Tasks {
		t.Errorf("forced rerun board = %+v, want %d existing and none added", again.Board, first.Board.Tasks)
	}
	if n := len(readBoard(t, cfg.DB, fixedNow)); n != first.Board.Tasks {
		t.Errorf("board has %d tasks after a forced rerun, want %d", n, first.Board.Tasks)
	}
}

func TestHistoryApprovalsFlag(t *testing.T) {
	t.Parallel()
	enabled := func(cfg devseed.HistoryConfig) bool {
		st, err := store.Open(context.Background(), cfg.DB)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = st.Close() }()
		a, err := mail.New(st, nil).ApprovalSettings(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return a.Enabled
	}
	off := histCfg(t, 1)
	if _, err := devseed.SeedHistory(context.Background(), off); err != nil {
		t.Fatal(err)
	}
	if enabled(off) {
		t.Error("approvals enabled without --approvals")
	}
	on := histCfg(t, 1)
	on.Approvals = true
	sum, err := devseed.SeedHistory(context.Background(), on)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled(on) || !sum.ApprovalsEnabled {
		t.Error("approvals not enabled by --approvals")
	}
}
