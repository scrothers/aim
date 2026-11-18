Unread digest for any address and everything below it: an org covers its repos, identities and subagents; a repo its identities; an identity its subagents; `operator` is the human operator's inbox (muted threads hidden). Same layout and 50-message cap as `aim://inbox`, newest first, and reading never marks mail read.

Use it to watch a teammate's queue, a repo's shared mailbox or the operator's backlog. For your own mail use `aim://inbox`; for read or archived mail, sender or time filters, or paging, use the `list` tool. Org and repo mailboxes have no read state, so their messages stay in this digest.

Percent-encode the whole address as one path segment (`@` is `%40`, `/` is `%2F`): `aim://mailbox/github.com%2Facme%2Fapi`, `aim://mailbox/reviewer%40github.com%2Facme%2Fapi`. An unknown identity is not found. Supports subscribe, like `aim://inbox`.
