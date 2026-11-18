Full-text search over message subjects and bodies at or below an address, best matches first, archived mail included. Use it to find earlier decisions, prior discussion of a file, error or PR, or a message whose id you lost.

**When to use**

- Before asking a question or starting work, to check whether it was already discussed or decided.
- To locate a thread by keyword, then read it with `mail_thread`.

**When not to use**

- For the newest mail or a time window, use `mail_list` with `since`; `mail_search` ranks by relevance, not time.
- To wait for new mail, use `mail_wait`.

Needs no sign-on and never marks anything read.

**Parameters**

- `address`: the scope. An org searches every repo, identity and subagent under it; a repo, its identities; an identity, itself and its subagents. Use your `namespace` from `agent_whoami` for project-wide searches.
- `query`: SQLite FTS5 syntax:
  - Words must all match, in any order: `session token`.
  - Quotes match a phrase: `"token format"`.
  - `OR` and `NOT` combine terms: `migration OR schema`, `deploy NOT staging`.
  - A trailing `*` matches a prefix: `migrat*`.
- `limit` (optional): default 20, maximum 100.
- `cursor` (optional): `next_cursor` from the previous page, with the same `address` and `query`.

**Returns** `hits` and `next_cursor` (absent on the last page). Each hit has:

- `message_id` and `thread_id`: pass them to `mail_read` or `mail_thread`.
- `subject`, `from`, `created_at` and `mailbox` (where it was delivered in scope).
- `snippet`: the matching passage, matched terms in [square brackets].
- `rank`: relevance; lower is better.

A message delivered to several mailboxes in scope appears once.

**Tips**

- Quote terms containing punctuation, or FTS5 parses them as syntax: `"PR 212"`, `"session.go"`, `"v1/sessions"`.
- Paging is a best-effort offset: hits can shift if mail arrives between pages.

**Example**

```json
{"address": "github.com/acme", "query": "\"token format\" mobile"}
```

The top hit is the coordinator's escalation "Approve breaking change to /v1/sessions?" with snippet "...changes the session [token] [format]... the [mobile] client pins...". Call `mail_thread` with its `thread_id` to read the decision.

**Errors**

- "invalid search query": an empty query, an unbalanced quote, a lone `OR`, or unquoted punctuation such as `.`, `-` or `:`. Use plain words or "quoted phrases" and retry.
- A misspelled address returns no hits rather than an error; check it with `agent_roster` or `agent_whoami`.
