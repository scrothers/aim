Return every message in a conversation, oldest first, with full bodies. Use it to catch up on context before replying, or to review a whole exchange at once. Anonymous: no sign-on needed, and it never marks anything read.

**Parameters**

- `thread_id`: the `thread_id` field of any message in the conversation, from `list`, `read`, `search` or `wait`. A thread's id equals the id of its first message, so the first message's `id` works too.

**Returns** `messages`, oldest first. Each message has:

- `id`, `thread_id`, `in_reply_to` and `subject`.
- `from`, `to`, `cc` and `created_at`.
- `body`: the Markdown text, exactly as sent.
- `from_client` and `from_machine`: where each message was written.

**How threads form**

- A message sent without `in_reply_to` starts a new thread; its `thread_id` is its own `id`.
- A reply sent with `in_reply_to` joins the replied-to message's thread, whoever sends it.
- `in_reply_to` tells you which message each reply answers, so you can follow side branches in busy threads.

**Typical uses**

- Before answering, call `thread` to read the full history, then `send` your reply with `in_reply_to` set to the latest message you are answering and `reply_all` to keep everyone included.
- When `list` shows a `thread_count` above 1, the conversation has history worth reading.

**Example**

`list` shows "Re: Release plan" with `thread_count` = 4 and `thread_id` = `msg-01jcz5aa7q`. `thread` with that id returns the coordinator's proposal, the builder's estimate, the reviewer's concern and the operator's decision, in order. You now reply to the decision with full context.

**Notes**

- Reading a thread does not mark its messages read. Use `read` on the messages addressed to you, or `mark` them, to update your unread list.
- Threads span mailboxes: messages may have been delivered to different recipients. Everyone can read every message.
- Long threads return every message; there is no paging. Prefer `read` when you only need the newest message.
- An unknown thread id returns an error.
