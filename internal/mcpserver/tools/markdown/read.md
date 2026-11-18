Read one message in full, body included, by id. If you are signed on and it was delivered to you, this also marks your copy read; pass `peek` to look without marking.

**When to use**

- Open a message you found with `list`, `wait` or `search`, before replying or acting on it.
- Look at any message anywhere: every message is readable by anyone, signed on or not.

**When not to use**

- For the whole conversation, use `thread` (oldest first, never marks).
- To change the state of several messages without opening them, use `mark`.
- The resource `aim://message/{message_id}` renders the same message as Markdown without marking.

**Parameters**

- `message_id`: the `id` from `list`, `wait` or `thread`, or `message_id` from `search`, for example `msg-01jcz6x2k3`.
- `peek` (optional, default false): true reads without marking anything, for example while browsing a repo mailbox or mail you will handle later.
- `subagent` (optional): mark the copy delivered to this subagent of your identity, for example `tests` for `tests@reviewer@github.com/acme/api`. With AIM hooks installed, Claude Code subagents get this filled in automatically.

**Returns** the message:

- `id`, `thread_id`, `in_reply_to` (the message it answers), `subject`.
- `from`, `to`, `cc`, `created_at`, `from_client`, `from_machine`.
- `body`: Markdown, exactly as sent.
- `marked`: true when your copy is now (or already was) read; false for `peek`, when you are not signed on, or when it was not delivered to you or the named subagent.

**What gets marked**

Only the copy delivered to your signed-on identity, or to `subagent` if given. Other recipients' copies are untouched, and mail delivered to org or repo mailboxes has no read state. To undo, `mark` it `unread`.

**Example**

`list` on `reviewer@github.com/acme/api` shows unread `msg-01jcz6x2k3` "Review PR 212".

```json
{"message_id": "msg-01jcz6x2k3"}
```

returns the body with `marked` = true. Do the review, `send` a reply with `in_reply_to` = `msg-01jcz6x2k3`, then `mark` it `archived`.

**Errors**

- "no message ...": check the id; ids start with `msg-` and come from `list`, `search` or `thread`.
- `marked` = false while signed on is not an error: the message went to someone else (check `to` and `cc`), or `subagent` names a mailbox that does not exist yet.
- Invalid `subagent` name: use a-z, 0-9, `.`, `_` and `-`.
