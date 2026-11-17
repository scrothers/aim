package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/store/db"
)

func tableExists(t *testing.T, h *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := h.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE name = ?`, name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func fkClean(t *testing.T, h *sql.DB) {
	t.Helper()
	rows, err := h.QueryContext(context.Background(), `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Error("foreign_key_check reported violations")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestMigration0004UpDownUp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, path := newStorePath(t)
	seed(t, s)
	send(t, s, "msg-1", "msg-1", "alpha", "beta", 10, "mbx-1")
	err := s.WithTx(ctx, func(q *db.Queries) error {
		if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: "k", Value: `{}`, UpdatedAt: 1}); err != nil {
			return err
		}
		return q.MuteThread(ctx, db.MuteThreadParams{ThreadID: "msg-1", MutedAt: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	h := rawHandle(t, path)
	down, err := os.ReadFile(filepath.Join("migrations", "0004_phase3.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ExecContext(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, name := range []string{"session_signons", "settings", "thread_mutes"} {
		if tableExists(t, h, name) {
			t.Errorf("%s survived down migration", name)
		}
	}
	fkClean(t, h)
	if _, err := h.ExecContext(ctx, `UPDATE schema_migrations SET version = 3`); err != nil {
		t.Fatal(err)
	}

	s2, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("re-up: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	for _, name := range []string{"session_signons", "settings", "thread_mutes"} {
		if !tableExists(t, h, name) {
			t.Errorf("%s missing after re-up", name)
		}
	}
	fkClean(t, h)
	if _, err := s2.Read().GetMessageByID(ctx, "msg-1"); err != nil {
		t.Fatalf("message lost: %v", err)
	}
}

func TestSessionSignons(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStore(t)
	up := func(id, away string, at int64) {
		t.Helper()
		err := s.WithTx(ctx, func(q *db.Queries) error {
			return q.UpsertSessionSignon(ctx, db.UpsertSessionSignonParams{
				SessionID: id, Identity: "alice@h/acme", Away: away, Client: "c", Machine: "m", UpdatedAt: at,
			})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	up("s1", "", 100)
	up("s1", "lunch", 200) // upsert replaces
	up("s2", "", 50)

	got, err := s.Read().GetSessionSignon(ctx, db.GetSessionSignonParams{SessionID: "s1", MinUpdatedAt: 150})
	if err != nil || got.Away != "lunch" || got.UpdatedAt != 200 || got.Client != "c" || got.Machine != "m" {
		t.Fatalf("got %+v, err %v", got, err)
	}
	// Stale rows are invisible via min_updated_at.
	if _, err := s.Read().GetSessionSignon(ctx, db.GetSessionSignonParams{SessionID: "s1", MinUpdatedAt: 201}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale err = %v", err)
	}

	var n int64
	err = s.WithTx(ctx, func(q *db.Queries) error {
		var perr error
		n, perr = q.PruneSessionSignons(ctx, 100)
		return perr
	})
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, err %v", n, err)
	}
	if _, err := s.Read().GetSessionSignon(ctx, db.GetSessionSignonParams{SessionID: "s2"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("s2 survived prune: %v", err)
	}
	err = s.WithTx(ctx, func(q *db.Queries) error { return q.DeleteSessionSignon(ctx, "s1") })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read().GetSessionSignon(ctx, db.GetSessionSignonParams{SessionID: "s1"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("s1 survived delete: %v", err)
	}
}

func TestSettings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStore(t)
	err := s.WithTx(ctx, func(q *db.Queries) error {
		for _, p := range []db.UpsertSettingParams{
			{Key: "b", Value: `1`, UpdatedAt: 1},
			{Key: "a", Value: `"x"`, UpdatedAt: 2},
			{Key: "b", Value: `2`, UpdatedAt: 3},
		} {
			if err := q.UpsertSetting(ctx, p); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	all, err := s.Read().ListSettings(ctx)
	if err != nil || len(all) != 2 || all[0].Key != "a" || all[1].Value != "2" || all[1].UpdatedAt != 3 {
		t.Fatalf("list = %+v, err %v", all, err)
	}
	one, err := s.Read().GetSetting(ctx, "a")
	if err != nil || one.Value != `"x"` {
		t.Fatalf("get = %+v, err %v", one, err)
	}
	if err := s.WithTx(ctx, func(q *db.Queries) error { return q.DeleteSetting(ctx, "a") }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read().GetSetting(ctx, "a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v", err)
	}
}

func TestThreadMutes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStore(t)
	muted := func(id string) bool {
		t.Helper()
		ok, err := s.Read().IsThreadMuted(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	err := s.WithTx(ctx, func(q *db.Queries) error {
		if err := q.MuteThread(ctx, db.MuteThreadParams{ThreadID: "t1", MutedAt: 10}); err != nil {
			return err
		}
		// Second mute is ignored and keeps the original timestamp.
		if err := q.MuteThread(ctx, db.MuteThreadParams{ThreadID: "t1", MutedAt: 99}); err != nil {
			return err
		}
		return q.MuteThread(ctx, db.MuteThreadParams{ThreadID: "t2", MutedAt: 20})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !muted("t1") || muted("t3") {
		t.Fatal("IsThreadMuted wrong")
	}
	list, err := s.Read().ListMutedThreads(ctx)
	if err != nil || len(list) != 2 || list[0].ThreadID != "t2" || list[1].MutedAt != 10 {
		t.Fatalf("list = %+v, err %v", list, err)
	}
	if err := s.WithTx(ctx, func(q *db.Queries) error { return q.UnmuteThread(ctx, "t1") }); err != nil {
		t.Fatal(err)
	}
	if muted("t1") {
		t.Fatal("still muted")
	}
}

func TestRetentionPrune(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, path := newStorePath(t)
	seed(t, s)
	send(t, s, "old-all", "old-all", "gone", "needle", 10, "mbx-1", "mbx-2")
	send(t, s, "old-partial", "old-partial", "kept", "x", 11, "mbx-1", "mbx-2")
	send(t, s, "new-all", "new-all", "kept", "x", 500, "mbx-1")
	send(t, s, "old-nodeliv", "old-nodeliv", "kept", "x", 12)
	// A reply to a prunable message, itself unarchived.
	err := s.WithTx(ctx, func(q *db.Queries) error {
		if err := q.InsertMessage(ctx, db.InsertMessageParams{
			ID: "reply", ThreadID: "old-all", InReplyTo: sql.NullString{String: "old-all", Valid: true},
			FromMailbox: "mbx-3", Subject: "re", Body: "x", CreatedAt: 600,
		}); err != nil {
			return err
		}
		return q.InsertDelivery(ctx, db.InsertDeliveryParams{MessageID: "reply", MailboxID: "mbx-1", Kind: "to"})
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := rawHandle(t, path)
	for _, q := range []string{
		`UPDATE deliveries SET archived_at = 1 WHERE message_id IN ('old-all', 'new-all')`,
		`UPDATE deliveries SET archived_at = 1 WHERE message_id = 'old-partial' AND mailbox_id = 'mbx-1'`,
	} {
		if _, err := raw.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	ids, err := s.Read().ListPrunableMessages(ctx, db.ListPrunableMessagesParams{BeforeMs: 100, RowLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, ids, []string{"old-all"})

	// The limit is honored.
	if _, err := raw.ExecContext(ctx, `UPDATE deliveries SET archived_at = 1`); err != nil {
		t.Fatal(err)
	}
	ids, err = s.Read().ListPrunableMessages(ctx, db.ListPrunableMessagesParams{BeforeMs: 100, RowLimit: 1})
	if err != nil || len(ids) != 1 || ids[0] != "old-all" {
		t.Fatalf("limit ids = %v, err %v", ids, err)
	}
	ids, err = s.Read().ListPrunableMessages(ctx, db.ListPrunableMessagesParams{BeforeMs: 100, RowLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, ids, []string{"old-all", "old-partial"}) // old-nodeliv has no deliveries

	// With foreign keys enforced a referenced message cannot be deleted, so
	// replies are detached first.
	err = s.WithTx(ctx, func(q *db.Queries) error {
		if err := q.DeleteMessageDeliveries(ctx, "old-all"); err != nil {
			return err
		}
		return q.DeleteMessage(ctx, "old-all")
	})
	if err == nil {
		t.Fatal("deleting a replied-to message succeeded")
	}
	var detached int64
	err = s.WithTx(ctx, func(q *db.Queries) error {
		var nerr error
		detached, nerr = q.NullRepliesTo(ctx, sql.NullString{String: "old-all", Valid: true})
		if nerr != nil {
			return nerr
		}
		if err := q.DeleteMessageDeliveries(ctx, "old-all"); err != nil {
			return err
		}
		return q.DeleteMessage(ctx, "old-all")
	})
	if err != nil {
		t.Fatal(err)
	}
	if detached != 1 {
		t.Fatalf("detached = %d", detached)
	}
	m, err := s.Read().GetMessageByID(ctx, "reply")
	if err != nil || m.InReplyTo.Valid {
		t.Fatalf("reply = %+v, err %v", m, err)
	}
	hits, err := s.Search(ctx, store.SearchParams{Host: "h", Org: "acme", Query: "needle"})
	if err != nil || len(hits) != 0 {
		t.Fatalf("fts hits = %v, err %v", hits, err)
	}
	fkClean(t, raw)
}

func TestBackup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStore(t)
	seed(t, s)
	send(t, s, "msg-1", "msg-1", "alpha", "beta", 10, "mbx-1")
	send(t, s, "msg-2", "msg-2", "gamma", "delta", 11, "mbx-1")

	dst := filepath.Join(t.TempDir(), "sub", "backup.db")
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(ctx, dst); err != nil {
		t.Fatal(err)
	}
	b, err := store.Open(ctx, dst)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	for _, id := range []string{"msg-1", "msg-2"} {
		if _, err := b.Read().GetMessageByID(ctx, id); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	rows, err := store.ListInbox(ctx, b.Read(), store.InboxParams{Scope: store.Scope{All: true}, State: "all", PageLimit: 10})
	if err != nil || len(rows) != 2 {
		t.Fatalf("backup inbox = %d, err %v", len(rows), err)
	}

	if err := s.Backup(ctx, dst); err == nil {
		t.Fatal("Backup overwrote an existing path")
	}
	// A path that cannot be stat'ed or written surfaces the error.
	if err := s.Backup(ctx, filepath.Join(t.TempDir(), "missing", "dir", "b.db")); err == nil {
		t.Fatal("Backup into missing dir succeeded")
	}
}

func TestNullDanglingReplies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, path := newStorePath(t)
	seed(t, s)
	send(t, s, "msg-1", "msg-1", "a", "b", 10, "mbx-1")
	err := s.WithTx(ctx, func(q *db.Queries) error {
		return q.InsertMessage(ctx, db.InsertMessageParams{
			ID: "msg-2", ThreadID: "msg-1", InReplyTo: sql.NullString{String: "msg-1", Valid: true},
			FromMailbox: "mbx-3", Subject: "re", Body: "x", CreatedAt: 11,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	// Create a dangling pointer the way a legacy database could have one:
	// with foreign keys off on a dedicated connection.
	conn, err := rawHandle(t, path).Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`PRAGMA foreign_keys = OFF`, `DELETE FROM deliveries WHERE message_id = 'msg-1'`, `DELETE FROM messages WHERE id = 'msg-1'`} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	_ = conn.Close()

	var n int64
	err = s.WithTx(ctx, func(q *db.Queries) error {
		var nerr error
		n, nerr = q.NullDanglingReplies(ctx)
		return nerr
	})
	if err != nil || n != 1 {
		t.Fatalf("n = %d, err %v", n, err)
	}
	m, err := s.Read().GetMessageByID(ctx, "msg-2")
	if err != nil || m.InReplyTo.Valid {
		t.Fatalf("msg-2 = %+v, err %v", m, err)
	}
	fkClean(t, rawHandle(t, path))
}
