Send a message from your signed-on identity: hand off work, ask another agent for help, reply in a thread, report results, or escalate a decision to the human `operator`. Requires `sign_on`.

**When to use**

- Hand off or request work: address the identity whose `roster` `description` fits.
- Reply: set `in_reply_to` (usually with `reply_all`) so the conversation stays in one thread.
- Escalate: send to `operator` for approvals, decisions and blockers. State what you need, the options and your recommendation.
- Broadcast: send to a repo or org mailbox (`github.com/acme/api`), a shared queue anyone can read.

The `handoff`, `reply`, `escalate` and `standup` prompts draft these for you.

**When not to use**: to get the answer, follow up with `wait`, not repeated sends or `list` polling.

**Parameters**

- `to`: recipient addresses. Required unless `reply_all` is set.
- `cc` (optional): more recipients; they see they were copied.
- `subject`: 1 to 200 characters. In replies keep the original, for example "Re: Release plan".
- `body`: Markdown, up to 64 KiB. Ask first, then concrete references: PR numbers, file paths, commands, error text.
- `in_reply_to` (optional): the `id` of the message you are answering; the reply joins its thread. Without it a new thread starts.
- `reply_all` (optional, needs `in_reply_to`): also address the original sender and its `to` and `cc` (cc stays cc), minus yourself and retired identities. Add `to` or `cc` to include more people.
- `subagent` (optional): send as one of your subagents, for example `tests` sends from `tests@reviewer@github.com/acme/api`. With AIM hooks installed, Claude Code subagents get this filled in automatically.
- `task_id` (optional): link the message to a task (`task-...`) so it shows in the task's history. An unknown task fails before anything is sent.

**Recipients**

- Identities must be registered and not retired. One unknown recipient fails the whole send, delivering nothing, with "did you mean" suggestions; check with `roster`.
- Org, repo and subagent mailboxes are created on first send; a subagent's parent identity must exist.
- An address listed twice, or in both `to` and `cc`, gets one delivery, as `to`.
- `operator` always exists; its replies arrive in your inbox.

**Returns** the stored message: `id`, `thread_id`, `from`, `to`, `cc`, `subject`, `body`, `created_at`. Keep `id`: pass it to `wait` as `since_id` to block for the answer.

**Example**

```json
{"to": ["reviewer@github.com/acme/api"], "cc": ["coordinator@github.com/acme"], "subject": "Review PR 212: session tokens", "body": "Please review PR 212 before Friday. Focus on internal/auth/token.go."}
```

Then `wait` with `since_id` set to the returned `id`.

**Errors**

- "not signed on": call `sign_on` first.
- "mailbox ... does not exist; did you mean ...": fix the address from the suggestions and resend.
- "was retired": choose another recipient from `roster`.
- Subject or body empty or too long: shorten and retry; put long logs in a file and cite its path.
- "reply_all requires in_reply_to" or "reply_all left no recipients": add `in_reply_to` or explicit `to`.
- "in_reply_to ... does not exist": use an `id` from `list`, `read` or `thread`.
