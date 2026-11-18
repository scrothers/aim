package mdview

import (
	"fmt"
	"strings"

	"github.com/scrothers/aim/internal/tasks"
)

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// TaskNudge renders the one-line open-work reminder for hook context, or ""
// when the identity holds and is assigned nothing.
func TaskNudge(c tasks.Counts) string {
	if c.Held == 0 && c.Assigned == 0 {
		return ""
	}
	var parts []string
	if c.Held > 0 {
		s := "hold " + plural(c.Held, "task", "tasks")
		if c.Stale > 0 {
			s += fmt.Sprintf(" (%d stale)", c.Stale)
		}
		parts = append(parts, s)
	}
	if c.Assigned > 0 {
		parts = append(parts, "have "+plural(c.Assigned, "assigned task", "assigned tasks")+" to claim")
	}
	return "You " + strings.Join(parts, " and ") + "; `task_list` shows them.\n"
}

// taskLine renders a task as one bullet with its key facts.
func taskLine(t tasks.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- **%s** `%s` - %s", OneLine(t.Title), t.ID, t.Status)
	if t.Health != "" {
		fmt.Fprintf(&b, " (%s)", t.Health)
	}
	if t.Priority != tasks.PriorityNormal && t.Priority != "" {
		fmt.Fprintf(&b, ", %s priority", t.Priority)
	}
	if t.Claimant != "" {
		fmt.Fprintf(&b, ", held by %s", t.Claimant)
	}
	if t.Assignee != "" {
		fmt.Fprintf(&b, ", assigned to %s", t.Assignee)
	}
	if t.Scope != "" {
		fmt.Fprintf(&b, ", scope %s", t.Scope)
	}
	if t.DueAt != nil {
		fmt.Fprintf(&b, ", due %s", Stamp(*t.DueAt))
	}
	b.WriteString("\n")
	return b.String()
}

// TaskSection is a titled group of tasks in a digest.
type TaskSection struct {
	Title string
	Empty string // shown when Tasks is empty
	Tasks []tasks.Task
}

// TaskDigest renders an identity's open work as sections of task bullets.
func TaskDigest(title string, sections []TaskSection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", title)
	for _, s := range sections {
		fmt.Fprintf(&b, "\n## %s\n\n", s.Title)
		if len(s.Tasks) == 0 {
			fmt.Fprintf(&b, "_%s_\n", s.Empty)
			continue
		}
		for _, t := range s.Tasks {
			b.WriteString(taskLine(t))
		}
	}
	b.WriteString("\nRead one with `aim://task/{task_id}` or `task_get`; change it with `task_update`.\n")
	return b.String()
}

// TaskDetail renders a task with its history and linked messages.
func TaskDetail(d tasks.Detail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", OneLine(d.Title))
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "- %s: %s\n", k, v)
		}
	}
	row("id", d.ID)
	status := string(d.Status)
	if d.Health != "" {
		status += " (" + string(d.Health) + ")"
	}
	row("status", status)
	row("priority", string(d.Priority))
	row("scope", d.Scope)
	row("created by", d.CreatedBy)
	row("assignee", d.Assignee)
	row("claimant", d.Claimant)
	if d.DueAt != nil {
		row("due", Stamp(*d.DueAt))
	}
	if d.LastActivityAt != nil {
		row("last activity", Stamp(*d.LastActivityAt))
	}
	if d.FinishedAt != nil {
		row("finished", Stamp(*d.FinishedAt))
	}
	row("created", Stamp(d.CreatedAt))
	row("updated", Stamp(d.UpdatedAt))
	if body := strings.TrimSpace(d.Body); body != "" {
		fmt.Fprintf(&b, "\n%s\n", body)
	}
	b.WriteString("\n## History\n\n")
	if len(d.Events) == 0 {
		b.WriteString("_No history._\n")
	}
	for _, e := range d.Events {
		fmt.Fprintf(&b, "- %s %s by %s", Stamp(e.At), e.Kind, e.Actor)
		if e.FromStatus != "" || e.ToStatus != "" {
			if e.FromStatus == "" {
				fmt.Fprintf(&b, " (-> %s)", e.ToStatus)
			} else {
				fmt.Fprintf(&b, " (%s -> %s)", e.FromStatus, e.ToStatus)
			}
		}
		if e.Note != "" {
			fmt.Fprintf(&b, ": %s", OneLine(e.Note))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Linked messages\n\n")
	if len(d.Links) == 0 {
		b.WriteString("_None. Link one with `send` (task_id) or `task_update` (action link)._\n")
	}
	for _, l := range d.Links {
		fmt.Fprintf(&b, "- **%s** (id `%s`, thread `%s`), linked by %s %s\n",
			OneLine(l.Subject), l.MessageID, l.ThreadID, l.LinkedBy, Stamp(l.LinkedAt))
	}
	return b.String()
}
