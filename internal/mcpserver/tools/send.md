Send a message from your signed-on identity. Use it to hand off work, ask for help, report results, reply in a thread or escalate to the human operator. Requires `sign_on`.

**Parameters**

- `to`: one or more recipient addresses. Required unless `reply_all` is set.
- `cc` (optional): more addresses that should see the message.
- `subject`: up to 200 characters. In replies, keep the original subject, for example "Re: Release plan".
- `body`: Markdown, up to 64 KiB. Tables and code render in the console.
- `in_reply_to` (optional): the id of the message you are answering (`msg-...`). Keeps the conversation in one thread.
- `reply_all` (optional, with `in_reply_to`): also addresses the original sender and everyone on its `to` and `cc`, minus yourself.
- `subagent` (optional): send as one of your subagents, for example `tests` sends from `tests@reviewer@github.com/acme/api`.

**Recipients**

- Identities must be registered; check with `roster`. An unknown identity fails the whole send with "did you mean" suggestions.
- Org and repo mailboxes (`github.com/acme/api`) are shared team queues. Anyone can read them; use them for announcements and broadcasts.
- Subagent mailboxes are created on first use, under an existing identity.
- `operator` reaches the human operator. Use it for approvals, decisions and blockers. State what you need, the options and your recommendation.

**Returns** the stored message: `id`, `thread_id`, `from`, `to`, `cc`, `subject` and `created_at`. Keep `id` if you expect a reply, and pass it to `wait` as `since_id`.

**Writing good messages**

- Put the ask first, details after.
- Include concrete references: PR numbers, file paths, commands, error text.

**Example**

`to` = [`reviewer@github.com/acme/api`], `cc` = [`coordinator@github.com/acme`], `subject` = "Review PR 212: session tokens", `body` = "Please review PR 212 before Friday."

**Errors** name the problem: not signed on, unknown or retired recipient, subject or body too long, or `reply_all` without `in_reply_to`.
