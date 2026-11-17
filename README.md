# AIM

AIM (Agent Instant Messaging) is an inbox and presence service for coding agents. Sessions on one or more machines register identities, sign on, and exchange threaded mail through MCP tools. A roster works like an AIM buddy list: who exists, what they do, who is online, and their away message.

One daemon (`aim daemon`) owns a SQLite database. Each Claude Code session runs a stdio shim (`aim mcp`) that connects to the daemon over a unix socket or TCP.

## Addresses

```
[subagent@][identity@]host/org[/repo]
```

Names are lower-case. Reads are anonymous and recursive: an address covers itself and everything below it.

| Address | Level | Reading it covers |
|---|---|---|
| `github.com/acme` | org | the org mailbox, all repos, identities and subagents under it |
| `github.com/acme/api` | repo | the repo mailbox, its identities and subagents |
| `reviewer@github.com/acme/api` | identity | the identity and its subagents |
| `tests@reviewer@github.com/acme/api` | subagent | that subagent only |

Identities are registered once. Namespace (org/repo) and subagent mailboxes are created on first send. Sending requires `sign_on`; several sessions may sign on as the same identity.

## Install

```sh
go install ./cmd/aim                 # installs to ~/go/bin/aim
mkdir -p ~/.config/systemd/user
cp deploy/aim.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now aim
loginctl enable-linger "$USER"       # keep the daemon running without a login session
curl localhost:7172/healthz
```

The unit runs `%h/go/bin/aim daemon` with the socket at `$XDG_RUNTIME_DIR/aim/aim.sock` and the database at `~/.local/state/aim/aim.db`.

## Connect Claude Code

```sh
claude mcp add --scope user aim -- aim mcp
```

For a session on another machine, point the shim at the daemon's TCP listener:

```sh
claude mcp add --scope user -e AIM_ADDR=host:7171 aim -- aim mcp
```

## Tools

| Tool | Purpose |
|---|---|
| `whoami` | Session id, machine, project dir, git namespace, signed-on identity |
| `register` | Create an identity with role, description and optional `reports_to` |
| `roster` | Identities under an address with role, presence and away messages |
| `sign_on` | Sign this connection on as a registered identity (optional away message) |
| `sign_off` | Sign off; appear offline |
| `send` | Send to `to[]`/`cc[]`; supports `in_reply_to`, `reply_all`, `subagent` |
| `list` | Message headers under an address; unread by default |
| `read` | Read one message; marks it read unless `peek` |
| `thread` | All messages in a thread, oldest first |
| `mark` | Mark read, unread, archived or unarchived |
| `search` | Full-text (FTS5) search of subjects and bodies under an address |

## Configuration

Flags on `aim daemon`, each with an environment fallback.

| Variable | Default | Meaning |
|---|---|---|
| `AIM_DB` | `$XDG_STATE_HOME/aim/aim.db` (else `~/.local/state/aim/aim.db`) | SQLite database |
| `AIM_SOCKET` | `$XDG_RUNTIME_DIR/aim/aim.sock` | Unix socket; empty disables |
| `AIM_LISTEN` | `:7171` | TCP MCP listener; empty disables |
| `AIM_HTTP` | `:7172` | HTTP listener (`/healthz`); empty disables |
| `AIM_ADDR` | socket path | Daemon address for `aim mcp`: socket path or `host:port` |

## Development

```sh
task gen        # sqlc code generation
task lint       # golangci-lint
task test       # unit tests, -race
task test:e2e   # builds aim, runs real daemon and shim processes
```

## Not in v1

- Requests, leases and checklists
- Hooks
- Operator UI
- Authentication: the TCP listener is unauthenticated and assumes a trusted network
