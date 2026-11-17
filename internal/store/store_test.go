package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/store/db"
)

var templatePath string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "aim-store-template-")
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

func newStore(t *testing.T) *store.Store {
	s, _ := newStorePath(t)
	return s
}

func newStorePath(t *testing.T) (*store.Store, string) {
	t.Helper()
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "aim.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

type mb struct{ id, repo, identity, subagent, kind string }

// seed creates mailboxes under host h / org acme:
//
//	alice@acme (org-level identity, repo ''), alice@acme/web (repo identity),
//	bob@acme/web, alice@acme/web subagent "helper".
func seed(t *testing.T, s *store.Store) {
	t.Helper()
	boxes := []mb{
		{"mbx-1", "", "alice", "", "identity"},
		{"mbx-2", "web", "alice", "", "identity"},
		{"mbx-3", "web", "bob", "", "identity"},
		{"mbx-4", "web", "alice", "helper", "subagent"},
	}
	err := s.WithTx(context.Background(), func(q *db.Queries) error {
		for _, b := range boxes {
			if err := q.InsertMailbox(context.Background(), db.InsertMailboxParams{
				ID: b.id, Host: "h", Org: "acme", Repo: b.repo, Identity: b.identity,
				Subagent: b.subagent, Kind: b.kind, CreatedAt: 1,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func send(t *testing.T, s *store.Store, id, thread, subject, body string, at int64, to ...string) {
	t.Helper()
	ctx := context.Background()
	err := s.WithTx(ctx, func(q *db.Queries) error {
		if err := q.InsertMessage(ctx, db.InsertMessageParams{
			ID: id, ThreadID: thread, FromMailbox: "mbx-3", Subject: subject, Body: body, CreatedAt: at,
		}); err != nil {
			return err
		}
		for _, m := range to {
			if err := q.InsertDelivery(ctx, db.InsertDeliveryParams{MessageID: id, MailboxID: m, Kind: "to"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func inbox(t *testing.T, s *store.Store, p store.InboxParams) []string {
	t.Helper()
	if p.PageLimit == 0 {
		p.PageLimit = 100
	}
	if p.State == "" {
		p.State = "all"
	}
	p.Host, p.Org = "h", "acme"
	rows, err := store.ListInbox(context.Background(), s.Read(), p)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.MessageID)
	}
	return ids
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestOpenIdempotentAndPing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "dir", "aim.db")
	for range 2 {
		s, err := store.Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Ping(ctx); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	fi, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", fi.Mode().Perm())
	}
}

func TestOpenBadPath(t *testing.T) {
	t.Parallel()
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), filepath.Join(f, "sub", "aim.db")); err == nil {
		t.Fatal("expected error")
	}
}

func TestReaderIsReadOnly(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	err := s.Read().InsertMailbox(context.Background(), db.InsertMailboxParams{ID: "x", Kind: "identity"})
	if err == nil {
		t.Fatal("reader accepted a write")
	}
}

func TestWithTxRollback(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	boom := errors.New("boom")
	err := s.WithTx(ctx, func(q *db.Queries) error {
		if err := q.InsertMailbox(ctx, db.InsertMailboxParams{ID: "mbx-x", Host: "h", Org: "o", Kind: "identity"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.Read().GetMailboxByID(ctx, "mbx-x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected rollback, got %v", err)
	}
}

func TestMailboxQueries(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	seed(t, s)
	ctx := context.Background()
	q := s.Read()

	m, err := q.GetMailboxByKey(ctx, db.GetMailboxByKeyParams{Host: "h", Org: "acme", Repo: "web", Identity: "alice"})
	if err != nil || m.ID != "mbx-2" {
		t.Fatalf("by key: %v %v", m.ID, err)
	}
	if _, err := q.GetMailboxByID(ctx, "mbx-3"); err != nil {
		t.Fatal(err)
	}

	// Unique constraint.
	err = s.WithTx(ctx, func(w *db.Queries) error {
		return w.InsertMailbox(ctx, db.InsertMailboxParams{ID: "mbx-dup", Host: "h", Org: "acme", Repo: "web", Identity: "alice", Kind: "identity"})
	})
	if err == nil {
		t.Fatal("expected unique violation")
	}
	// Check constraint.
	err = s.WithTx(ctx, func(w *db.Queries) error {
		return w.InsertMailbox(ctx, db.InsertMailboxParams{ID: "mbx-bad", Host: "h", Org: "acme", Identity: "z", Kind: "bogus"})
	})
	if err == nil {
		t.Fatal("expected check violation")
	}

	ids, err := store.ListIdentitiesInScope(ctx, q, store.Scope{Host: "h", Org: "acme"})
	if err != nil || len(ids) != 3 {
		t.Fatalf("org scope: %d %v", len(ids), err)
	}
	if ids[0].ID != "mbx-1" { // repo '' sorts first
		t.Fatalf("order: %v", ids[0].ID)
	}
	ids, _ = store.ListIdentitiesInScope(ctx, q, store.Scope{Host: "h", Org: "acme", MatchRepo: true, Repo: "web"})
	if len(ids) != 2 {
		t.Fatalf("repo scope: %d", len(ids))
	}
	ids, _ = store.ListIdentitiesInScope(ctx, q, store.Scope{Host: "h", Org: "acme", MatchRepo: true, MatchIdentity: true, Identity: "alice"})
	if len(ids) != 1 || ids[0].ID != "mbx-1" {
		t.Fatalf("identity scope repo '': %v", ids)
	}

	subs, err := q.ListSubagentsOf(ctx, db.ListSubagentsOfParams{Host: "h", Org: "acme", Repo: "web", Identity: "alice"})
	if err != nil || len(subs) != 1 || subs[0].Subagent != "helper" {
		t.Fatalf("subagents: %v %v", subs, err)
	}
	byIDs, err := q.ListMailboxesByIDs(ctx, []string{"mbx-1", "mbx-3", "nope"})
	if err != nil || len(byIDs) != 2 {
		t.Fatalf("by ids: %v %v", byIDs, err)
	}
}

func TestInboxScopes(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	seed(t, s)
	send(t, s, "msg-1", "msg-1", "a", "b", 10, "mbx-1")
	send(t, s, "msg-2", "msg-2", "a", "b", 20, "mbx-2")
	send(t, s, "msg-3", "msg-3", "a", "b", 30, "mbx-3")
	send(t, s, "msg-4", "msg-4", "a", "b", 40, "mbx-4")

	eq(t, inbox(t, s, store.InboxParams{}), []string{"msg-4", "msg-3", "msg-2", "msg-1"})
	eq(t, inbox(t, s, store.InboxParams{Scope: store.Scope{MatchRepo: true, Repo: "web"}}), []string{"msg-4", "msg-3", "msg-2"})
	eq(t, inbox(t, s, store.InboxParams{Scope: store.Scope{MatchRepo: true, Repo: "web", MatchIdentity: true, Identity: "alice"}}), []string{"msg-4", "msg-2"})
	// identity scope with repo '' excludes repo identities of the same name.
	eq(t, inbox(t, s, store.InboxParams{Scope: store.Scope{MatchRepo: true, MatchIdentity: true, Identity: "alice", MatchSubagent: true}}), []string{"msg-1"})
	eq(t, inbox(t, s, store.InboxParams{Scope: store.Scope{MatchRepo: true, Repo: "web", MatchIdentity: true, Identity: "alice", MatchSubagent: true, Subagent: "helper"}}), []string{"msg-4"})
	eq(t, inbox(t, s, store.InboxParams{Scope: store.Scope{MatchRepo: true, Repo: "web", MatchIdentity: true, Identity: "alice", MatchSubagent: true}}), []string{"msg-2"})

	// Other host sees nothing.
	rows, err := store.ListInbox(context.Background(), s.Read(), store.InboxParams{Scope: store.Scope{Host: "other", Org: "acme"}, State: "all", PageLimit: 10})
	if err != nil || len(rows) != 0 {
		t.Fatalf("other host: %v %v", rows, err)
	}
}

func TestInboxFiltersStatesPagination(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	seed(t, s)
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		send(t, s, fmt.Sprintf("msg-%d", i), fmt.Sprintf("msg-%d", i), "s", "b", int64(i*10), "mbx-1")
	}
	// msg-1 read, msg-2 archived, msg-3 read+archived.
	err := s.WithTx(ctx, func(q *db.Queries) error {
		for _, step := range []func() (int64, error){
			func() (int64, error) {
				return q.SetReadAt(ctx, db.SetReadAtParams{Now: sql.NullInt64{Int64: 5, Valid: true}, MessageID: "msg-1", MailboxID: "mbx-1"})
			},
			func() (int64, error) {
				return q.SetArchivedAt(ctx, db.SetArchivedAtParams{Now: sql.NullInt64{Int64: 5, Valid: true}, MessageID: "msg-2", MailboxID: "mbx-1"})
			},
			func() (int64, error) {
				return q.SetReadAt(ctx, db.SetReadAtParams{Now: sql.NullInt64{Int64: 5, Valid: true}, MessageID: "msg-3", MailboxID: "mbx-1"})
			},
			func() (int64, error) {
				return q.SetArchivedAt(ctx, db.SetArchivedAtParams{Now: sql.NullInt64{Int64: 5, Valid: true}, MessageID: "msg-3", MailboxID: "mbx-1"})
			},
		} {
			n, err := step()
			if err != nil || n != 1 {
				return fmt.Errorf("step: %d %w", n, err)
			}
		}
		n, err := q.SetReadAt(ctx, db.SetReadAtParams{MessageID: "nope", MailboxID: "mbx-1"})
		if err != nil || n != 0 {
			return fmt.Errorf("missing delivery: %d %w", n, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, inbox(t, s, store.InboxParams{State: "unread"}), []string{"msg-5", "msg-4"})
	eq(t, inbox(t, s, store.InboxParams{State: "read"}), []string{"msg-1"})
	eq(t, inbox(t, s, store.InboxParams{State: "archived"}), []string{"msg-3", "msg-2"})
	eq(t, inbox(t, s, store.InboxParams{State: "inbox"}), []string{"msg-5", "msg-4", "msg-1"})
	eq(t, inbox(t, s, store.InboxParams{State: "all"}), []string{"msg-5", "msg-4", "msg-3", "msg-2", "msg-1"})

	// Clear queries restore state.
	err = s.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.ClearReadAt(ctx, db.ClearReadAtParams{MessageID: "msg-1", MailboxID: "mbx-1"}); err != nil {
			return err
		}
		_, err := q.ClearArchivedAt(ctx, db.ClearArchivedAtParams{MessageID: "msg-2", MailboxID: "mbx-1"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, inbox(t, s, store.InboxParams{State: "unread"}), []string{"msg-5", "msg-4", "msg-2", "msg-1"})

	// Filters.
	eq(t, inbox(t, s, store.InboxParams{Since: sql.NullInt64{Int64: 20, Valid: true}, Until: sql.NullInt64{Int64: 40, Valid: true}}), []string{"msg-4", "msg-3", "msg-2"})
	eq(t, inbox(t, s, store.InboxParams{FromMailbox: sql.NullString{String: "mbx-3", Valid: true}, PageLimit: 2}), []string{"msg-5", "msg-4"})
	eq(t, inbox(t, s, store.InboxParams{FromMailbox: sql.NullString{String: "mbx-9", Valid: true}}), nil)

	// Keyset pagination.
	eq(t, inbox(t, s, store.InboxParams{PageLimit: 2}), []string{"msg-5", "msg-4"})
	eq(t, inbox(t, s, store.InboxParams{PageLimit: 2, BeforeMessage: sql.NullString{String: "msg-4", Valid: true}, BeforeMailbox: sql.NullString{String: "mbx-0", Valid: true}}), []string{"msg-3", "msg-2"})
	eq(t, inbox(t, s, store.InboxParams{PageLimit: 2, BeforeMessage: sql.NullString{String: "msg-2", Valid: true}, BeforeMailbox: sql.NullString{String: "mbx-0", Valid: true}}), []string{"msg-1"})
}

func TestMessageThreadAndDeliveries(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	seed(t, s)
	ctx := context.Background()
	send(t, s, "msg-1", "msg-1", "hello", "body", 10, "mbx-1")
	err := s.WithTx(ctx, func(q *db.Queries) error {
		if err := q.InsertMessage(ctx, db.InsertMessageParams{
			ID: "msg-2", ThreadID: "msg-1", InReplyTo: sql.NullString{String: "msg-1", Valid: true},
			FromMailbox: "mbx-1", Subject: "Re: hello", Body: "reply", CreatedAt: 20,
		}); err != nil {
			return err
		}
		if err := q.InsertDelivery(ctx, db.InsertDeliveryParams{MessageID: "msg-2", MailboxID: "mbx-3", Kind: "to"}); err != nil {
			return err
		}
		return q.InsertDelivery(ctx, db.InsertDeliveryParams{MessageID: "msg-2", MailboxID: "mbx-2", Kind: "cc"})
	})
	if err != nil {
		t.Fatal(err)
	}
	q := s.Read()
	m, err := q.GetMessageByID(ctx, "msg-2")
	if err != nil || m.Body != "reply" || m.FromIdentity != "alice" || !m.InReplyTo.Valid {
		t.Fatalf("get: %+v %v", m, err)
	}
	if _, err := q.GetMessageByID(ctx, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("got %v", err)
	}
	th, err := q.ListThread(ctx, "msg-1")
	if err != nil || len(th) != 2 || th[0].ID != "msg-1" || th[1].ID != "msg-2" {
		t.Fatalf("thread: %v %v", th, err)
	}
	ds, err := q.ListDeliveriesForMessage(ctx, "msg-2")
	if err != nil || len(ds) != 2 || ds[0].Kind != "to" || ds[1].Kind != "cc" {
		t.Fatalf("deliveries: %+v %v", ds, err)
	}
	// Foreign keys enforced.
	err = s.WithTx(ctx, func(w *db.Queries) error {
		return w.InsertDelivery(ctx, db.InsertDeliveryParams{MessageID: "msg-1", MailboxID: "ghost", Kind: "to"})
	})
	if err == nil {
		t.Fatal("expected FK violation")
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	seed(t, s)
	ctx := context.Background()
	send(t, s, "msg-1", "msg-1", "deploy plan", "the quick brown fox", 10, "mbx-1", "mbx-2")
	send(t, s, "msg-2", "msg-2", "lunch", "foxes love deploy scripts", 20, "mbx-3")
	send(t, s, "msg-3", "msg-3", "other", "nothing relevant", 30, "mbx-2")

	base := store.SearchParams{Host: "h", Org: "acme", Query: "deploy"}
	hits, err := s.Search(ctx, base)
	if err != nil || len(hits) != 2 {
		t.Fatalf("org search: %v %v", hits, err)
	}
	// msg-1 has two in-scope deliveries but yields one hit.
	seen := map[string]int{}
	for _, h := range hits {
		seen[h.MessageID]++
		if h.Snippet == "" || h.ThreadID == "" || h.FromMailboxID != "mbx-3" {
			t.Fatalf("hit fields: %+v", h)
		}
	}
	if seen["msg-1"] != 1 || seen["msg-2"] != 1 {
		t.Fatalf("dedupe: %v", seen)
	}

	// Scope: identity alice with repo '' only sees msg-1 via mbx-1.
	p := base
	p.MatchRepo, p.MatchIdentity, p.Identity = true, true, "alice"
	hits, err = s.Search(ctx, p)
	if err != nil || len(hits) != 1 || hits[0].MailboxID != "mbx-1" {
		t.Fatalf("identity scope: %+v %v", hits, err)
	}
	// Repo scope web: msg-1 (mbx-2) and msg-2 (mbx-3).
	p = base
	p.MatchRepo, p.Repo = true, "web"
	hits, _ = s.Search(ctx, p)
	if len(hits) != 2 {
		t.Fatalf("repo scope: %+v", hits)
	}
	// Subagent scope matches nothing.
	p.MatchIdentity, p.Identity, p.MatchSubagent, p.Subagent = true, "alice", true, "helper"
	hits, _ = s.Search(ctx, p)
	if len(hits) != 0 {
		t.Fatalf("subagent scope: %+v", hits)
	}

	// Time filter, limit, offset.
	since, until := int64(15), int64(25)
	p = base
	p.Since, p.Until = &since, &until
	hits, _ = s.Search(ctx, p)
	if len(hits) != 1 || hits[0].MessageID != "msg-2" {
		t.Fatalf("time filter: %+v", hits)
	}
	p = base
	p.Limit, p.Offset = 1, 1
	hits, _ = s.Search(ctx, p)
	if len(hits) != 1 {
		t.Fatalf("offset: %+v", hits)
	}
	p.Offset = 5
	if hits, _ = s.Search(ctx, p); len(hits) != 0 {
		t.Fatalf("offset past end: %+v", hits)
	}
}

func TestSearchBadQuery(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	seed(t, s)
	for _, q := range []string{"", "   ", `"unterminated`, "foo AND", "nosuchcol:foo", "(", "a OR OR b", "foo*bar("} {
		_, err := s.Search(context.Background(), store.SearchParams{Host: "h", Org: "acme", Query: q})
		if !errors.Is(err, store.ErrBadQuery) {
			t.Errorf("query %q: got %v, want ErrBadQuery", q, err)
		}
	}
	// Injection-looking input is just text or a syntax error.
	_, err := s.Search(context.Background(), store.SearchParams{Host: "h", Org: "acme", Query: "x'; DROP TABLE messages; --"})
	if err != nil && !errors.Is(err, store.ErrBadQuery) {
		t.Fatalf("got %v", err)
	}
}

func TestSearchCanceled(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Search(ctx, store.SearchParams{Host: "h", Org: "acme", Query: "x"})
	if err == nil || errors.Is(err, store.ErrBadQuery) {
		t.Fatalf("got %v", err)
	}
}

func TestFTSTriggers(t *testing.T) {
	t.Parallel()
	s, path := newStorePath(t)
	seed(t, s)
	ctx := context.Background()
	send(t, s, "msg-1", "msg-1", "alpha", "beta", 10, "mbx-1")
	count := func(q string) int {
		hits, err := s.Search(ctx, store.SearchParams{Host: "h", Org: "acme", Query: q})
		if err != nil {
			t.Fatal(err)
		}
		return len(hits)
	}
	if count("alpha") != 1 {
		t.Fatal("insert not indexed")
	}
	// UPDATE/DELETE on messages have no sqlc query, so use a raw handle.
	raw := rawHandle(t, path)
	if _, err := raw.ExecContext(ctx, `UPDATE messages SET subject='gamma' WHERE id='msg-1'`); err != nil {
		t.Fatal(err)
	}
	if count("alpha") != 0 || count("gamma") != 1 {
		t.Fatal("update not synced")
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM deliveries WHERE message_id='msg-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM messages WHERE id='msg-1'`); err != nil {
		t.Fatal(err)
	}
	if count("gamma") != 0 || count("beta") != 0 {
		t.Fatal("delete not synced")
	}
}

func rawHandle(t *testing.T, path string) *sql.DB {
	t.Helper()
	h, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func TestClosedStoreErrors(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Ping(ctx); err == nil {
		t.Error("ping after close")
	}
	if err := s.WithTx(ctx, func(*db.Queries) error { return nil }); err == nil {
		t.Error("tx after close")
	}
	if err := s.Close(); err != nil {
		t.Logf("second close: %v", err)
	}
}

func TestOpenCorruptFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aim.db")
	if err := os.WriteFile(path, []byte("this is definitely not a sqlite database file at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), path); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenDirectoryAsDB(t *testing.T) {
	t.Parallel()
	if _, err := store.Open(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Open(ctx, filepath.Join(t.TempDir(), "aim.db")); err == nil {
		t.Fatal("expected error")
	}
}

func TestPathWithSpecialChars(t *testing.T) {
	t.Parallel()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "we?ird#na%me.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}

func TestOpenUnknownMigrationVersion(t *testing.T) {
	t.Parallel()
	s, path := newStorePath(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	h := rawHandle(t, path)
	if _, err := h.ExecContext(context.Background(), `UPDATE schema_migrations SET version = 99`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), path); err == nil {
		t.Fatal("expected migrate error")
	}
}

func TestInboxCompositeCursor(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	seed(t, s)
	// One message delivered to two mailboxes, then an older one.
	send(t, s, "msg-1", "msg-1", "a", "b", 10, "mbx-1")
	send(t, s, "msg-2", "msg-2", "a", "b", 20, "mbx-1", "mbx-2")
	page := func(p store.InboxParams) []string {
		t.Helper()
		p.PageLimit = 1
		p.Host, p.Org, p.State = "h", "acme", "all"
		rows, err := store.ListInbox(context.Background(), s.Read(), p)
		if err != nil || len(rows) != 1 {
			t.Fatalf("page: %v %v", rows, err)
		}
		return []string{rows[0].MessageID, rows[0].MailboxID}
	}
	got := page(store.InboxParams{})
	eq(t, got, []string{"msg-2", "mbx-2"})
	got = page(store.InboxParams{BeforeMessage: sql.NullString{String: got[0], Valid: true}, BeforeMailbox: sql.NullString{String: got[1], Valid: true}})
	eq(t, got, []string{"msg-2", "mbx-1"})
	got = page(store.InboxParams{BeforeMessage: sql.NullString{String: got[0], Valid: true}, BeforeMailbox: sql.NullString{String: got[1], Valid: true}})
	eq(t, got, []string{"msg-1", "mbx-1"})
}

// planDB records the EXPLAIN QUERY PLAN output of every query it runs.
type planDB struct {
	db.DBTX
	plan []string
}

func (p *planDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rows, err := p.DBTX.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return nil, err
		}
		p.plan = append(p.plan, detail)
	}
	return p.DBTX.QueryContext(ctx, "SELECT 1 WHERE 0")
}

func TestInboxUsesMailboxIndex(t *testing.T) {
	t.Parallel()
	_, path := newStorePath(t)
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	tests := []struct {
		name string
		sc   store.Scope
		want string
	}{
		{"org", store.Scope{Host: "h", Org: "acme"}, "host=? AND org=?"},
		{"repo", store.Scope{Host: "h", Org: "acme", Repo: "web", MatchRepo: true}, "repo=?"},
		{"identity", store.Scope{Host: "h", Org: "acme", Repo: "web", Identity: "alice", MatchRepo: true, MatchIdentity: true}, "identity=?"},
		{"subagent", store.Scope{Host: "h", Org: "acme", Repo: "web", Identity: "alice", Subagent: "helper", MatchRepo: true, MatchIdentity: true, MatchSubagent: true}, "subagent=?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &planDB{DBTX: raw}
			_, err := store.ListInbox(context.Background(), db.New(pd), store.InboxParams{Scope: tt.sc, State: "all", PageLimit: 10})
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(pd.plan, "\n")
			if !strings.Contains(joined, tt.want) || strings.Contains(joined, "SCAN mb") {
				t.Errorf("plan lacks %q or scans mailboxes:\n%s", tt.want, joined)
			}
		})
	}
}

func TestMigration0003PreservesData(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	h := rawHandle(t, path)
	for _, f := range []string{"0001_init.up.sql", "0002_fts.up.sql"} {
		b, err := os.ReadFile(filepath.Join("migrations", f))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.ExecContext(ctx, string(b)); err != nil {
			t.Fatal(err)
		}
	}
	stmts := []string{
		`CREATE TABLE schema_migrations (version uint64, dirty bool)`,
		`CREATE UNIQUE INDEX version_unique ON schema_migrations (version)`,
		`INSERT INTO schema_migrations (version, dirty) VALUES (2, 0)`,
		`INSERT INTO mailboxes (id, host, org, repo, identity, subagent, kind, role, reports_to, created_at) VALUES
		   ('mbx-a','h','acme','web','alice','','identity','dev','operator',1),
		   ('mbx-b','h','acme','web','bob','','identity','qa','',2)`,
		`INSERT INTO messages (id, thread_id, from_mailbox, subject, body, created_at) VALUES ('msg-1','msg-1','mbx-a','hello','world',5)`,
		`INSERT INTO deliveries (message_id, mailbox_id, kind) VALUES ('msg-1','mbx-b','to')`,
	}
	for _, q := range stmts {
		if _, err := h.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	q := s.Read()
	bob, err := q.GetMailboxByID(ctx, "mbx-b")
	if err != nil || bob.Role != "qa" || bob.RetiredAt.Valid {
		t.Fatalf("bob = %+v, err %v", bob, err)
	}
	alice, err := q.GetMailboxByID(ctx, "mbx-a")
	if err != nil || alice.ReportsTo != "operator" {
		t.Fatalf("alice = %+v, err %v", alice, err)
	}
	m, err := q.GetMessageByID(ctx, "msg-1")
	if err != nil || m.Body != "world" || m.FromClient != "" || m.FromMachine != "" {
		t.Fatalf("msg = %+v, err %v", m, err)
	}
	rows, err := store.ListInbox(ctx, q, store.InboxParams{Scope: store.Scope{All: true}, State: "all", PageLimit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatalf("inbox = %v, err %v", rows, err)
	}

	// Foreign keys are intact and enforced again after the rebuild.
	h2 := rawHandle(t, path)
	fk, err := h2.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if fk.Next() {
		t.Error("foreign_key_check reported violations")
	}
	_ = fk.Close()
	err = s.WithTx(ctx, func(q *db.Queries) error {
		return q.InsertDelivery(ctx, db.InsertDeliveryParams{MessageID: "msg-1", MailboxID: "mbx-missing", Kind: "to"})
	})
	if err == nil {
		t.Error("foreign keys not enforced after migration")
	}

	// The widened CHECK accepts the operator kind.
	err = s.WithTx(ctx, func(q *db.Queries) error {
		return q.InsertMailbox(ctx, db.InsertMailboxParams{ID: "mbx-op", Identity: "operator", Kind: "operator", CreatedAt: 9})
	})
	if err != nil {
		t.Fatalf("insert operator: %v", err)
	}
}

func TestMigration0003DownRemovesOperatorMail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "new.db")
	h := rawHandle(t, path)
	h.SetMaxOpenConns(1)
	run := func(files ...string) {
		t.Helper()
		for _, f := range files {
			b, err := os.ReadFile(filepath.Join("migrations", f))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.ExecContext(ctx, string(b)); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
		}
	}
	run("0001_init.up.sql", "0002_fts.up.sql", "0003_operator_ui.up.sql")
	for _, q := range []string{
		`INSERT INTO mailboxes (id, host, org, identity, kind, created_at) VALUES
		   ('mbx-a','h','acme','alice','identity',1), ('mbx-op','','','operator','operator',2)`,
		`INSERT INTO messages (id, thread_id, from_mailbox, subject, body, created_at, from_client, from_machine) VALUES
		   ('msg-op','msg-op','mbx-op','zebra','stripes',5,'aim-web','devbox'),
		   ('msg-a','msg-op','mbx-a','re','reply',6,'','')`,
		`UPDATE messages SET in_reply_to = 'msg-op' WHERE id = 'msg-a'`,
		`INSERT INTO deliveries (message_id, mailbox_id, kind) VALUES ('msg-op','mbx-a','to'), ('msg-a','mbx-op','to')`,
	} {
		if _, err := h.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	run("0003_operator_ui.down.sql")

	var msgs, dels, boxes, fts int
	for q, dst := range map[string]*int{
		`SELECT COUNT(*) FROM messages`:                                      &msgs,
		`SELECT COUNT(*) FROM deliveries`:                                    &dels,
		`SELECT COUNT(*) FROM mailboxes`:                                     &boxes,
		`SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'zebra'`: &fts,
	} {
		if err := h.QueryRowContext(ctx, q).Scan(dst); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if msgs != 1 || dels != 0 || boxes != 1 || fts != 0 {
		t.Errorf("messages=%d deliveries=%d mailboxes=%d fts=%d, want 1 0 1 0", msgs, dels, boxes, fts)
	}
	fk, err := h.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if fk.Next() {
		t.Error("foreign_key_check reported violations after down")
	}
	_ = fk.Close()

	// Up again works on the downgraded database.
	run("0003_operator_ui.up.sql")
}

func TestAggregatesAndRetire(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStore(t)
	seed(t, s)
	send(t, s, "msg-1", "msg-1", "s", "b", 86400000*3+5, "mbx-1", "mbx-2")
	send(t, s, "msg-2", "msg-2", "s", "b", 86400000*4+5, "mbx-2")
	q := s.Read()

	act, err := q.MailboxActivity(ctx)
	if err != nil || len(act) != 2 {
		t.Fatalf("activity = %v, %v", act, err)
	}
	for _, a := range act {
		if a.MailboxID == "mbx-2" && (a.Unread != 2 || a.Total != 2 || a.LastReceived != 86400000*4+5) {
			t.Errorf("mbx-2 activity = %+v", a)
		}
	}
	sent, err := q.MailboxLastSent(ctx)
	if err != nil || len(sent) != 1 || sent[0].MailboxID != "mbx-3" {
		t.Errorf("last sent = %v, %v", sent, err)
	}
	days, err := q.MailboxDailyDeliveries(ctx, 86400000*4)
	if err != nil || len(days) != 1 || days[0].Day != 4 || days[0].N != 1 {
		t.Errorf("days = %v, %v", days, err)
	}
	if n, err := q.CountUnreadDeliveries(ctx); err != nil || n != 3 {
		t.Errorf("unread = %d, %v", n, err)
	}
	if n, err := q.CountMessages(ctx); err != nil || n != 2 {
		t.Errorf("messages = %d, %v", n, err)
	}
	if n, err := q.CountMessagesSince(ctx, 86400000*4); err != nil || n != 1 {
		t.Errorf("since = %d, %v", n, err)
	}
	// msg-1 has two deliveries but is older than the cutoff; msg-2 has one.
	if n, err := q.CountDeliveriesSince(ctx, 86400000*3); err != nil || n != 3 {
		t.Errorf("deliveries since = %d, %v", n, err)
	}
	if size, err := s.Size(ctx); err != nil || size <= 0 {
		t.Errorf("size = %d, %v", size, err)
	}

	err = s.WithTx(ctx, func(q *db.Queries) error {
		if n, err := q.UpdateIdentity(ctx, db.UpdateIdentityParams{ID: "mbx-3", Role: "r", Description: "d", ReportsTo: "operator"}); err != nil || n != 1 {
			return fmt.Errorf("update %d: %w", n, err)
		}
		_, err := q.SetRetiredAt(ctx, db.SetRetiredAtParams{ID: "mbx-3", RetiredAt: sql.NullInt64{Int64: 7, Valid: true}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sc := store.Scope{Host: "h", Org: "acme"}
	ids, _ := store.ListIdentitiesInScope(ctx, q, sc)
	if len(ids) != 2 {
		t.Errorf("active identities = %d, want 2", len(ids))
	}
	sc.IncludeRetired = true
	ids, _ = store.ListIdentitiesInScope(ctx, q, sc)
	if len(ids) != 3 {
		t.Errorf("all identities = %d, want 3", len(ids))
	}
	all, err := q.ListAllIdentities(ctx)
	if err != nil || len(all) != 3 {
		t.Errorf("ListAllIdentities = %d, %v", len(all), err)
	}
	boxes, err := q.ListAllMailboxes(ctx)
	if err != nil || len(boxes) != 4 {
		t.Errorf("ListAllMailboxes = %d, %v", len(boxes), err)
	}
}
