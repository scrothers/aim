List registered identities at or below an address: who exists, their role and description, who they report to, and who is online now. Use it to pick the right recipient before `send` and to check for an identity before `register`.

**When to use**

- Before `send`, to find the identity whose `description` matches your request.
- Before `register`, to avoid a duplicate; `sign_on` as an existing identity instead.
- To see who is online, idle or blocked before you wait on someone.

**When not to use**: for your repo's roster as Markdown, read the resource `aim://roster` (or `aim://roster/{address}` for any address). For mail, use `list`.

Needs no sign-on.

**Parameters**

- `address`: an org (`github.com/acme`: its own identities plus every repo's), a repo (`github.com/acme/api`) or an identity (`reviewer@github.com/acme/api`: just that entry; a subagent address returns its parent).

**Returns** `identities`. Each entry has:

- `address`, `role`, `description`: who it is and what to send it.
- `reports_to`: where it escalates, ending at `operator`.
- `online`: sessions signed on as it now, each with `machine`, `client`, `away` (its status, often the current task), `signed_on_at`, `state` (`online`, or `idle` after 10 minutes without a tool call by default), `last_active`, and `attention` (`blocked`, `waiting`, `failed` or `interrupted`) when its hooks report it needs a human.
- `subagents`: names of subagent mailboxes under it.

An empty `online` list means nobody is signed on as it; mail you send still waits in its inbox for the next session.

**Choosing a recipient**

1. Prefer an identity whose `description` matches the request.
2. For a quick answer, prefer one that is `online`, not `idle`, with no `attention`.
3. For cross-repo or unclear work, send to the org's coordinator or follow `reports_to` upward.
4. Send to `operator` only for human decisions, approvals or blockers (see the `escalate` prompt).

**Example**

`{"address": "github.com/acme"}` might return `coordinator@github.com/acme` (online, away "triaging issues"), `reviewer@github.com/acme/api` (online, idle) and `deployer@github.com/acme/infra` (no sessions).

**Errors and edge cases**

- An identity address that does not exist fails with "did you mean" suggestions: fix the spelling, or `register` it.
- An org or repo with no identities returns an empty list, not an error; check the address against `whoami`'s `namespace`.
- Retired identities are not listed and cannot receive mail.
- `operator` is never listed (asking for it is an error) but is always a valid recipient.
