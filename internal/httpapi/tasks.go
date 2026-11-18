package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/tasks"
)

// TaskView is one task as the console shows it. Enum tags repeat the tasks
// package's values; tasks_test.go keeps them in step.
type TaskView struct {
	ID             string     `json:"id" doc:"Task id, task-..."`
	Title          string     `json:"title" doc:"One-line summary."`
	Body           string     `json:"body" doc:"Markdown details."`
	Status         string     `json:"status" enum:"not_started,claimed,started,finished,abandoned" doc:"Lifecycle status."`
	Priority       string     `json:"priority" enum:"low,normal,high,urgent" doc:"Priority."`
	Scope          string     `json:"scope,omitempty" doc:"Org or repo address the task belongs to; absent when unscoped."`
	CreatedBy      string     `json:"created_by" doc:"Identity address that created the task, or operator."`
	Assignee       string     `json:"assignee,omitempty" doc:"Identity the task is reserved for; absent when unassigned."`
	Claimant       string     `json:"claimant,omitempty" doc:"Identity holding the task while claimed or started."`
	Health         string     `json:"health,omitempty" enum:"active,idle,stale,abandoned" doc:"Lease health of a claimed or started task."`
	DueAt          *time.Time `json:"due_at,omitempty" doc:"Deadline."`
	ClaimedAt      *time.Time `json:"claimed_at,omitempty" doc:"When the current claim began."`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty" doc:"Latest activity by the claimant's session."`
	FinishedAt     *time.Time `json:"finished_at,omitempty" doc:"When the task was finished."`
	CreatedAt      time.Time  `json:"created_at" doc:"When the task was created."`
	UpdatedAt      time.Time  `json:"updated_at" doc:"When the task last changed."`
}

// TaskEventView is one history entry.
type TaskEventView struct {
	At         time.Time `json:"at" doc:"When it happened."`
	Actor      string    `json:"actor" doc:"Identity or operator that acted; the former claimant for decay events."`
	Kind       string    `json:"kind" enum:"created,stale,decayed,claim,start,finish,release,assign,unassign,reopen,comment,link,edit" doc:"What happened."`
	FromStatus string    `json:"from_status,omitempty" enum:"not_started,claimed,started,finished,abandoned" doc:"Status before, when it changed."`
	ToStatus   string    `json:"to_status,omitempty" enum:"not_started,claimed,started,finished,abandoned" doc:"Status after, when it changed."`
	Note       string    `json:"note,omitempty" doc:"Comment, reason, assignee or linked message id."`
}

// TaskLinkView is a message linked to a task.
type TaskLinkView struct {
	MessageID string    `json:"message_id" doc:"Linked message id."`
	ThreadID  string    `json:"thread_id" doc:"Thread of the linked message."`
	Subject   string    `json:"subject" doc:"Subject of the linked message."`
	LinkedBy  string    `json:"linked_by" doc:"Who linked it."`
	LinkedAt  time.Time `json:"linked_at" doc:"When it was linked."`
}

// TaskDetailView is a task with rendered body, history and links.
type TaskDetailView struct {
	TaskView
	BodyHTML string          `json:"body_html" doc:"Body rendered from Markdown to sanitized HTML; safe to inject."`
	Events   []TaskEventView `json:"events" doc:"History, oldest first."`
	Links    []TaskLinkView  `json:"links" doc:"Linked messages, oldest link first."`
}

// TaskListBody is a page of tasks, newest-updated first.
type TaskListBody struct {
	Items      []TaskView `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty" doc:"Pass as cursor for the next page; absent on the last page."`
}

// ThreadTaskView is a task linked to a thread, for message chips.
type ThreadTaskView struct {
	ThreadID string `json:"thread_id"`
	TaskID   string `json:"task_id"`
	Title    string `json:"title"`
	Status   string `json:"status" enum:"not_started,claimed,started,finished,abandoned"`
}

// ThreadTasksBody lists the tasks linked to the requested threads.
type ThreadTasksBody struct {
	Items []ThreadTaskView `json:"items"`
}

// TaskCreateRequest creates a task as the operator.
type TaskCreateRequest struct {
	Title     string     `json:"title" minLength:"1" maxLength:"200" doc:"One-line summary."`
	Body      string     `json:"body,omitempty" maxLength:"32768" doc:"Markdown details."`
	Priority  string     `json:"priority,omitempty" enum:"low,normal,high,urgent" doc:"Priority (default normal)."`
	Scope     string     `json:"scope,omitempty" doc:"Org or repo address, for example github.com/acme/api."`
	Assignee  string     `json:"assignee,omitempty" doc:"Identity address (or operator) to reserve the task for; they are mailed."`
	DueAt     *time.Time `json:"due_at,omitempty" doc:"Deadline, RFC 3339."`
	MessageID string     `json:"message_id,omitempty" doc:"Message id to link at creation."`
}

// TaskActionRequest applies one action to a task as the operator. Fields
// beyond action are read only by the actions that use them.
type TaskActionRequest struct {
	Action    string     `json:"action" enum:"claim,start,finish,release,assign,unassign,reopen,comment,link,edit" doc:"The change to apply."`
	Note      string     `json:"note,omitempty" maxLength:"8192" doc:"Comment text (required for comment) or a reason for any action."`
	Assignee  string     `json:"assignee,omitempty" doc:"For assign: identity address or operator."`
	MessageID string     `json:"message_id,omitempty" doc:"For link: message id."`
	Title     *string    `json:"title,omitempty" maxLength:"200" doc:"For edit: new title."`
	Body      *string    `json:"body,omitempty" maxLength:"32768" doc:"For edit: new Markdown body; empty clears it."`
	Priority  *string    `json:"priority,omitempty" enum:"low,normal,high,urgent" doc:"For edit: new priority."`
	Scope     *string    `json:"scope,omitempty" doc:"For edit: new org or repo address; empty clears it."`
	DueAt     *time.Time `json:"due_at,omitempty" doc:"For edit: new deadline."`
	ClearDue  bool       `json:"clear_due,omitempty" doc:"For edit: remove the deadline."`
}

type (
	taskListInput struct {
		Status   string `query:"status" enum:"not_started,claimed,started,finished,abandoned" doc:"Only tasks in this status."`
		Assignee string `query:"assignee" doc:"Only tasks reserved for this identity (or operator)."`
		Claimant string `query:"claimant" doc:"Only tasks held by this identity."`
		Involves string `query:"involves" doc:"Only tasks this identity created, is assigned or holds."`
		Scope    string `query:"scope" doc:"Only tasks scoped to this org or repo address or below it."`
		OpenOnly bool   `query:"open_only" doc:"Leave out finished tasks."`
		Limit    int    `query:"limit" minimum:"1" maximum:"200" doc:"Page size (default 50)."`
		Cursor   string `query:"cursor" doc:"next_cursor from the previous page."`
	}
	taskIDPath struct {
		ID string `path:"id" doc:"Task id."`
	}
	taskActionInput struct {
		ID   string `path:"id" doc:"Task id."`
		Body TaskActionRequest
	}
	threadTasksInput struct {
		ThreadIDs []string `query:"thread_id,explode" required:"true" minItems:"1" maxItems:"200" doc:"Thread ids; repeat the parameter for several."`
	}
)

func registerTasks(api huma.API, h *handlers) {
	op := func(method, id, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: []string{"aim"}}
	}
	huma.Register(api, op(http.MethodGet, "listTasks", "/api/tasks", "List tasks, newest-updated first"), h.listTasks)
	create := op(http.MethodPost, "createTask", "/api/tasks", "Create a task as the operator")
	create.DefaultStatus = http.StatusCreated
	huma.Register(api, create, h.createTask)
	huma.Register(api, op(http.MethodGet, "getTask", "/api/tasks/{id}", "A task with its history and linked messages"), h.getTask)
	huma.Register(api, op(http.MethodPost, "updateTask", "/api/tasks/{id}/actions", "Apply an action to a task as the operator"), h.taskAction)
	huma.Register(api, op(http.MethodGet, "listThreadTasks", "/api/thread-tasks", "Tasks linked to messages in the given threads"), h.threadTasks)
}

// taskError maps a tasks error to an HTTP error.
func taskError(err error) error {
	switch {
	case errors.Is(err, tasks.ErrNotFound):
		return huma.NewError(http.StatusNotFound, err.Error())
	case errors.Is(err, tasks.ErrForbidden), errors.Is(err, mail.ErrNotSignedOn):
		return huma.NewError(http.StatusForbidden, err.Error())
	case errors.Is(err, tasks.ErrTransition):
		return huma.NewError(http.StatusConflict, err.Error())
	default:
		return apiError(err)
	}
}

func taskView(t tasks.Task) TaskView {
	return TaskView{
		ID: t.ID, Title: t.Title, Body: t.Body, Status: string(t.Status), Priority: string(t.Priority),
		Scope: t.Scope, CreatedBy: t.CreatedBy, Assignee: t.Assignee, Claimant: t.Claimant, Health: string(t.Health),
		DueAt: t.DueAt, ClaimedAt: t.ClaimedAt, LastActivityAt: t.LastActivityAt, FinishedAt: t.FinishedAt,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func (h *handlers) detailView(d tasks.Detail) (TaskDetailView, error) {
	html, err := h.render.HTML(d.Body)
	if err != nil {
		return TaskDetailView{}, huma.NewError(http.StatusInternalServerError, "render failed", err)
	}
	v := TaskDetailView{TaskView: taskView(d.Task), BodyHTML: html, Events: make([]TaskEventView, len(d.Events)), Links: make([]TaskLinkView, len(d.Links))}
	for i, e := range d.Events {
		v.Events[i] = TaskEventView{At: e.At, Actor: e.Actor, Kind: e.Kind, FromStatus: string(e.FromStatus), ToStatus: string(e.ToStatus), Note: e.Note}
	}
	for i, l := range d.Links {
		v.Links[i] = TaskLinkView{MessageID: l.MessageID, ThreadID: l.ThreadID, Subject: l.Subject, LinkedBy: l.LinkedBy, LinkedAt: l.LinkedAt}
	}
	return v, nil
}

func (h *handlers) listTasks(ctx context.Context, in *taskListInput) (*out[TaskListBody], error) {
	if h.tasks == nil {
		return nil, unavailable("the task board")
	}
	res, err := h.tasks.List(ctx, tasks.ListInput{
		Status: tasks.Status(in.Status), Assignee: in.Assignee, Claimant: in.Claimant, Involves: in.Involves,
		Scope: in.Scope, OpenOnly: in.OpenOnly, Limit: in.Limit, Cursor: in.Cursor,
	})
	if err != nil {
		return nil, taskError(err)
	}
	body := TaskListBody{Items: make([]TaskView, len(res.Items)), NextCursor: res.NextCursor}
	for i, t := range res.Items {
		body.Items[i] = taskView(t)
	}
	return &out[TaskListBody]{Body: body}, nil
}

func (h *handlers) createTask(ctx context.Context, in *struct{ Body TaskCreateRequest }) (*out[TaskDetailView], error) {
	if h.tasks == nil {
		return nil, unavailable("the task board")
	}
	b := in.Body
	t, err := h.tasks.Create(ctx, h.caller, tasks.CreateInput{
		Title: b.Title, Body: b.Body, Priority: tasks.Priority(b.Priority), Scope: b.Scope,
		Assignee: b.Assignee, DueAt: b.DueAt, LinkMessageID: b.MessageID,
	})
	if err != nil {
		return nil, taskError(err)
	}
	return h.detail(ctx, t.ID)
}

func (h *handlers) detail(ctx context.Context, id string) (*out[TaskDetailView], error) {
	d, err := h.tasks.Get(ctx, id)
	if err != nil {
		return nil, taskError(err)
	}
	v, err := h.detailView(d)
	if err != nil {
		return nil, err
	}
	return &out[TaskDetailView]{Body: v}, nil
}

func (h *handlers) getTask(ctx context.Context, in *taskIDPath) (*out[TaskDetailView], error) {
	if h.tasks == nil {
		return nil, unavailable("the task board")
	}
	return h.detail(ctx, in.ID)
}

func (h *handlers) taskAction(ctx context.Context, in *taskActionInput) (*out[TaskDetailView], error) {
	if h.tasks == nil {
		return nil, unavailable("the task board")
	}
	b := in.Body
	up := tasks.UpdateInput{
		TaskID: in.ID, Action: tasks.Action(b.Action), Note: b.Note, Assignee: b.Assignee, MessageID: b.MessageID,
		Title: b.Title, Body: b.Body, Scope: b.Scope, DueAt: b.DueAt, ClearDue: b.ClearDue,
	}
	if b.Priority != nil {
		up.Priority = new(tasks.Priority(*b.Priority))
	}
	if _, err := h.tasks.Update(ctx, h.caller, up); err != nil {
		return nil, taskError(err)
	}
	return h.detail(ctx, in.ID)
}

func (h *handlers) threadTasks(ctx context.Context, in *threadTasksInput) (*out[ThreadTasksBody], error) {
	if h.tasks == nil {
		return nil, unavailable("the task board")
	}
	ts, err := h.tasks.ForThreads(ctx, in.ThreadIDs)
	if err != nil {
		return nil, taskError(err)
	}
	body := ThreadTasksBody{Items: make([]ThreadTaskView, len(ts))}
	for i, t := range ts {
		body.Items[i] = ThreadTaskView{ThreadID: t.ThreadID, TaskID: t.TaskID, Title: t.Title, Status: string(t.Status)}
	}
	return &out[ThreadTasksBody]{Body: body}, nil
}
