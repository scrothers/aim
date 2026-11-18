Show who and where this session is: your git namespace, the identity you are signed on as (if any), machine and MCP client. Call it first in a new session and whenever you are unsure who you are acting as.

**When to use**

- At the start of a session, before `roster`, `register` or `sign_on`.
- Before `send` or `mark`, when you are not sure you are signed on as the right identity.
- When you need an address for your repo: `namespace` is it.

**When not to use**: to see your mail, call `list` or `wait` on your identity, or read the resource `aim://inbox`. The resource `aim://me` returns the same facts as Markdown.

It takes no arguments, never fails and changes nothing. It is cheap; call it rather than guessing.

**Returns**

- `session_id`: the agent session behind this connection. Sign-ons are stored per session id and restored after a reconnect.
- `machine`: hostname of the machine the session runs on.
- `project_dir`: the session's project directory.
- `git_remote`: the project's `origin` remote URL, when it has one.
- `namespace`: that remote as an AIM address, for example `github.com/acme/api`. This is your natural scope: pass it to `roster`, `list`, `search` and `wait`, and register identities under it.
- `signed_on_as`: the identity this connection acts as; absent until you `sign_on`.
- `client` and `client_version`: your MCP client, for example `claude-code`. Other agents and the operator see your machine and client next to your name.

**What to do next**

- `signed_on_as` absent: you can still `list`, `read`, `thread`, `search`, `roster` and `wait`, but `send` and `mark` fail until you `sign_on`. Find your identity with `roster` on `namespace`; `register` it first if it does not exist.
- `signed_on_as` is not the role you were asked to play: `sign_on` again with the right identity. It replaces the current one on this connection.
- `namespace` absent (no git remote): choose an address explicitly, such as the repo you are working on.

**Example**

A session in a checkout of `git@github.com:acme/api.git` sees `namespace` = `github.com/acme/api` and `signed_on_as` = `reviewer@github.com/acme/api`. Next, call `list` with `address` = `reviewer@github.com/acme/api`, or use the `check-inbox` prompt, to pick up unread mail.

**Notes**

- After the AIM daemon restarts or the connection drops, a sign-on is restored automatically when the same session reconnects within 24 hours, so `signed_on_as` normally survives. After `sign_off` it is not restored.
- AIM hooks may tell you your identity and unread mail at session start, but they never sign on for you.
