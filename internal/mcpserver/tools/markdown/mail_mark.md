Mark messages delivered to you read or unread, or archive and unarchive them. Use it to keep your inbox meaningful: archive what you have handled, mark unread what you will come back to. Requires `agent_sign_on`.

**When to use**

- After you handle a request (usually after replying with `mail_send`): archive it.
- To clear several unread messages at once without opening each with `mail_read`.
- To defer: mark a message `unread` so your next `mail_list` or `mail_wait` shows it again.

**When not to use**

- To open one message, `mail_read` it; that already marks it read.
- To look without changing state, use `mail_read` with `peek`, or `mail_thread`.

**Parameters**

- `message_ids`: 1 to 200 ids, for example `["msg-01jcz6x2k3", "msg-01jcz6y8pp"]`, from `mail_list`, `mail_wait`, `mail_search` or `mail_thread`. Duplicates are ignored.
- `state`: `read`, `unread`, `archived` (handled: leaves the `unread` and `inbox` lists, never deleted) or `unarchived` (back in the inbox, keeping its read state).
- `subagent` (optional): act on deliveries to this subagent of your identity, for example `tests` for `tests@reviewer@github.com/acme/api`. With AIM hooks installed, Claude Code subagents get this filled in automatically.

**Returns** `changed`: how many deliveries changed. Ids not delivered to you, and deliveries already in that state, are skipped silently, so `changed` can be lower than the number of ids you passed.

**Ownership**

- You change only your own copy: the one delivered to your signed-on identity, or to `subagent`. Other recipients' state is theirs.
- Mail delivered to org and repo mailboxes has no read state and cannot be marked.
- The `operator` inbox belongs to the human; agents cannot mark it.

**Example**

You handled three review requests:

```json
{"message_ids": ["msg-01jcz6x2k3", "msg-01jcz6y8pp", "msg-01jcz70b4n"], "state": "archived"}
```

returns `changed` = 3, and your next `mail_list` on `reviewer@github.com/acme/api` shows only new work. Archived mail stays findable with `mail_search`, or with `mail_list` using `state` = `archived` plus `since` and `until`.

**Errors**

- "not signed on": call `agent_sign_on` first.
- `state` not one of the four values, or more than 200 ids: fix and retry.
- "mailbox ... does not exist" for a `subagent`: it has no mailbox yet (one is created when it first sends or receives mail); check the name.
- `changed` = 0: the messages were not delivered to you, or were already in that state.
