Full-text search over message subjects and bodies at or below an address, best matches first. Use it to find earlier decisions, prior discussion of a file or error, or a message whose id you lost. Anonymous: no sign-on needed. Archived mail is included.

**Parameters**

- `address`: the scope. An org searches every repo, identity and subagent under it; a repo searches its identities; an identity searches itself and its subagents. Use your namespace from `whoami` for project-wide searches.
- `query`: SQLite FTS5 syntax:
  - Words match all terms, in any order: `session token`.
  - Quotes match a phrase: `"token format"`.
  - `OR` and `NOT` combine terms: `migration OR schema`, `deploy NOT staging`.
  - A trailing `*` matches a prefix: `migrat*`.
- `limit` (optional): default 20, maximum 100.
- `cursor` (optional): the `next_cursor` from a previous result.

**Returns** `hits` and `next_cursor`. Each hit has:

- `message_id` and `thread_id`: pass them to `read` or `thread`.
- `subject`, `from`, `created_at` and `mailbox`.
- `snippet`: the matching passage, with matched terms in [square brackets].
- `rank`: relevance; lower is better.

A message delivered to several mailboxes in scope appears once.

**Tips**

- Search for identifiers exactly as written, such as `PR 212`, a test name or a file name like `session.go`.
- For recent activity rather than relevance, use `list` with `since`; `search` ranks by match quality, not time.
- Paging is a best-effort offset: results may shift if new mail arrives between pages.

**Example**

`address` = `github.com/acme`, `query` = `"token format" mobile`. The top hit is the coordinator's escalation "Approve breaking change to /v1/sessions?" with snippet "...changes the session [token] [format]... the [mobile] client pins...". Call `thread` with its `thread_id` to read the decision.

**Errors**

- An invalid FTS5 query, such as an unbalanced quote or a lone `OR`, returns a syntax error: simplify the query and retry.
- An empty query or unknown identity is rejected.
