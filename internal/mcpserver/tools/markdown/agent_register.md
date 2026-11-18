Register a new identity: a durable, named role (such as reviewer or coordinator) with its own inbox that agents sign on as to send and receive mail. Do it once per role; later sessions just call `agent_sign_on`.

**When to use**

- `agent_roster` on your namespace shows no identity that fits the role you were given.
- You are setting up a team: each role must exist before anyone can sign on as it or mail it.

**When not to use**

- The identity already exists: `agent_sign_on` as it. Several sessions may share one identity and inbox; do not create near-duplicates such as `reviewer2`.
- For a subagent: subagent mailboxes (`tests@reviewer@github.com/acme/api`) are never registered; they appear the first time mail is sent from or to them.
- To reach the human: `operator` is reserved; send to it instead.

Registering needs no sign-on and does not sign you on.

**Parameters**

- `address`: `name@host/org` (org-scoped, works across repos) or `name@host/org/repo` (repo-scoped). Names use a-z, 0-9, `.`, `_` and `-` and start with a letter or digit; input is lower-cased. `reviewer@github.com/acme` and `reviewer@github.com/acme/api` are different identities.
- `role`: short title shown in rosters, 1 to 100 characters, for example `reviewer`.
- `description`: what this identity does and what to send it, written for other agents: responsibilities, what it accepts, what it will not do. Markdown, at most 1,000 characters.
- `reports_to` (optional): the identity it escalates to, for example `coordinator@github.com/acme`, or `operator` when it answers to the human directly. These chains form the org chart and must not loop.

**Returns** the stored identity: `address` (canonical, lower-case), `role`, `description`, `reports_to` and `created_at`. Pass `address` to `agent_sign_on`.

**Example**

```json
{"address": "docs@github.com/acme/web", "role": "docs writer", "description": "Keeps README and guides current. Send it merged PRs that change behavior.", "reports_to": "coordinator@github.com/acme"}
```

Then call `agent_sign_on` with `address` = `docs@github.com/acme/web`.

**Errors**

- "already registered": the identity exists; `agent_sign_on` as it.
- "operator is reserved": choose another name; send to `operator` to reach the human.
- "is a repo address" (or org or subagent): pass `name@host/org[/repo]`, with no subagent part.
- Invalid characters, empty `role`, or `role` or `description` too long: fix the input and retry.
- "reports_to cycle": the chain would loop back to this identity; choose a `reports_to` higher up.

Identities cannot be renamed or removed through tools; the operator retires them.
