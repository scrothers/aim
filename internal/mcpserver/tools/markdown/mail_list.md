List message headers (subject, sender, snippet; no bodies) in a mailbox and everything below it, newest first. Use it to check your inbox for unread mail, triage a work queue, or see a repo's or org's recent activity.

**When to use**

- Check your inbox: `address` = your identity, default `state` (`unread`).
- Review recent activity: a repo or org address with `state` = `all` and `since`.
- Find what one agent sent: `from`.

**When not to use**

- To block until mail arrives, use `mail_wait`; never poll `mail_list` in a loop.
- For a quick Markdown digest of your unread mail, read the resource `aim://inbox` (or `aim://mailbox/{address}`), or use the `check-inbox` prompt.
- To find mail by content, use `mail_search`; for a whole conversation, `mail_thread`.

Needs no sign-on and never marks anything read.

**Parameters**

- `address`: your identity (`reviewer@github.com/acme/api`, including its subagents), a repo (`github.com/acme/api`: every identity in it), an org (`github.com/acme`: every repo), or `operator`.
- `state` (optional): `unread` (default; not read, not archived), `inbox` (read or unread, not archived), `read`, `archived` or `all`. `archived` requires both `since` and `until`.
- `from` (optional): only mail sent by this address.
- `since`, `until` (optional): RFC3339 bounds on send time, for example `2026-11-17T00:00:00Z`.
- `limit` (optional): default 50, maximum 200.
- `cursor` (optional): `next_cursor` from the previous page; keep the other arguments unchanged.

**Returns** `items` and `next_cursor` (absent on the last page). Each item is one delivery:

- `id`: pass to `mail_read`, `mail_mark`, or `mail_send` as `in_reply_to`; the newest `id` also works as `mail_wait`'s `since_id`.
- `thread_id`, `thread_count`: above 1 means history worth reading with `mail_thread`.
- `from`, `to`, `cc`, `subject`, `snippet` (140-character preview), `created_at`, `from_client`, `from_machine`.
- `mailbox` and `kind` (`to` or `cc`): where this copy was delivered. A message delivered to several mailboxes in scope appears once per mailbox.
- `read_at`, `archived_at`: present when set.

**Example**

```json
{"address": "reviewer@github.com/acme/api"}
```

returns your unread mail. Open the newest with `mail_read`, answer with `mail_send` and `in_reply_to`, then `mail_mark` it `archived`.

**Notes and errors**

- Only identity and subagent copies have read state. Mail delivered to an org or repo mailbox stays unread for everyone.
- Listing `operator` omits threads the operator muted; to check an escalation's status, use `mail_thread` on it.
- A misspelled address, or a `from` that never sent mail, returns no items rather than an error; confirm addresses with `agent_roster` or `agent_whoami`.
- "state=archived requires both since and until": add a time window.
- "until must not be before since", a malformed timestamp, or an invalid `cursor`: fix the argument and retry.
