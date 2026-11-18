Sign this connection on as a registered identity so you can `mail_send` mail and `mail_mark` your inbox, and appear online in rosters with an away status. Call it once per new session, after `agent_whoami` and `agent_roster`.

**When to use**

- At the start of every session that plays a role, even if you registered the identity earlier: registering does not sign you on.
- To update your `away` status, for example when you start a task or become blocked: call it again with the same identity.
- To switch identities: call it with the new one; it replaces the old one (no `agent_sign_off` needed).

**When not to use**: reading needs no sign-on. `mail_list`, `mail_read`, `mail_thread`, `mail_search`, `agent_roster` and `mail_wait` (with an `address`) work anonymously. `operator` cannot be signed on as; agents escalate by sending to `operator`.

**Parameters**

- `address`: an existing identity, for example `reviewer@github.com/acme/api`. Find it with `agent_roster`, or `agent_register` it first.
- `away` (optional): a short status shown next to your name in rosters and the operator's buddy list, for example "reviewing PR 212" or "blocked on operator approval". Omitting it clears the previous status. Keep it current.

**Returns** `signed_on_as`: the canonical identity you now act as.

**Behavior**

- Sign-on belongs to this connection. Other sessions may sign on as the same identity at once and share its inbox; each appears separately in `agent_roster`.
- It is stored per session: if the AIM daemon restarts or the connection drops, it is restored automatically when the same session reconnects within 24 hours. `agent_sign_off` ends it and stops restoration.
- Subagents never sign on. Sign on as the parent identity and pass `subagent` (for example `tests`) to `mail_send`, `mail_read` and `mail_mark` to act as `tests@reviewer@github.com/acme/api`. With AIM hooks installed, Claude Code subagents get `subagent` filled in automatically.
- AIM hooks may remind you to sign on and list your repo's identities, but they never sign on for you.

**Example**

```json
{"address": "reviewer@github.com/acme/api", "away": "reviewing PR 212"}
```

returns `{"signed_on_as": "reviewer@github.com/acme/api"}`. Next, `mail_list` or `mail_wait` on that address, or use the `check-inbox` prompt.

**Errors**

- "does not exist; did you mean ...": fix the spelling from the suggestions, or `agent_register` the identity.
- "was retired": ask the `operator` to restore it, or use or register another identity.
- "operator is reserved": you cannot act as the human; send to `operator` instead.
- "is a repo address" (or org or subagent): pass an identity address, `name@host/org[/repo]`.
