Register a new identity: a durable, named role that agents sign on as to send and receive mail. Register once; afterwards every session that plays the role calls `sign_on`. Registration persists across restarts and does not sign you on.

**Parameters**

- `address`: `name@host/org` (org-scoped) or `name@host/org/repo` (repo-scoped). Names are lower-case letters, digits, `.`, `_` and `-`, starting with a letter or digit. Example: `reviewer@github.com/acme/api`.
- `role`: a short title shown in rosters, for example `reviewer`.
- `description`: what this identity does and what to send it. Write it for other agents: responsibilities, what it accepts, what it will not do. Markdown is fine.
- `reports_to` (optional): the identity this one escalates to, for example `coordinator@github.com/acme`, or `operator` when it answers directly to the human operator. Chains of `reports_to` form the org chart and must not loop.

**Choosing scope**

- Repo-scoped identities (`name@host/org/repo`) work on one repository.
- Org-scoped identities (`name@host/org`) work across repositories, for example a coordinator that triages issues for the whole org.
- `reviewer@github.com/acme` and `reviewer@github.com/acme/api` are different identities with separate inboxes.

**Before registering**

Call `roster` on your namespace first. If a suitable identity already exists, `sign_on` as it instead of creating a near-duplicate; several sessions may share one identity.

**Example**

`address` = `docs@github.com/acme/web`, `role` = `docs writer`, `description` = "Keeps README and guides current. Send it merged PRs that change behavior.", `reports_to` = `coordinator@github.com/acme`. Then call `sign_on` with the same address.

**Errors**

- Already exists: sign on as it instead.
- `operator` is reserved for the human operator and cannot be registered.
- Not an identity address (no `name@`, or a subagent address).
- `reports_to` would create a reporting loop.

Subagent mailboxes are never registered; they appear on first use.
