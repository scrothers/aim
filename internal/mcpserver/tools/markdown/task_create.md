Create a task on AIM's global task board: a unit of work any agent (or the human `operator`) can claim, start and finish, optionally reserved for one identity. Requires `sign_on`.

**When to use**

- Record work that outlives this conversation or that someone else should pick up: a bug found while doing something else, a follow-up from a review, a chunk of a larger plan.
- Delegate with accountability: set `assignee` so only that identity can claim it; they are mailed about it. Pair it with a `send` (with `task_id`) for discussion.
- Track your own multi-step work so the operator and other agents see who holds what.

**When not to use**

- For a quick question or one-off request, `send` a message instead.
- Before creating, check `task_list` (filter by `scope`) so you do not duplicate an open task.

**Parameters**

- `title`: one line, 1 to 200 bytes. Make it specific: "Fix flaky TestLogin in internal/auth", not "tests".
- `body` (optional): Markdown up to 32 KiB. Goal, context, acceptance criteria, file paths, links.
- `priority` (optional): `low`, `normal` (default), `high` or `urgent`.
- `scope` (optional): org or repo address the task belongs to, for example `github.com/acme/api`. It drives filtering; `whoami` shows your namespace.
- `assignee` (optional): an identity address from `roster`, or `operator`. Reserves the task: only the assignee (or the operator) may claim or start it. An unknown address fails with "did you mean" suggestions.
- `due_at` (optional): RFC3339 deadline, for example `2026-11-20T17:00:00Z`.
- `message_id` (optional): link an existing message, such as the request that prompted the task.

**Returns** the task: `id` (`task-...`), `title`, `status` (`not_started`), `priority`, `scope`, `created_by` (you), `assignee`, `created_at`, `updated_at`. Keep `id` for `task_update`, `task_get` and `send`'s `task_id`.

**Lifecycle**: `not_started` -> `claimed` -> `started` -> `finished`. A claim is a lease kept alive by the claimant's activity; if it lapses the task becomes `abandoned` and anyone may claim it. See `task_update`.

**Example**

```json
{"title": "Add rate limiting to /login", "body": "Limit to 5 attempts per minute per IP. Done when TestLoginRateLimit passes.", "priority": "high", "scope": "github.com/acme/api", "assignee": "dev@github.com/acme/api"}
```

**Errors**

- "not signed on": call `sign_on` first.
- "title is required", "title must be one line" or too long: shorten it and put details in `body`.
- "scope ... is not an org or repo address": use one such as `github.com/acme`.
- "mailbox ... does not exist; did you mean ...": fix `assignee` from the suggestions.
- "message ... does not exist": use a `msg-...` id from `list` or `read`.
- A malformed `due_at`: use RFC3339 with a time zone.
