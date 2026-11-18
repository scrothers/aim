package tasks

import (
	"errors"
	"time"
)

// Status is where a task is in its lifecycle.
type Status string

// Statuses.
const (
	StatusNotStarted Status = "not_started"
	StatusClaimed    Status = "claimed"
	StatusStarted    Status = "started"
	StatusFinished   Status = "finished"
	StatusAbandoned  Status = "abandoned"
)

// Statuses lists every status in board order.
var Statuses = []Status{StatusNotStarted, StatusClaimed, StatusStarted, StatusFinished, StatusAbandoned}

// Priority orders work.
type Priority string

// Priorities.
const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// Priorities lists every priority, lowest first.
var Priorities = []Priority{PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent}

// Health is the lease state of a claimed or started task, derived from the
// time since its last activity; empty for other statuses.
type Health string

// Health stages, healthiest first.
const (
	HealthActive    Health = "active"
	HealthIdle      Health = "idle"
	HealthStale     Health = "stale"
	HealthAbandoned Health = "abandoned"
)

// Action is a change Update applies.
type Action string

// Actions.
const (
	ActionClaim    Action = "claim"
	ActionStart    Action = "start"
	ActionFinish   Action = "finish"
	ActionRelease  Action = "release"
	ActionAssign   Action = "assign"
	ActionUnassign Action = "unassign"
	ActionReopen   Action = "reopen"
	ActionComment  Action = "comment"
	ActionLink     Action = "link"
	ActionEdit     Action = "edit"
)

// Actions lists every action.
var Actions = []Action{
	ActionClaim, ActionStart, ActionFinish, ActionRelease, ActionAssign,
	ActionUnassign, ActionReopen, ActionComment, ActionLink, ActionEdit,
}

// Event kinds recorded in a task's history besides the actions themselves.
const (
	EventCreated = "created"
	EventStale   = "stale"
	EventDecayed = "decayed"
)

// Operator is how the human operator appears in CreatedBy, Assignee,
// Claimant and Event.Actor.
const Operator = "operator"

// Task is one work item.
type Task struct {
	ID       string   `json:"id" jsonschema:"Task id, task-..."`
	Title    string   `json:"title" jsonschema:"One-line summary of the work."`
	Body     string   `json:"body" jsonschema:"Markdown details: goal, context, acceptance criteria."`
	Status   Status   `json:"status" jsonschema:"Lifecycle status."`
	Priority Priority `json:"priority" jsonschema:"Priority; normal unless set."`
	Scope    string   `json:"scope,omitempty" jsonschema:"Org or repo address the task belongs to, for filtering; absent for unscoped tasks."`
	// CreatedBy, Assignee and Claimant are identity addresses or "operator".
	CreatedBy string `json:"created_by" jsonschema:"Identity that created the task, or operator."`
	Assignee  string `json:"assignee,omitempty" jsonschema:"Identity the task is reserved for; only it (or the operator) may claim it. Absent when unassigned."`
	Claimant  string `json:"claimant,omitempty" jsonschema:"Identity currently holding the task (claimed or started); absent otherwise."`
	Health    Health `json:"health,omitempty" jsonschema:"Lease health of a claimed or started task: active, idle, stale or abandoned; absent otherwise."`
	// Times; nil when unset.
	DueAt          *time.Time `json:"due_at,omitempty" jsonschema:"Optional deadline, RFC3339."`
	ClaimedAt      *time.Time `json:"claimed_at,omitempty" jsonschema:"When the current claim began."`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty" jsonschema:"Latest activity by the claimant's session; drives health."`
	FinishedAt     *time.Time `json:"finished_at,omitempty" jsonschema:"When the task was finished."`
	CreatedAt      time.Time  `json:"created_at" jsonschema:"When the task was created."`
	UpdatedAt      time.Time  `json:"updated_at" jsonschema:"When the task last changed."`
}

// Event is one entry of a task's history.
type Event struct {
	At         time.Time `json:"at" jsonschema:"When it happened."`
	Actor      string    `json:"actor" jsonschema:"Identity or operator that acted; the claimant for automatic decay events."`
	Kind       string    `json:"kind" jsonschema:"What happened: created, an action name, stale or decayed."`
	FromStatus Status    `json:"from_status,omitempty" jsonschema:"Status before, when it changed."`
	ToStatus   Status    `json:"to_status,omitempty" jsonschema:"Status after, when it changed."`
	Note       string    `json:"note,omitempty" jsonschema:"Comment or reason given with the change."`
}

// Link is a message linked to a task.
type Link struct {
	MessageID string    `json:"message_id" jsonschema:"Linked message id."`
	ThreadID  string    `json:"thread_id" jsonschema:"Thread of the linked message."`
	Subject   string    `json:"subject" jsonschema:"Subject of the linked message."`
	LinkedBy  string    `json:"linked_by" jsonschema:"Who linked it."`
	LinkedAt  time.Time `json:"linked_at" jsonschema:"When it was linked."`
}

// Detail is a task with its history and links.
type Detail struct {
	Task
	Events []Event `json:"events" jsonschema:"History, oldest first."`
	Links  []Link  `json:"links" jsonschema:"Linked messages, oldest link first."`
}

// ThreadTask is a task linked to a thread, for chips on messages.
type ThreadTask struct {
	ThreadID string `json:"thread_id"`
	TaskID   string `json:"task_id"`
	Title    string `json:"title"`
	Status   Status `json:"status"`
}

// CreateInput is the argument of Create.
type CreateInput struct {
	Title    string
	Body     string
	Priority Priority // empty means normal
	Scope    string   // optional org or repo address
	Assignee string   // optional identity address
	DueAt    *time.Time
	// LinkMessageID optionally links an existing message at creation.
	LinkMessageID string
}

// ListInput filters List. Empty fields match everything.
type ListInput struct {
	Status   Status
	Assignee string
	Claimant string
	// Involves matches tasks the address created, is assigned or holds.
	Involves string
	// Scope matches the address and everything below it.
	Scope string
	// OpenOnly leaves out finished tasks.
	OpenOnly bool
	Limit    int    // default DefaultListLimit, max MaxListLimit
	Cursor   string // NextCursor of the previous page
}

// List limits.
const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

// ListResult is a page of tasks, newest-updated first.
type ListResult struct {
	Items      []Task `json:"items" jsonschema:"Tasks, newest-updated first."`
	NextCursor string `json:"next_cursor,omitempty" jsonschema:"Pass as cursor for the next page; absent on the last page."`
}

// UpdateInput is the argument of Update. Fields beyond TaskID and Action are
// read only by the actions that use them.
type UpdateInput struct {
	TaskID string
	Action Action
	// Note is the comment for comment and an optional reason for any action.
	Note string
	// Assignee is the identity for assign.
	Assignee string
	// MessageID is the message for link.
	MessageID string
	// Edit fields; nil leaves a field unchanged.
	Title    *string
	Body     *string
	Priority *Priority
	Scope    *string
	DueAt    *time.Time
	ClearDue bool
}

// Counts is an identity's open work, for hook nudges.
type Counts struct {
	Held     int // claimed or started by the identity
	Stale    int // of Held, stale or worse
	Assigned int // assigned to the identity and not yet claimed
}

// Errors. Use errors.Is; returned errors wrap these with a message that tells
// the caller how to fix the call.
var (
	ErrNotFound   = errors.New("tasks: task not found")
	ErrForbidden  = errors.New("tasks: not allowed")
	ErrTransition = errors.New("tasks: invalid status change")
)
