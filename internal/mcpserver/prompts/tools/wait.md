Block until new unread mail arrives, then return it. Use it instead of calling `list` in a loop whenever you expect a reply: after asking another agent for help, after escalating to `operator`, or while idle and waiting for work.

**Parameters**

- `address` (optional): the mailbox to watch, covering everything below it. Defaults to your signed-on identity, including its subagents. Anonymous callers must pass one. Examples: `reviewer@github.com/acme/api`, or `github.com/acme/api` to watch a whole repo.
- `timeout_seconds` (optional): how long to block. Default 60, maximum 300. Keep it below your MCP client's tool-call timeout.
- `since_id` (optional): only wake for messages with an id newer than this, for example the `id` returned by your last `send`, or the newest id you have already seen. Without it, any unread message in scope returns immediately.

**Returns** either

- `items`: the matching unread headers, in the same shape as `list` (`id`, `thread_id`, `from`, `subject`, `snippet`, `created_at`, `mailbox`, ...), as soon as at least one arrives; or
- `timed_out` = true with no items when the timeout passes first.

Waiting never marks anything read. Call `read` or `thread` on what you get back.

**Patterns**

- Ask and wait: `send` a question, keep its returned `id`, then `wait` with `since_id` set to it. Loop while `timed_out`.
- Work queue: `wait` on your identity with no `since_id`; handle each message, archive it with `mark`, then wait again.
- Escalation: after sending to `operator`, wait on your identity with a long timeout.
**Behavior**

- While blocked, it sends a progress notification every 15 seconds when your client supplied a progress token, which keeps long calls alive.
- If the AIM daemon restarts while you wait, the call fails with "AIM daemon unavailable, retry". Call `wait` again with the same arguments; nothing is lost.

**Errors**

- Called anonymously without `address`: pass an address or `sign_on` first.
- An unknown identity fails with suggestions.
