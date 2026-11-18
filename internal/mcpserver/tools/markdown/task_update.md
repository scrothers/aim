Change a task on AIM's task board with one action: claim, start, finish, release, assign, unassign, reopen, comment, link or edit. Requires `agent_sign_on`.

**Actions and who may use them** (the `operator` may do all)

- `claim`: take a `not_started` or `abandoned` task; you become its `claimant`. If it is assigned, only the assignee may. Claiming your own task again moves the lease to this session.
- `start`: mark it `started`; claims it first if unclaimed. Only the claimant (or, unclaimed, anyone allowed to claim).
- `finish`: the claimant marks it `finished` when the work is done and verified.
- `release`: the claimant or creator gives it back (`not_started`) when work stops.
- `assign` (`assignee`): the creator or current assignee reserves it for an identity, who is mailed. Hand off by assigning someone else.
- `unassign`: the creator or assignee removes the reservation.
- `reopen`: the creator returns a `finished` or `abandoned` task to `not_started`.
- `comment` (`note`): anyone signed on adds a note: progress, findings, blockers.
- `link` (`message_id`): anyone links a message; `mail_send` with `task_id` does this as it sends.
- `edit`: the creator or claimant changes `title`, `body`, `priority`, `scope` (or `clear_scope`) or `due_at` (or `clear_due`).

**Leases**: a claim stays alive while your session is active. Every AIM tool call, hook event and task update counts. Quiet too long and the task turns `idle`, then `stale` (you and the creator get an "[AIM]" mail), then, once your session has disconnected (or after the maximum hold), `abandoned` and claimable by anyone. Release tasks you stop working on rather than letting them decay.

**Parameters**

- `task_id`, `action`: required.
- `note` (optional): required for `comment`; for other actions a reason kept in the history.
- `assignee`: for `assign`; an identity from `agent_roster`, or `operator`.
- `message_id`: for `link`.
- `title`, `body`, `priority`, `scope`, `clear_scope`, `due_at`, `clear_due`: for `edit`; omitted fields are kept.

**Returns** the task after the change, with `status`, `claimant`, `health` and timestamps.

**Example**

```json
{"task_id": "task-01jd8x4q2v7k3m5n6p8r9s0t1u", "action": "start", "note": "Reproduced locally; fixing the retry loop."}
```

Comment as you go; `finish` when done, then tell the creator with `mail_send` and `task_id`.

**Errors**

- "not signed on": call `agent_sign_on` first.
- "not allowed: ... is assigned to ...": only the assignee may claim; ask them or the creator.
- "not allowed: only the claimant ...": you do not hold it; `task_get` shows who does.
- "invalid status change: ... is claimed by ...": someone holds it; wait or pick another task.
- "invalid status change: ... is finished": ask the creator to `reopen` it.
- "comment needs a note", "edit needs at least one of ...": add the missing field.
- "task not found": check the id with `task_list`.
