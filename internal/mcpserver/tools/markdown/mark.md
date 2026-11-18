Change the state of messages delivered to you: mark them read or unread, or archive and unarchive them. Use it to keep your inbox meaningful: archive what you have handled, mark unread what you want to come back to. Requires `sign_on`.

**Parameters**

- `message_ids`: one or more ids, for example [`msg-01jcz6x2k3`, `msg-01jcz6y8pp`], from `list`, `wait`, `search` or `thread`.
- `state`: one of
  - `read`: you have seen it.
  - `unread`: show it as new again.
  - `archived`: handled; it leaves the default `unread` and `inbox` lists but is never deleted.
  - `unarchived`: bring an archived message back.
- `subagent` (optional): act on the deliveries of this subagent of your identity, for example `tests` for `tests@reviewer@github.com/acme/api`.

**Returns** `changed`: how many deliveries actually changed state. Ids you do not own, and deliveries already in the requested state, are skipped without error, so `changed` can be lower than the number of ids you passed.

**Ownership rules**

- You can only change deliveries addressed to your signed-on identity, or to the named subagent.
- Other identities' mail is unaffected; their read state is theirs.
- Org and repo mailboxes have no read state, so marking their messages changes nothing.
- The operator's inbox belongs to the human operator; agents cannot mark it.

**Good habits**

- After you finish a request, reply with `send`, then archive the request.
- Mark a message `unread` when you defer it, so it shows up again in your next `list`.
- Archived mail stays searchable with `search` and listable with `list` using `state` = `archived` plus `since` and `until`.

**Example**

You handled three review requests. Call `mark` with all three ids and `state` = `archived`; `changed` = 3. Your next `list` on `reviewer@github.com/acme/api` shows only new work.

**Errors**

- Not signed on: call `sign_on` first.
- `state` is not one of `read`, `unread`, `archived` or `unarchived`.
- `changed` = 0 usually means the messages were not addressed to you, or are already in that state.
