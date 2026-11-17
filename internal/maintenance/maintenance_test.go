package maintenance_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/maintenance"
	"github.com/scrothers/aim/internal/store"
)

var t0 = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

type env struct {
	st    *store.Store
	svc   *mail.Service
	dir   string
	clock atomic.Int64 // seconds past t0
}

func (e *env) now() time.Time { return t0.Add(time.Duration(e.clock.Load()) * time.Second) }

func (e *env) runner(opts ...maintenance.Option) *maintenance.Runner {
	opts = append([]maintenance.Option{maintenance.WithClock(e.now), maintenance.WithDefaultDir(e.dir)}, opts...)
	return maintenance.New(e.st, e.svc, slog.New(slog.NewTextHandler(io.Discard, nil)), opts...)
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "aim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &env{st: st, dir: filepath.Join(t.TempDir(), "backups")}
	e.svc = mail.New(st, nil, mail.WithClock(e.now))
	return e
}

func (e *env) names(t *testing.T) []string {
	t.Helper()
	ents, err := os.ReadDir(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(ents))
	for _, en := range ents {
		out = append(out, en.Name())
	}
	return out
}

func (e *env) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	a := "alice@github.com/acme/web"
	if _, err := e.svc.Register(ctx, mail.Caller{}, mail.RegisterInput{Address: a, Role: "dev", Description: "d"}); err != nil {
		t.Fatal(err)
	}
	c := mail.Caller{SessionID: "s", Identity: address.MustParse(a), SignedOn: true}
	if _, err := e.svc.Send(ctx, c, mail.SendInput{To: []string{"operator"}, Subject: "hello", Body: "hi there"}); err != nil {
		t.Fatal(err)
	}
}

func TestBackupNow(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.seed(t)
	ctx := context.Background()
	res, err := e.runner().BackupNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(e.dir, "aim-20261117T120000Z.db")
	if res.LastPath != want || res.LastSize <= 0 || res.LastSuccessAt == nil || res.LastError != "" {
		t.Fatalf("result = %+v, want path %s", res, want)
	}
	if got, _ := e.svc.BackupStatus(ctx); got.LastPath != want || got.LastSize != res.LastSize {
		t.Errorf("stored status = %+v", got)
	}
	// The backup opens as a database holding the data.
	bst, err := store.Open(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	n, err := bst.Read().CountMessages(ctx)
	if err != nil || n != 1 {
		t.Errorf("backup messages = %d, %v", n, err)
	}
	if err := bst.Close(); err != nil {
		t.Fatal(err)
	}
	// Same second again: refuses to overwrite, records the failure, keeps
	// the earlier success and the file.
	res, err = e.runner().BackupNow(ctx)
	if err == nil || res.LastError == "" || res.LastPath != want {
		t.Errorf("duplicate = %+v, %v", res, err)
	}
	if got := e.names(t); !slices.Equal(got, []string{"aim-20261117T120000Z.db"}) {
		t.Errorf("files = %v", got)
	}
}

func TestBackupErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	r := maintenance.New(e.st, e.svc, slog.New(slog.NewTextHandler(io.Discard, nil)), maintenance.WithClock(e.now))
	st, err := r.BackupNow(ctx)
	if !errors.Is(err, maintenance.ErrNoBackupDir) || st.LastError == "" {
		t.Errorf("no dir: %+v, %v", st, err)
	}
	// A directory path that is a file cannot be created.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetBackups(ctx, mail.BackupSettings{Dir: filepath.Join(file, "sub")}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.BackupNow(ctx); err == nil {
		t.Error("expected error for unusable dir")
	}
	if got, _ := e.svc.BackupStatus(ctx); got.LastError == "" || got.LastSuccessAt != nil {
		t.Errorf("status = %+v", got)
	}
}

func TestRotateOnlyMatchingNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keepers := []string{
		"aim-notes.db", "aim-20260101T000000Z.db.bak", "other.db", "aim-2026010T000000Z.db", "AIM-20260101T000000Z.db",
	}
	backups := make([]string, 0, 5)
	for i := range 5 {
		backups = append(backups, "aim-2026010"+string(rune('1'+i))+"T000000Z.db")
	}
	for _, n := range append(slices.Clone(keepers), backups...) {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "aim-20250101T000000Z.db"), 0o700); err != nil { // a directory with a matching name
		t.Fatal(err)
	}
	link := filepath.Join(dir, "aim-20240101T000000Z.db")
	if err := os.Symlink(filepath.Join(dir, "other.db"), link); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.Rotate(dir, 2); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dir)
	got := make([]string, 0, len(ents))
	for _, e := range ents {
		got = append(got, e.Name())
	}
	want := append(slices.Clone(keepers), "aim-20240101T000000Z.db", "aim-20250101T000000Z.db", "aim-20260104T000000Z.db", "aim-20260105T000000Z.db")
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("after rotate:\n got %v\nwant %v", got, want)
	}
}

func TestTickSchedulesBackups(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	r := e.runner()
	if err := e.svc.SetBackups(ctx, mail.BackupSettings{EveryHours: 6, Keep: 2}); err != nil {
		t.Fatal(err)
	}
	r.Tick(ctx) // first run: due immediately
	if n := len(e.names(t)); n != 1 {
		t.Fatalf("after first tick: %d backups", n)
	}
	e.clock.Add(5 * 3600)
	r.Tick(ctx) // not due yet
	if n := len(e.names(t)); n != 1 {
		t.Fatalf("after 5h: %d backups", n)
	}
	e.clock.Add(3600)
	r.Tick(ctx)
	e.clock.Add(6 * 3600)
	r.Tick(ctx)
	e.clock.Add(6 * 3600)
	r.Tick(ctx)
	got := e.names(t)
	want := []string{"aim-20261118T000000Z.db", "aim-20261118T060000Z.db"}
	if !slices.Equal(got, want) {
		t.Errorf("backups = %v, want %v", got, want)
	}
	st, _ := e.svc.BackupStatus(ctx)
	if st.LastSuccessAt == nil || !st.LastSuccessAt.Equal(e.now()) {
		t.Errorf("status = %+v", st)
	}
}

func TestTickPrunesWhenEnabled(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.seed(t)
	ctx := context.Background()
	r := maintenance.New(e.st, e.svc, slog.New(slog.NewTextHandler(io.Discard, nil)), maintenance.WithClock(e.now)) // no backup dir
	// Archive the operator's copy.
	op := mail.OperatorCaller("ui", "aim-web", "devbox")
	l, err := e.svc.List(ctx, mail.ListInput{Address: "operator", State: mail.StateAll})
	if err != nil || len(l.Items) != 1 {
		t.Fatalf("list = %+v, %v", l, err)
	}
	if _, err := e.svc.Mark(ctx, op, mail.MarkInput{MessageIDs: []string{l.Items[0].ID}, State: mail.MarkArchived}); err != nil {
		t.Fatal(err)
	}
	e.clock.Add(40 * 86400)
	r.Tick(ctx) // retention off
	if s, _ := e.svc.Stats(ctx); s.Messages != 1 {
		t.Fatalf("pruned while off: %d", s.Messages)
	}
	if err := e.svc.SetRetention(ctx, mail.RetentionSettings{PruneArchivedAfterDays: 30}); err != nil {
		t.Fatal(err)
	}
	res, err := r.Prune(ctx, true)
	if err != nil || res.Messages != 1 {
		t.Fatalf("dry run = %+v, %v", res, err)
	}
	r.Tick(ctx)
	if s, _ := e.svc.Stats(ctx); s.Messages != 0 {
		t.Errorf("messages after tick = %d", s.Messages)
	}
	if due, err := r.BackupDue(ctx); due || err != nil {
		t.Errorf("BackupDue without dir = %v, %v", due, err)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := e.runner(maintenance.WithInterval(5 * time.Millisecond))
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	deadline := time.After(10 * time.Second)
	for len(readNames(e.dir)) == 0 {
		select {
		case <-deadline:
			t.Fatal("no backup from Run")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func readNames(dir string) []string {
	ents, _ := os.ReadDir(dir)
	out := make([]string, len(ents))
	for i, e := range ents {
		out[i] = e.Name()
	}
	return out
}

func TestDefaultDir(t *testing.T) {
	t.Parallel()
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := maintenance.DefaultDir(get(map[string]string{"XDG_STATE_HOME": "/x"}), "/h"); got != "/x/aim/backups" {
		t.Errorf("xdg = %q", got)
	}
	if got := maintenance.DefaultDir(get(nil), "/h"); got != "/h/.local/state/aim/backups" {
		t.Errorf("home = %q", got)
	}
}
