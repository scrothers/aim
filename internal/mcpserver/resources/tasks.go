package resources

import (
	"context"

	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/mdview"
	"github.com/scrothers/aim/internal/tasks"
)

// URITasks is the open-work digest of the signed-on identity.
const URITasks = "aim://tasks"

// PrefixTask starts the task template; the task id follows encoded.
const PrefixTask = "aim://task/"

// taskSectionLimit caps each section of the tasks digest.
const taskSectionLimit = 20

var (
	myTasks = resource{name: "tasks", title: "Your tasks", uri: URITasks, render: renderTasks}
	task    = resource{name: "task", title: "Task", prefix: PrefixTask, variable: "task_id", render: renderTask}
)

func renderTasks(ctx context.Context, c *session.Conn, _ string) (string, error) {
	board, err := c.TaskBoard()
	if err != nil {
		return "", err
	}
	list := func(in tasks.ListInput, keep func(tasks.Task) bool) ([]tasks.Task, error) {
		in.OpenOnly, in.Limit = true, tasks.MaxListLimit
		res, err := board.List(ctx, in)
		if err != nil {
			return nil, err
		}
		out := make([]tasks.Task, 0, taskSectionLimit)
		for _, t := range res.Items {
			if len(out) < taskSectionLimit && keep(t) {
				out = append(out, t)
			}
		}
		return out, nil
	}
	unclaimed := func(t tasks.Task) bool { return t.Assignee == "" && t.Claimant == "" }
	var sections []mdview.TaskSection
	id, signedOn := c.Entry.Identity()
	if signedOn {
		me := id.IdentityAddress().String()
		held, err := list(tasks.ListInput{Claimant: me}, func(tasks.Task) bool { return true })
		if err != nil {
			return "", err
		}
		assigned, err := list(tasks.ListInput{Assignee: me}, func(t tasks.Task) bool { return t.Claimant != me })
		if err != nil {
			return "", err
		}
		sections = append(sections,
			mdview.TaskSection{Title: "Held by you", Empty: "You hold no tasks.", Tasks: held},
			mdview.TaskSection{Title: "Assigned to you", Empty: "Nothing is assigned to you.", Tasks: assigned})
	}
	if ns := c.Entry.Preamble().Namespace; ns != "" {
		open, err := list(tasks.ListInput{Scope: ns}, unclaimed)
		if err != nil {
			return "", err
		}
		sections = append(sections, mdview.TaskSection{Title: "Open in " + ns, Empty: "No unclaimed open tasks here.", Tasks: open})
	}
	text := mdview.TaskDigest("Your tasks", sections)
	if !signedOn {
		text += "\nNot signed on, so nothing is yours yet: call `sign_on` to claim or create tasks.\n"
	}
	return text, nil
}

func renderTask(ctx context.Context, c *session.Conn, id string) (string, error) {
	board, err := c.TaskBoard()
	if err != nil {
		return "", err
	}
	d, err := board.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return mdview.TaskDetail(d), nil
}
