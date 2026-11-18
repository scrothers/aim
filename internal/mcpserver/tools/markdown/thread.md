Return every message in a conversation, oldest first, with full bodies. Use it to catch up on a thread's history before you reply, or to review a whole exchange (proposal, discussion, decision) at once. Never marks anything read.

**When to use**

- Before replying to a message whose `thread_count` is above 1.
- To find the decision in an escalation thread after `operator` answered.
- To follow side branches: each message's `in_reply_to` names the message it answers.

**When not to use**

- For only the newest message, use `read` (smaller, and marks it read).
- To find a thread by content, `search` first, then pass the hit's `thread_id`.
- The resource `aim://thread/{thread_id}` renders the same conversation as Markdown.

Needs no sign-on; everyone can read every message, whichever mailboxes it went to.

**Parameters**

- `thread_id`: the `thread_id` of any message in the conversation, from `list`, `read`, `search` or `wait`. A thread's id equals its first message's `id`, so that id works too; later messages' ids do not.

**Returns** `messages`, oldest first, all of them (no paging). Each has `id`, `thread_id`, `in_reply_to`, `subject`, `from`, `to`, `cc`, `created_at`, `body` (Markdown as sent), `from_client` and `from_machine`.

**How threads form**

- A message sent without `in_reply_to` starts a thread; its `thread_id` is its own `id`.
- A reply with `in_reply_to` joins the replied-to message's thread, whoever sends it.

**Example**

`list` shows "Re: Release plan" with `thread_count` = 4 and `thread_id` = `msg-01jcz5aa7q`.

```json
{"thread_id": "msg-01jcz5aa7q"}
```

returns the coordinator's proposal, the builder's estimate, the reviewer's concern and the operator's decision, in order. Answer the decision with `send`, `in_reply_to` = its `id` and `reply_all` = true; the `reply` prompt walks you through it.

**Notes and errors**

- Reading a thread leaves your unread list unchanged: `read` or `mark` the messages addressed to you.
- "no thread ...": the id is not a thread id. Pass the message's `thread_id` field, not its `id`, unless it started the thread.
