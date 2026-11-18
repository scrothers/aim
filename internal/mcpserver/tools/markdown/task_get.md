Get one task from AIM's task board with its full history (every claim, status change, comment and lease event) and the messages linked to it.

**When to use**

- Before claiming or resuming a task: read its body, comments and linked threads so you continue where the last holder stopped.
- After a "[AIM] Task going stale" or "[AIM] Task abandoned" notice, or an assignment mail, to see the details.
- To find the conversation about a task: each link names a `thread_id` for `mail_thread`.

**When not to use**: to find tasks use `task_list`. The resource `aim://task/{task_id}` returns the same detail as Markdown.

Needs no sign-on.

**Parameters**

- `task_id`: the `task-...` id from `task_list`, `task_create`, a notice or a resource.

**Returns** the task fields (as in `task_list`: `id`, `title`, `body`, `status`, `priority`, `scope`, `created_by`, `assignee`, `claimant`, `health`, timestamps) plus:

- `events`: history, oldest first. Each has `at`, `actor` (identity or `operator`), `kind` (`created`, an action name such as `claim` or `comment`, `stale`, or `decayed` when a lease lapsed), `from_status` and `to_status` when the status changed, and `note` (the comment, reason, assignee or linked message).
- `links`: linked messages, oldest first, each with `message_id`, `thread_id`, `subject`, `linked_by`, `linked_at`. Open one with `mail_read` or the whole conversation with `mail_thread`.

**Example**

```json
{"task_id": "task-01jd8x4q2v7k3m5n6p8r9s0t1u"}
```

Then claim it with `task_update` if it is `not_started` or `abandoned`, or comment on it.

**Reading health**

- `active`: the claimant's session is working (a tool call or hook within the idle threshold).
- `idle`: quiet for a while; normal during long builds.
- `stale`: quiet long enough that the claimant and creator were mailed once.
- `abandoned`: the lease lapsed and the claimant's session is gone (or the maximum hold passed); the sweeper is releasing it.

**Errors**

- "task not found": check the id with `task_list`; ids are lowercase and start with `task-`.
