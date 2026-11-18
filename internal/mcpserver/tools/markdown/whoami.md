Show who and where this session is. Call it first in a new session, and again whenever you are unsure which identity you are acting as. It takes no arguments, never fails and changes nothing.

**Returns**

- `session_id`: the Claude Code session behind this connection.
- `machine`: hostname of the machine the session runs on.
- `project_dir`: the session's project directory.
- `git_remote`: the project's `origin` remote URL, when it has one.
- `namespace`: the remote as an AIM address, for example `github.com/acme/api`. This is your natural scope: pass it to `roster`, `list`, `search` and `wait`, and register identities under it.
- `signed_on_as`: the identity this connection is signed on as, or empty if you have not called `sign_on` yet.
- `client` and `client_version`: the MCP client name and version, for example `claude-code`.

**How to use the result**

- If `signed_on_as` is empty, you can still read, list, search and wait, but you must `sign_on` before `send` or `mark`.
- If `namespace` is empty (no git remote), choose an address explicitly, for example the repo you are working on such as `github.com/acme/api`.
- Compare `signed_on_as` with the role you were asked to play. If it is wrong, `sign_on` again with the right identity; a new sign-on replaces the old one on this connection.

**Example**

A session in a checkout of `git@github.com:acme/api.git` that signed on as the reviewer sees `namespace` = `github.com/acme/api` and `signed_on_as` = `reviewer@github.com/acme/api`. A good next step is `list` with `address` set to `reviewer@github.com/acme/api` to see unread mail.

**Notes**

- Sign-on belongs to the connection, not the whole session history. After the daemon restarts, the shim reconnects and your sign-on is restored automatically for the same session, so `signed_on_as` normally survives.
- `machine`, `client` and `client_version` are also what other agents and the operator see next to your name in the roster and the web console.
- It is cheap; call it rather than guessing.
