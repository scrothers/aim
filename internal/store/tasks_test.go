package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/store/db"
)

func TestMigration0005UpDownUp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, path := newStorePath(t)
	seed(t, s)
	send(t, s, "msg-1", "msg-1", "alpha", "beta", 10, "mbx-1")
	err := s.WithTx(ctx, func(q *db.Queries) error {
		if err := q.InsertTask(ctx, db.InsertTaskParams{
			ID: "task-1", Title: "t", Status: "not_started", Priority: "normal", CreatedBy: "operator", CreatedAt: 1, UpdatedAt: 1,
		}); err != nil {
			return err
		}
		return q.InsertTaskLink(ctx, db.InsertTaskLinkParams{TaskID: "task-1", MessageID: "msg-1", LinkedBy: "operator", LinkedAt: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	h := rawHandle(t, path)
	down, err := os.ReadFile(filepath.Join("migrations", "0005_tasks.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ExecContext(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, name := range []string{"tasks", "task_links", "task_events"} {
		if tableExists(t, h, name) {
			t.Errorf("%s survived down migration", name)
		}
	}
	fkClean(t, h)
	if _, err := h.ExecContext(ctx, `UPDATE schema_migrations SET version = 4`); err != nil {
		t.Fatal(err)
	}

	s2, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("re-up: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	for _, name := range []string{"tasks", "task_links", "task_events"} {
		if !tableExists(t, h, name) {
			t.Errorf("%s missing after re-up", name)
		}
	}
	if _, err := s2.Read().GetMessageByID(ctx, "msg-1"); err != nil {
		t.Fatalf("message lost: %v", err)
	}
}

func TestTaskQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := newStorePath(t)
	seed(t, s)
	send(t, s, "msg-1", "msg-1", "alpha", "beta", 10, "mbx-1")
	err := s.WithTx(ctx, func(q *db.Queries) error {
		for _, p := range []db.InsertTaskParams{
			{ID: "task-a", Title: "a", Status: "not_started", Priority: "normal", Scope: "github.com/acme/api", CreatedBy: "operator", Assignee: "bob@github.com/acme", CreatedAt: 1, UpdatedAt: 1},
			{ID: "task-b", Title: "b", Status: "finished", Priority: "high", Scope: "github.com/acme", CreatedBy: "alice@github.com/acme", CreatedAt: 2, UpdatedAt: 2},
		} {
			if err := q.InsertTask(ctx, p); err != nil {
				return err
			}
		}
		return q.InsertTaskLink(ctx, db.InsertTaskLinkParams{TaskID: "task-a", MessageID: "msg-1", LinkedBy: "operator", LinkedAt: 3})
	})
	if err != nil {
		t.Fatal(err)
	}
	q := s.Read()
	list := func(p db.ListTasksParams) []string {
		t.Helper()
		p.Limit = 10
		rows, err := q.ListTasks(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		return ids
	}
	if got := list(db.ListTasksParams{}); len(got) != 2 || got[0] != "task-b" {
		t.Errorf("all = %v, want newest first", got)
	}
	if got := list(db.ListTasksParams{OpenOnly: 1}); len(got) != 1 || got[0] != "task-a" {
		t.Errorf("open = %v", got)
	}
	if got := list(db.ListTasksParams{Scope: "github.com/acme", ScopePrefix: "github.com/acme/%"}); len(got) != 2 {
		t.Errorf("scope org = %v, want both", got)
	}
	if got := list(db.ListTasksParams{Involves: "bob@github.com/acme"}); len(got) != 1 || got[0] != "task-a" {
		t.Errorf("involves = %v", got)
	}
	if got := list(db.ListTasksParams{BeforeUpdated: 2, BeforeID: "task-b"}); len(got) != 1 || got[0] != "task-a" {
		t.Errorf("page 2 = %v", got)
	}
	links, err := q.ListTaskLinks(ctx, "task-a")
	if err != nil || len(links) != 1 || links[0].ThreadID != "msg-1" {
		t.Errorf("links = %+v, %v", links, err)
	}
	chips, err := q.ListTasksForThreads(ctx, []string{"msg-1"})
	if err != nil || len(chips) != 1 || chips[0].ID != "task-a" {
		t.Errorf("thread tasks = %+v, %v", chips, err)
	}
}
