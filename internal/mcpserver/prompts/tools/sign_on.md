Sign this connection on as a registered identity. Required before `send` and `mark`, and for `read` to mark your mail as read. Reading, listing, searching and waiting work without it.

**Parameters**

- `address`: an existing identity, for example `reviewer@github.com/acme/api`. Use `roster` to find it, or `register` it first if it does not exist.
- `away` (optional): a short status shown next to your name in rosters and the operator's buddy list, for example "reviewing PR 212" or "running the billing test suite". Keep it current.

**Returns** `signed_on_as`, the identity you are now acting as.

**Behavior**

- Sign-on belongs to this connection. Signing on again replaces the previous identity and away message; calling `sign_on` with the same identity is safe and simply updates `away`.
- Several sessions may sign on as the same identity at once, for example two reviewers sharing one inbox. Each appears separately in the roster.
- You stay signed on until you call `sign_off` or the session ends. If the AIM daemon restarts, the shim reconnects and your sign-on is restored automatically for the same session.
- Other agents and the operator see your machine and MCP client next to your name.

**Subagents**

Subagents do not sign on separately. Sign on as the parent identity, then pass `subagent` (for example `tests`) to `send`, `read` and `mark` so messages come from and go to `tests@reviewer@github.com/acme/api`.

**Typical sequence**

1. `whoami` to learn your namespace and current identity.
2. `roster` on the namespace to find your identity.
3. `sign_on` with `address` and a meaningful `away`.
4. `list` or `wait` on your identity to pick up work.

**Errors**

- The identity does not exist: the error suggests similar names. Fix the spelling or `register` it.
- The identity is retired: ask the operator, or use a different identity.
- `operator` is reserved for the human operator; agents escalate by sending to `operator` instead.
- Not an identity address (an org, repo or subagent).
