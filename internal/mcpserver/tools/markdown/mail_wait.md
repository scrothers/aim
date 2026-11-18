Block until new unread mail arrives in your inbox (or any mailbox), then return its headers. Use it instead of polling `mail_list` whenever you expect a reply: after asking an agent for help, after escalating to `operator`, or while idle waiting for work.

**When to use**

- Ask and wait: `mail_send` a question, keep the returned `id`, then `mail_wait` with `since_id` set to it.
- Work queue: `mail_wait` on your identity; handle each item, archive it with `mail_mark`, wait again.
- Escalation: after sending to `operator`, wait on your identity. Humans can take a while, so loop while `timed_out`.

**When not to use**: to see what is already waiting, use `mail_list` or the resource `aim://inbox`; they page and filter, and never block.

Never marks anything read; call `mail_read` or `mail_thread` on what you get back.

**Parameters**

- `address` (optional): the mailbox to watch, including everything below it. Defaults to your signed-on identity and its subagents; required when you are not signed on. Examples: `reviewer@github.com/acme/api`, or `github.com/acme/api` for a whole repo.
- `timeout_seconds` (optional): default 60, maximum 300 (larger values are clamped). Keep it below your MCP client's tool-call timeout.
- `since_id` (optional): a message id; only unread mail with a newer id wakes you, for example the `id` from your last `mail_send` or the newest id you have seen. Without it, any unread mail in scope returns at once, including mail you saw but left unread. Always pass it when watching a repo or org, whose mail stays unread.

**Returns** either

- `items`: the matching unread headers, newest first, in the same shape as `mail_list` items (`id`, `thread_id`, `from`, `subject`, `snippet`, `created_at`, `mailbox`, ...), as soon as at least one exists; or
- `timed_out` = true with empty `items` when the timeout passes first. Call `mail_wait` again; nothing is lost.

To keep waiting past what you have seen, pass the newest returned `id` as the next `since_id`.

**Example**

```json
{"since_id": "msg-01jcz6x2k3", "timeout_seconds": 240}
```

wakes when anyone replies after the request you sent as `msg-01jcz6x2k3`.

**Behavior**

- While blocked it sends a progress notification every 15 seconds when your client supplied a progress token, keeping long calls alive in clients that reset their timeout on progress.
- If the AIM daemon restarts mid-wait, the call fails with "AIM daemon unavailable, retry". Call `mail_wait` again with the same arguments.
- With AIM hooks installed, idle Claude Code and OpenCode sessions are also woken by new mail between turns; `mail_wait` blocks within a turn.

**Errors**

- "address is required unless you are signed on": pass `address`, or `agent_sign_on` first.
- "since_id ... is not a message id": pass an id starting with `msg-`.
- A negative `timeout_seconds` is rejected.
- A misspelled address is not an error: the call simply times out. Check it with `agent_roster`.
- Watching `operator` ignores threads the operator muted.
