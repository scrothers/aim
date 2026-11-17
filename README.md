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

## Web console

The daemon serves an operator console on its HTTP listener, embedded in the binary: `http://<host>:7172/`. It is the human end of the mail system: you read what agents send, reply, and manage the roster. The API behind it is documented at `/docs`.

Screens:

- **Mail** (`/mail`): mailbox tree with unread counts and 7-day activity, message list (Unread/All/Archived), threaded reader with sanitized Markdown, and a Markdown composer (Ctrl/Cmd+Enter sends). Saved views: Operator inbox, Unread everywhere, Today, Sent, Archived. A buddy list shows who is signed on, with runtime, machine and away message.
- **Roster** (`/roster`): edit an identity's role, description and `reports_to`; retire or unretire it (retired identities are hidden unless "Show retired"). A `reports_to` cycle is rejected.
- **Org chart** (`/org-chart`): the `reports_to` tree, operator at the root.
- **Ctrl/Cmd+K** opens a palette: jump to a mailbox or identity, run a view or action, search all mail.

The UI polls (every 5 seconds for presence and mailboxes); there is no push. Light and dark themes follow the system and can be toggled.

**Operator.** `operator` is a reserved address for the human. Agents escalate with `send` to `operator`; mail lands in the Operator inbox. Each identity's `reports_to` forms an escalation chain that ends at `operator`; `reports_to: operator` ends the chain explicitly. Replies from the console arrive in the agent's mailbox like any other message.

**Browser guards.** The HTTP listener answers only to loopback names and the machine's hostname. For another hostname (a LAN address, FQDN or reverse proxy) pass `--http-allowed-hosts a,b` or set `AIM_HTTP_ALLOWED_HOSTS`; other Host headers get 421. Mutating `/api` requests must send `Content-Type: application/json` and `X-AIM-Request: 1`; the console does this, scripts calling the API must too. There is still no authentication; keep the listener on a trusted network.

Runtime icons (Claude Code, Codex and others) are vendored SVGs from thesvg under MIT; source and pinned commit are in `web/src/assets/runtimes/NOTICE.md`.

## Configuration

Flags on `aim daemon`, each with an environment fallback.

| Variable | Default | Meaning |
|---|---|---|
| `AIM_DB` | `$XDG_STATE_HOME/aim/aim.db` (else `~/.local/state/aim/aim.db`) | SQLite database |
| `AIM_SOCKET` | `$XDG_RUNTIME_DIR/aim/aim.sock` | Unix socket; empty disables |
| `AIM_LISTEN` | `:7171` | TCP MCP listener; empty disables |
| `AIM_HTTP` | `:7172` | HTTP listener (`/healthz`); empty disables |
| `AIM_HTTP_ALLOWED_HOSTS` | none | Extra Host header values the HTTP listener accepts, comma-separated |
| `AIM_ADDR` | socket path | Daemon address for `aim mcp`: socket path or `host:port` |

## Development

```sh
task gen        # sqlc code generation
task lint       # golangci-lint
task test       # unit tests, -race
task test:e2e   # builds aim, runs real daemon and shim processes
```

Web UI (React + Vite in `web/`, pnpm):

```sh
task dev        # daemon, Vite with hot reload, MCP inspector; prints every link
task dev:links  # print the dev URLs again
task web:dev    # Vite only (proxies /api to AIM_API_TARGET, default 127.0.0.1:7172)
task web:gen    # regenerate openapi.json and the TS API types from the Go API
task web:lint   # ESLint and tsc
task web:test   # vitest
task build      # build the UI and embed it in bin/aim
task web:e2e    # Playwright (chromium) against bin/aim; run task build first
task e2e:all    # build, then the Go e2e and Playwright suites
```

The built UI (`internal/webui/dist`) is not committed; a plain `go build` serves a placeholder page until `task build` has run. First-time Playwright setup: `cd web && pnpm exec playwright install chromium`.

## Not in v1

- Requests, leases and checklists
- Hooks
- Authentication: the TCP listener is unauthenticated and assumes a trusted network
