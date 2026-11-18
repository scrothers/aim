package mdview_test

import (
	"testing"
	"time"

	"github.com/scrothers/aim/internal/mdview"
	"github.com/scrothers/aim/internal/tasks"
)

func TestTaskNudge(t *testing.T) {
	t.Parallel()
	cases := map[tasks.Counts]string{
		{}:                               "",
		{Held: 1}:                        "You hold 1 task; `task_list` shows them.\n",
		{Held: 2, Stale: 1}:              "You hold 2 tasks (1 stale); `task_list` shows them.\n",
		{Assigned: 3}:                    "You have 3 assigned tasks to claim; `task_list` shows them.\n",
		{Held: 1, Assigned: 1}:           "You hold 1 task and have 1 assigned task to claim; `task_list` shows them.\n",
		{Held: 2, Stale: 2, Assigned: 2}: "You hold 2 tasks (2 stale) and have 2 assigned tasks to claim; `task_list` shows them.\n",
	}
	for c, want := range cases {
		if got := mdview.TaskNudge(c); got != want {
			t.Errorf("TaskNudge(%+v) = %q, want %q", c, got, want)
		}
	}
}

func fixtureTasks() []tasks.Task {
	due := t0.Add(48 * time.Hour)
	act := t0.Add(time.Hour)
	return []tasks.Task{
		{
			ID: "task-1", Title: "Fix\nlogin", Status: tasks.StatusStarted, Priority: tasks.PriorityUrgent,
			Scope: "github.com/acme/api", CreatedBy: "operator", Assignee: "bob@github.com/acme/api",
			Claimant: "bob@github.com/acme/api", Health: tasks.HealthStale, DueAt: &due, LastActivityAt: &act,
			CreatedAt: t0, UpdatedAt: act,
		},
		{ID: "task-2", Title: "Docs", Status: tasks.StatusNotStarted, Priority: tasks.PriorityNormal, CreatedBy: "operator", CreatedAt: t0, UpdatedAt: t0},
	}
}

func TestTaskDigestGolden(t *testing.T) {
	t.Parallel()
	ts := fixtureTasks()
	golden(t, "task_digest", mdview.TaskDigest("Your tasks", []mdview.TaskSection{
		{Title: "Held by you", Empty: "You hold no tasks.", Tasks: ts[:1]},
		{Title: "Assigned to you", Empty: "Nothing is assigned to you.", Tasks: nil},
		{Title: "Open in github.com/acme/api", Empty: "None.", Tasks: ts[1:]},
	}))
}

func TestTaskDetailGolden(t *testing.T) {
	t.Parallel()
	ts := fixtureTasks()
	fin := t0.Add(3 * time.Hour)
	done := ts[1]
	done.Status, done.FinishedAt, done.Body = tasks.StatusFinished, &fin, "Write the README section.\n\n"
	golden(t, "task_detail", mdview.TaskDetail(tasks.Detail{
		Task: done,
		Events: []tasks.Event{
			{At: t0, Actor: "operator", Kind: tasks.EventCreated, ToStatus: tasks.StatusNotStarted},
			{At: t0.Add(time.Hour), Actor: "bob@github.com/acme/api", Kind: "start", FromStatus: tasks.StatusNotStarted, ToStatus: tasks.StatusStarted},
			{At: t0.Add(2 * time.Hour), Actor: "bob@github.com/acme/api", Kind: "comment", Note: "half\nway"},
			{At: fin, Actor: "bob@github.com/acme/api", Kind: "finish", FromStatus: tasks.StatusStarted, ToStatus: tasks.StatusFinished},
		},
		Links: []tasks.Link{{MessageID: "msg-1", ThreadID: "msg-1", Subject: "docs plan", LinkedBy: "operator", LinkedAt: t0}},
	}))
	golden(t, "task_detail_held", mdview.TaskDetail(tasks.Detail{Task: ts[0]}))
}
