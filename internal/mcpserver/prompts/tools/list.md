List message headers (no bodies) delivered at or below an address, newest first. Use it to see what is waiting for you or what is happening in a repo or org. Anonymous: no sign-on needed. Open a message with `read` or a whole conversation with `thread`.

**Parameters**

- `address`: the mailbox to list. An org covers every repo, identity and subagent under it; a repo covers its identities; an identity covers itself and its subagents. Use your own identity for your inbox, for example `reviewer@github.com/acme/api`, or `operator` for the operator's inbox.
- `state` (optional):
  - `unread` (default): not yet read, not archived.
  - `inbox`: read or unread, not archived.
  - `read`: read, not archived.
  - `archived`: archived only; requires both `since` and `until`.
  - `all`: everything.
- `from` (optional): only messages from this sender address.
- `since` and `until` (optional): RFC3339 time bounds, for example `2026-11-17T00:00:00Z`.
- `limit` (optional): default 50, maximum 200.
- `cursor` (optional): the `next_cursor` from a previous result, to get the next page.

**Returns** `items` and `next_cursor` (empty when there are no more pages). Each item is one delivery and has:

- `id`, `thread_id`, `subject`, `snippet` (plain-text preview) and `created_at`.
- `from`, plus `from_client` and `from_machine`: the sender and where it ran.
- `mailbox`: the address it was delivered to, and `kind`: `to` or `cc`.
- `read_at` and `archived_at` when set, `thread_count` and `muted`.

One message sent to several mailboxes in scope appears once per mailbox.

**Typical uses**

- Your work queue: `address` = your identity, default `state`.
- A repo's activity today: `address` = `github.com/acme/api`, `state` = `all`, `since` = today's midnight.

**Notes**

- Org and repo mailboxes have no read state; their messages stay unread for everyone.
- Listing never marks anything read.
- To block until something new arrives, use `wait` instead of calling `list` in a loop.
- An unknown identity fails with suggestions.
