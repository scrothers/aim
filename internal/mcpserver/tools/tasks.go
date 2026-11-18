package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/tasks"
)

// TaskCreateInput is the input of task_create.
type TaskCreateInput struct {
	Title     string         `json:"title" jsonschema:"One-line summary of the work, 1 to 200 bytes, for example Fix flaky login test."`
	Body      string         `json:"body,omitempty" jsonschema:"Markdown details, up to 32 KiB: goal, context, acceptance criteria, file paths."`
	Priority  tasks.Priority `json:"priority,omitempty" jsonschema:"low, normal (default), high or urgent."`
	Scope     string         `json:"scope,omitempty" jsonschema:"Org or repo address the task belongs to, for example github.com/acme/api; omit for an unscoped task. Used for filtering."`
	Assignee  string         `json:"assignee,omitempty" jsonschema:"Identity to reserve the task for (from roster), or operator; only it may claim the task. They are mailed about it."`
	DueAt     string         `json:"due_at,omitempty" jsonschema:"Optional RFC3339 deadline, for example 2026-11-20T17:00:00Z."`
	MessageID string         `json:"message_id,omitempty" jsonschema:"Optional message id (msg-...) to link at creation, such as the request that prompted the task."`
}

// TaskListInput is the input of task_list.
type TaskListInput struct {
	Status          tasks.Status `json:"status,omitempty" jsonschema:"Only tasks in this status: not_started, claimed, started, finished or abandoned."`
	Assignee        string       `json:"assignee,omitempty" jsonschema:"Only tasks reserved for this identity address (or operator)."`
	Claimant        string       `json:"claimant,omitempty" jsonschema:"Only tasks held (claimed or started) by this identity address."`
	Involves        string       `json:"involves,omitempty" jsonschema:"Only tasks this identity created, is assigned or holds."`
	Scope           string       `json:"scope,omitempty" jsonschema:"Only tasks scoped to this org or repo address or below it, for example github.com/acme."`
	IncludeFinished bool         `json:"include_finished,omitempty" jsonschema:"Also return finished tasks; by default only open ones (not_started, claimed, started, abandoned)."`
	Limit           int          `json:"limit,omitempty" jsonschema:"Maximum items per page; default 50, maximum 200."`
	Cursor          string       `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page; keep the other arguments unchanged."`
}

// TaskGetInput is the input of task_get.
type TaskGetInput struct {
	TaskID string `json:"task_id" jsonschema:"Task id (task-...) from task_list, task_create or a notice."`
}

// TaskUpdateInput is the input of task_update.
type TaskUpdateInput struct {
	TaskID     string         `json:"task_id" jsonschema:"Task id (task-...) to change."`
	Action     tasks.Action   `json:"action" jsonschema:"What to do: claim, start, finish, release, assign, unassign, reopen, comment, link or edit. See the tool description for who may do what."`
	Note       string         `json:"note,omitempty" jsonschema:"The comment text for comment (required); an optional reason for any other action, kept in the history."`
	Assignee   string         `json:"assignee,omitempty" jsonschema:"For assign: the identity address (from roster) or operator to reserve the task for."`
	MessageID  string         `json:"message_id,omitempty" jsonschema:"For link: the message id (msg-...) to link to the task."`
	Title      string         `json:"title,omitempty" jsonschema:"For edit: new title; omit to keep."`
	Body       string         `json:"body,omitempty" jsonschema:"For edit: new Markdown body; omit to keep."`
	Priority   tasks.Priority `json:"priority,omitempty" jsonschema:"For edit: new priority low, normal, high or urgent; omit to keep."`
	Scope      string         `json:"scope,omitempty" jsonschema:"For edit: new org or repo address scope; omit to keep, or set clear_scope."`
	ClearScope bool           `json:"clear_scope,omitempty" jsonschema:"For edit: remove the scope."`
	DueAt      string         `json:"due_at,omitempty" jsonschema:"For edit: new RFC3339 deadline; omit to keep, or set clear_due."`
	ClearDue   bool           `json:"clear_due,omitempty" jsonschema:"For edit: remove the deadline."`
}

func (t *toolset) taskCreate(ctx context.Context, _ *mcp.CallToolRequest, in TaskCreateInput) (*mcp.CallToolResult, tasks.Task, error) {
	board, err := t.c.TaskBoard()
	if err != nil {
		return nil, tasks.Task{}, err
	}
	due, err := session.ParseTime("due_at", in.DueAt)
	if err != nil {
		return nil, tasks.Task{}, err
	}
	tk, err := board.Create(ctx, t.c.Caller(), tasks.CreateInput{
		Title: in.Title, Body: in.Body, Priority: in.Priority, Scope: in.Scope,
		Assignee: in.Assignee, DueAt: due, LinkMessageID: in.MessageID,
	})
	return nil, tk, session.ToolErr(err)
}

// filtered reports whether in narrows the listing beyond the default.
func (in TaskListInput) filtered() bool {
	return in.Status != "" || in.Assignee != "" || in.Claimant != "" || in.Involves != "" ||
		in.Scope != "" || in.Cursor != "" || in.IncludeFinished
}

func (t *toolset) taskList(ctx context.Context, _ *mcp.CallToolRequest, in TaskListInput) (*mcp.CallToolResult, tasks.ListResult, error) {
	empty := tasks.ListResult{Items: []tasks.Task{}}
	board, err := t.c.TaskBoard()
	if err != nil {
		return nil, empty, err
	}
	if !in.filtered() {
		res, err := t.defaultTasks(ctx, board, in.Limit)
		return nil, res, session.ToolErr(err)
	}
	res, err := board.List(ctx, tasks.ListInput{
		Status: in.Status, Assignee: in.Assignee, Claimant: in.Claimant, Involves: in.Involves,
		Scope: in.Scope, OpenOnly: !in.IncludeFinished, Limit: in.Limit, Cursor: in.Cursor,
	})
	if res.Items == nil {
		res.Items = []tasks.Task{}
	}
	return nil, res, session.ToolErr(err)
}

// defaultTasks is task_list without filters: the caller's open tasks, then
// open unassigned, unclaimed tasks in the session's namespace. It is one
// page with no cursor.
func (t *toolset) defaultTasks(ctx context.Context, board *tasks.Service, limit int) (tasks.ListResult, error) {
	if limit <= 0 || limit > tasks.MaxListLimit {
		limit = tasks.DefaultListLimit
	}
	out := tasks.ListResult{Items: []tasks.Task{}}
	seen := map[string]bool{}
	add := func(in tasks.ListInput, keep func(tasks.Task) bool) error {
		in.OpenOnly, in.Limit = true, tasks.MaxListLimit
		res, err := board.List(ctx, in)
		if err != nil {
			return err
		}
		for _, tk := range res.Items {
			if len(out.Items) >= limit {
				return nil
			}
			if !seen[tk.ID] && keep(tk) {
				seen[tk.ID] = true
				out.Items = append(out.Items, tk)
			}
		}
		return nil
	}
	all := func(tasks.Task) bool { return true }
	c := t.c.Caller()
	ns := t.c.Entry.Preamble().Namespace
	if c.SignedOn {
		if err := add(tasks.ListInput{Involves: c.Identity.IdentityAddress().String()}, all); err != nil {
			return out, err
		}
	}
	if ns == "" && !c.SignedOn {
		// Nowhere to anchor: show the most recently updated open tasks.
		return out, add(tasks.ListInput{}, all)
	}
	if ns == "" {
		return out, nil
	}
	return out, add(tasks.ListInput{Scope: ns}, func(tk tasks.Task) bool { return tk.Assignee == "" && tk.Claimant == "" })
}

func (t *toolset) taskGet(ctx context.Context, _ *mcp.CallToolRequest, in TaskGetInput) (*mcp.CallToolResult, tasks.Detail, error) {
	board, err := t.c.TaskBoard()
	if err != nil {
		return nil, tasks.Detail{}, err
	}
	d, err := board.Get(ctx, in.TaskID)
	return nil, d, session.ToolErr(err)
}

func (t *toolset) taskUpdate(ctx context.Context, _ *mcp.CallToolRequest, in TaskUpdateInput) (*mcp.CallToolResult, tasks.Task, error) {
	board, err := t.c.TaskBoard()
	if err != nil {
		return nil, tasks.Task{}, err
	}
	up := tasks.UpdateInput{
		TaskID: in.TaskID, Action: in.Action, Note: in.Note, Assignee: in.Assignee,
		MessageID: in.MessageID, ClearDue: in.ClearDue,
	}
	if in.Action == tasks.ActionEdit {
		if in.Title != "" {
			up.Title = &in.Title
		}
		if in.Body != "" {
			up.Body = &in.Body
		}
		if in.Priority != "" {
			up.Priority = &in.Priority
		}
		switch {
		case in.ClearScope:
			up.Scope = new("")
		case in.Scope != "":
			up.Scope = &in.Scope
		}
		if !in.ClearDue {
			if up.DueAt, err = session.ParseTime("due_at", in.DueAt); err != nil {
				return nil, tasks.Task{}, err
			}
		}
	}
	tk, err := board.Update(ctx, t.c.Caller(), up)
	return nil, tk, session.ToolErr(err)
}

// linkSent links a just-sent message to a task, or explains that the
// message went out but the link failed.
func (t *toolset) linkSent(ctx context.Context, c mail.Caller, taskID string, m mail.Message) error {
	board, err := t.c.TaskBoard()
	if err != nil {
		return err
	}
	if err := board.Link(ctx, c, taskID, m.ID); err != nil {
		return fmt.Errorf("message %s was sent, but linking it to %s failed (retry with task_update action link): %w", m.ID, taskID, err)
	}
	return nil
}
