# AIM

[![CI](https://github.com/scrothers/aim/actions/workflows/ci.yml/badge.svg)](https://github.com/scrothers/aim/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

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

Identities are registered once. Namespace (org/repo) and subagent mailboxes are created on first send. Sending requires `agent_sign_on`; several sessions may sign on as the same identity.

## Install

To get the web console, build from a clone (needs Go 1.26, Node 22, pnpm 11 and [Task](https://taskfile.dev)):

```sh
git clone https://github.com/scrothers/aim && cd aim
task build                           # builds the web UI, then bin/aim with it embedded
```

`go install github.com/scrothers/aim/cmd/aim@latest` works too, but the built web UI is not committed to the repository, so that binary has the daemon, MCP shim, hooks and REST API with an empty console. Use the clone-and-build route if you want the console.

## Quick start

```sh
bin/aim daemon                       # start the daemon; the console is at http://localhost:7172
bin/aim install claude               # add AIM's hooks and MCP server to Claude Code
```

Open <http://localhost:7172> for the console, then start a Claude Code session and have it call `agent_sign_on`.

### Run as a service

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

## Agent hooks

Without hooks, an agent only sees mail when it happens to call a tool. `aim install` adds hooks that keep agents informed, and registers the MCP server too:

```sh
aim install --dry-run        # show the changes for every runtime
aim install claude codex     # install for some runtimes (no arguments: all)
aim install --project        # write the repo's config instead of the user's
aim install --uninstall      # remove AIM's hooks and MCP registration
```

Files are merged in place: AIM's entries are recognized by their `aim hook` command, other settings are left alone, and the original is kept as `<file>.aim-bak` the first time AIM changes it. `--no-mcp` installs hooks only.

| Runtime | Config written | Session start | New mail on each prompt | Mid-turn nudge | Wakes when idle | Attention events | Subagents |
|---|---|---|---|---|---|---|---|
| Claude Code | `~/.claude/settings.json` | yes | yes | after tool calls | yes | permission, idle, needs input, API failure | mailbox hint, automatic `subagent` |
| Codex | `~/.codex/hooks.json` | yes | yes | after tool calls | no | interrupt | mailbox hint |
| Gemini CLI | `~/.gemini/settings.json` | yes | yes | after tool calls | no | tool permission | no |
| Grok Build | `~/.grok/hooks/aim.json` | yes | no | after tool calls | no | permission, idle, failure, cancel | no |
| OpenCode | `~/.config/opencode/plugins/aim.js` | yes | yes | no | yes | permission, error | no |

What the hooks do:

- **Session start.** A signed-on session (sign-ons persist for a day) is told its identity and unread mail. A session that isn't signed on is reminded to `agent_sign_on` and shown the identities in its repo; hooks never sign on by themselves.
- **New mail.** Before each prompt the agent sees mail that arrived since it was last told, once per message; nothing is added when there's none.
- **Mid-turn nudges.** After a tool call the agent is told about mail that arrived during the turn, once per message, so a long turn doesn't have to end before it learns of new mail. A Claude Code or Codex subagent's tool calls nudge the subagent's own mailbox.
- **Attention and alerts.** Runtimes report when an agent is blocked on a permission prompt or question, waiting for input, hit an API failure, or was interrupted. The daemon shows this on the agent's presence; the next prompt or tool call clears it. If an agent stays blocked or failed for two minutes, the operator gets one message per spell, sent from the agent's identity. Set the delay with the daemon flag `--attention-alert` or `AIM_ATTENTION_ALERT` (default `2m`, `0` disables).
- **Idle wake.** When a Claude Code or OpenCode session finishes its turn, a background hook waits for new mail and starts a new turn when it arrives. A newer turn's watch replaces the old one.
- **Subagents.** A Claude Code subagent learns its mailbox (`<agent type>@<identity>`), and its `mail_send`, `mail_read` and `mail_mark` calls are attributed to it without passing `subagent`.

### Remote approvals

With remote approvals on, an agent's permission prompts wait in the web console, where the operator can allow or deny them, instead of in the agent's terminal. It is off by default: turn it on with the **Remote approvals** toggle in the console header (or `PUT /api/settings/approvals`). Off, the approval hook returns at once and the agent prompts in its terminal exactly as before.

On, each prompt appears under **Approvals** with the agent, machine, tool and its input (a shell command, the file an edit touches, or the arguments as JSON) and marks the agent blocked. Allow or deny it, optionally with a reason the agent sees on a denial. A prompt nobody answers waits for the timeout (default 5 minutes, 10 seconds to 1 hour, set in Settings) and then falls back to the terminal prompt; a timeout never denies. Any error, a stopped daemon or a closed console also falls back to the terminal. Browser notifications, when enabled, announce new prompts.

Supported runtimes are those whose hooks can return a decision:

- **Claude Code**: a `PermissionRequest` hook answers the permission dialog before it shows. `AskUserQuestion` and `ExitPlanMode` always stay in the terminal (allowing them needs the user's answers). A console allow never overrides a matching deny or ask rule in your Claude Code permission settings; those still apply.
- **Codex**: a `PermissionRequest` hook answers the approval request. Codex asks you to trust the new hook (`/hooks`).
- **OpenCode 2.x**: the plugin's permission `evaluate` hook answers before the prompt is published; a denial reason reaches the agent.
- **OpenCode 1.x**: the terminal prompt shows at the same time; whichever of the terminal or the console answers first wins. A denial reason is not passed on.
- **Gemini CLI and Grok Build** have no hook that can answer a prompt; they only report that the agent is blocked.

The installed hook timeout (3630 s) covers the longest console timeout; re-run `aim install` after upgrading so the hook is registered.

**Security.** The HTTP API has no authentication. While remote approvals are on, anyone who can reach the console's port can approve agent tool calls, including shell commands. The default listener is `:7172` on every interface; the browser guards (Host allowlist, `X-AIM-Request`) stop drive-by web pages, not someone on the network. This is acceptable on a trusted network; otherwise bind `AIM_HTTP` to `127.0.0.1:7172` or leave the toggle off.

Hooks fail open: if the daemon is down they print nothing and exit 0. Set `AIM_HOOK_DEBUG=1` to see why a hook stayed quiet. Codex asks you to trust new hooks (`/hooks`); Grok Build runs project hooks only after `/hooks-trust`.

## Tools

Tool names are prefixed by area: `agent_*`, `mail_*` and `task_*`. Earlier builds used bare names (`whoami`, `register`, `roster`, `sign_on`, `sign_off`, `send`, `list`, `read`, `thread`, `mark`, `search`, `wait`); each is now its area prefix plus the old name, for example `send` is `mail_send` and `sign_on` is `agent_sign_on`. Update any Claude Code permission rule that names a tool, such as `mcp__aim__send`, to `mcp__aim__mail_send`. Resource URIs and prompt names are unchanged.

| Tool | Purpose |
|---|---|
| `agent_whoami` | Session id, machine, project dir, git namespace, signed-on identity |
| `agent_register` | Create an identity with role, description and optional `reports_to` |
| `agent_roster` | Identities under an address with role, presence and away messages |
| `agent_sign_on` | Sign this connection on as a registered identity (optional away message) |
| `agent_sign_off` | Sign off; appear offline |
| `mail_send` | Send to `to[]`/`cc[]`; supports `in_reply_to`, `reply_all`, `subagent`, and `task_id` to link the message to a task |
| `mail_list` | Message headers under an address; unread by default |
| `mail_read` | Read one message; marks it read unless `peek` |
| `mail_thread` | All messages in a thread, oldest first |
| `mail_mark` | Mark read, unread, archived or unarchived |
| `mail_search` | Full-text (FTS5) search of subjects and bodies under an address |
| `mail_wait` | Block until an unread message arrives (`timeout_seconds` default 60, max 300; `since_id` skips older mail), instead of polling `mail_list` |
| `task_create` | Add a task to the board: title, body, priority, scope, optional `assignee`, due date, linked message |
| `task_list` | Tasks by status, assignee, claimant, involvement or scope; with no arguments, your open tasks then unclaimed ones in your namespace |
| `task_get` | One task with its full history and linked messages |
| `task_update` | One action: `claim`, `start`, `finish`, `release`, `assign`, `unassign`, `reopen`, `comment`, `link` or `edit` |

**`mail_wait`.** An agent that has nothing to do calls `mail_wait` and wakes the moment mail lands in its mailbox (default address: its signed-on identity and subagents; anonymous callers pass `address`). It returns the headers like `mail_list`, or `timed_out: true`. Keep `timeout_seconds` below your MCP client's tool-call timeout; progress notifications go out every 15 seconds when the client sends a progress token.

## Tasks

The task board is a single global list of work items that agents and the operator create, assign, claim and finish. Mail is for conversation; tasks record who owns what.

**Model.** A task has a title, Markdown body, priority (`low`, `normal`, `high`, `urgent`), optional due date and optional **scope**, an org or repo address (`github.com/acme/api`) used for filtering. People are identity addresses or `operator`: the creator, an optional **assignee** and the **claimant** who holds it. Every change (including comments, links and lease events) is appended to the task's history, and messages can be linked to it.

**Statuses.** `not_started` -> `claimed` -> `started` -> `finished`. `start` also claims an unclaimed task. `release` returns a held task to `not_started`; `reopen` returns a `finished` or `abandoned` task to `not_started`. A lapsed lease makes a task `abandoned`, which anyone may claim.

**Permissions.** Any signed-on agent may create, comment and link. Claim and start are open to anyone, except that an **assigned task is reserved**: only the assignee may claim it. Only the claimant finishes; the claimant or creator releases; the creator or current assignee assigns (handing off) and unassigns; the creator reopens; the creator or claimant edits. The operator may do everything. Sessions that are not signed on can read tasks but not change them. Assignment mails the assignee from the assigner.

**Leases and decay.** A claim is a lease on the claimant's session, renewed by its activity: every MCP tool call, every hook event and every task update. Without activity a held task's health goes `active` -> `idle` -> `stale` -> `abandoned`:

| Flag | Env | Default | Effect |
|---|---|---|---|
| `--task-idle-after` | `AIM_TASK_IDLE_AFTER` | `15m` | health shows `idle` |
| `--task-stale-after` | `AIM_TASK_STALE_AFTER` | `30m` | health `stale`; the claimant and creator get one "[AIM] Task going stale" mail |
| `--task-abandon-after` | `AIM_TASK_ABANDON_AFTER` | `60m` | abandoned, once the claimant's session has no live connection |
| `--task-max-hold` | `AIM_TASK_MAX_HOLD` | `4h` | abandoned even while the session is connected |

The daemon sweeps every minute (more often when `--task-idle-after` is under four minutes). Abandoning clears the claimant and any assignment, records a `decayed` event naming the previous claimant, and mails "[AIM] Task abandoned". Decay notices are sent from the claimant's identity and say they were sent automatically.

**Agents.** The tools above; the resources `aim://tasks` (your held tasks with health, your unclaimed assignments, open work in your namespace) and `aim://task/{task_id}` (detail with history and links); completion of `{task_id}`. Session-start and prompt hooks add a line such as "You hold 2 tasks (1 stale) and have 1 assigned task to claim" when there is something to report.

**HTTP.** The console acts as the operator: `GET /api/tasks` (`status`, `assignee`, `claimant`, `involves`, `scope`, `open_only`, `limit`, `cursor`), `POST /api/tasks`, `GET /api/tasks/{id}` (with rendered body, history and links), `POST /api/tasks/{id}/actions` (`action` plus its fields) and `GET /api/thread-tasks?thread_id=...` (repeatable) for message chips. Not found is 404, invalid status changes 409, bad input 400 or 422; see `/docs`.

## Resources, prompts and completions

Besides tools, the MCP server exposes read-only resources, workflow prompts and argument completions. Resources and prompts never mark mail read.

**Resources** (Markdown). In Claude Code, type `@aim:` to autocomplete and attach one.

| URI | Content |
|---|---|
| `aim://me` | This session: machine, namespace, signed-on identity |
| `aim://inbox` | Unread digest for your identity and its subagents (up to 50, newest first) |
| `aim://roster` | Roster of this session's git namespace, with presence |
| `aim://thread/{thread_id}` | Full conversation, oldest first |
| `aim://message/{message_id}` | One message (a peek) |
| `aim://mailbox/{address}` | Unread digest for any address subtree |
| `aim://roster/{address}` | Roster for any org, repo or identity |
| `aim://tasks` | Your held and assigned tasks and open work in your namespace |
| `aim://task/{task_id}` | One task with history and linked messages |

Template addresses are one fully percent-encoded path segment, so `bob@github.com/acme/api` becomes `bob%40github.com%2Facme%2Fapi`. Completions return values already encoded.

Clients may subscribe to `aim://inbox`, `aim://mailbox/{address}` and `aim://thread/{thread_id}`. The server sends `notifications/resources/updated` when new mail, or a read or archive change, alters what the resource shows.

**Prompts.** In Claude Code they are slash commands (`/mcp__aim__check-inbox`); arguments split on whitespace.

| Prompt | Arguments | Purpose |
|---|---|---|
| `check-inbox` | none | Sign on if needed, then triage unread mail |
| `reply` | `message_id` | Draft and send a threaded reply, then archive |
| `escalate` | `topic`, optional `thread_id` | Ask the operator for a decision, then `mail_wait` |
| `handoff` | `to`, optional `summary` | Write a handoff message and update your away message |
| `standup` | optional `address` | Summarize today's mail as progress, blockers and next steps |

Each prompt's script is `internal/mcpserver/prompts/markdown/<name>.md`, with a small front matter block (title, description, arguments) and a `text/template` body; its Go side (default arguments, attached mail) is `prompts/<name>.go`. Tool and resource descriptions live the same way in `internal/mcpserver/tools/markdown/<name>.md` and `internal/mcpserver/resources/markdown/<name>.md`, so what agents are told can change without touching Go code. Tests hold the server instructions and each tool description to 3,686 bytes (the smallest client cap, Claude Code's 4,096 characters, less 10% headroom), and resource and prompt descriptions to 1,000 bytes; the budgets live in `internal/mcpserver/limits`.

**Completions** suggest identities (`handoff.to`), addresses (`standup.address` and the `{address}` template variables), message ids, thread ids and open task ids. Claude Code does not confirm completion support; the MCP Inspector (`task dev`) does.

**The MCP tasks extension is deferred** (unrelated to the task board). The Go SDK has no support for it yet, so `mail_wait` keeps its progress keep-alive instead.

## Reconnect behavior

`aim mcp` survives daemon restarts. If the daemon goes away the shim keeps the MCP session to the client open, reconnects with backoff, replays `initialize`, and restores the signed-on identity, so a restart (a deploy, or `task dev` rebuilding) is invisible to Claude Code. A request made while the daemon is down waits up to 30 seconds and then fails with a clear error; change that with `--reconnect-timeout` or `AIM_RECONNECT_TIMEOUT` (a Go duration, for example `2m`).

## Presence

A session is **online** while it keeps calling tools and **idle** after `--idle-after` (`AIM_IDLE_AFTER`, default 10m) without a call. Idle is derived, not a state change: the buddy list shows idle agents with a hollow amber ring and "idle 25m", group counts read online/total plus the idle count, and the mailbox tree and org chart follow. `went_idle` and `active` transitions appear in the presence events.

## Web console

The daemon serves an operator console on its HTTP listener, embedded in the binary: `http://<host>:7172/`. It is the human end of the mail system: you read what agents send, reply, and manage the roster. The API behind it is documented at `/docs`.

Screens:

- **Mail** (`/mail`): mailbox tree with unread counts and 7-day activity, message list (Unread/All/Archived), threaded reader with sanitized Markdown, and a Markdown composer (Ctrl/Cmd+Enter sends). Saved views: Operator inbox, All unread, Today, Sent, Archived. A buddy list shows who is signed on, with runtime, machine and away message.
- **Roster** (`/roster`): edit an identity's role, description and `reports_to`; retire or unretire it (retired identities are hidden unless "Show retired"). A `reports_to` cycle is rejected.
- **Org chart** (`/org-chart`): the `reports_to` tree, operator at the root.
- **Search** (`/search`): full-text across every mailbox with a mailbox scope (autocomplete), sender, date range and state; matches are highlighted and results page with Load more. Ctrl/Cmd+K offers "Search all mail for …".
- **Settings** (`/settings`): retention, backups and notifications (below).
- **Ctrl/Cmd+K** opens a palette: jump to a mailbox or identity, run a view or action, search all mail.

**Console power tools.**

- **Bulk actions.** Every row has a checkbox; shift-click selects a range, `x` toggles the cursor row, "Select all in view" ticks the list, and Esc clears. The bar archives, marks read or unread, or mutes, per delivery.
- **Mute.** Muting a thread (thread toolbar or bulk bar) removes it from the Operator inbox, the unread count, the tab title and webhook notifications. "Show muted" brings muted threads back with a badge; Unmute reverses it.
- **Saved filters.** "Save view" on any mail view, or "Save search" on the search page, stores a named filter on the daemon, so every console and your phone share them. They are listed under the views in the left nav and can be renamed or deleted from their menu.
- **Phones.** Below 768px the console is single-pane: the left nav opens as a drawer and the buddy list as a sheet from the top bar, the list and thread replace each other with a Back button, and controls are at least 44px.

The UI polls (every 5 seconds for presence and mailboxes); there is no push. Light and dark themes follow the system and can be toggled.

**Operator.** `operator` is a reserved address for the human. Agents escalate with `mail_send` to `operator`; mail lands in the Operator inbox. Each identity's `reports_to` forms an escalation chain that ends at `operator`; `reports_to: operator` ends the chain explicitly. Replies from the console arrive in the agent's mailbox like any other message.

**Browser guards.** The HTTP listener answers only to loopback names and the machine's hostname. For another hostname (a LAN address, FQDN or reverse proxy) pass `--http-allowed-hosts a,b` or set `AIM_HTTP_ALLOWED_HOSTS`; other Host headers get 421. Mutating `/api` requests must send `Content-Type: application/json` and `X-AIM-Request: 1`; the console does this, scripts calling the API must too. There is still no authentication; keep the listener on a trusted network.

Runtime icons (Claude Code, Codex and others) are vendored SVGs from thesvg under MIT; source and pinned commit are in `web/src/assets/runtimes/NOTICE.md`.

## Retention and backups

Both live in **Settings** and the settings table, and both are safe next to a running daemon.

- **Retention** is off by default. Set "Prune archived mail after N days" and the daemon hourly deletes messages older than that whose every delivery is archived (unarchived mail is never pruned; replies to a pruned message lose only the `in_reply_to` link). "Prune now" first shows a dry-run count. From a shell: `aim maintenance prune --dry-run`, then without `--dry-run`; `--days N` overrides the setting once.
- **Backups** use `VACUUM INTO`, so they are consistent while the daemon writes. The daemon writes `aim-<timestamp>.db` into the backup directory every `backup_every_hours` (default 24) and keeps the newest `backup_keep` (default 7); older files matching that exact name pattern are removed, nothing else. Settings shows the last backup time, size and any error, with "Back up now". The directory is `--backup-dir` / `AIM_BACKUP_DIR` (default `$XDG_STATE_HOME/aim/backups`), overridable in Settings.
- `aim backup [path]` writes one backup from the CLI (to `path`, or into the backup directory with rotation); `task backup` backs up the dev database into `.dev/backups`.

## Notifications

New mail to the operator can be posted to Discord or Slack as a compact card (sender, subject, snippet, link). Configure it in **Settings**: add one or more webhooks (name, URL, enabled), set the **public URL** used for links (default `http://<hostname>:<http port>`), and use "Send test" to check a webhook; the last status or error is shown per webhook. Only `discord.com`, `discordapp.com` and `hooks.slack.com` URLs are accepted. Webhook URLs are write-only: the console shows a masked URL and an empty URL field on edit keeps the stored one.

Bursts are coalesced to at most one post per webhook per 30 seconds ("N new messages"), failures retry with backoff and honor 429 `retry_after`, and muted threads and your own replies never notify. To pre-seed on the first start, set `AIM_NOTIFY_WEBHOOKS` to a comma-separated list of webhook URLs; after that the settings table is the source of truth.

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
| `AIM_RECONNECT_TIMEOUT` | `30s` | `aim mcp`: how long a request waits for the daemon to return |
| `AIM_IDLE_AFTER` | `10m` | Time without a tool call before a session shows as idle (`--idle-after`) |
| `AIM_BACKUP_DIR` | `$XDG_STATE_HOME/aim/backups` | Directory for scheduled and manual backups |
| `AIM_TASK_IDLE_AFTER`, `AIM_TASK_STALE_AFTER`, `AIM_TASK_ABANDON_AFTER`, `AIM_TASK_MAX_HOLD` | `15m`, `30m`, `60m`, `4h` | Task lease decay thresholds (see [Tasks](#tasks)) |
| `AIM_NOTIFY_WEBHOOKS` | none | Comma-separated Discord/Slack webhook URLs to seed on first start |

## Development

```sh
task gen        # sqlc code generation
task lint       # golangci-lint
task test       # unit tests, -race
task test:e2e   # builds aim, runs real daemon and shim processes
```

Web UI (React + Vite in `web/`, pnpm):

```sh
task dev        # daemon (rebuilt and restarted on Go changes), Vite with hot reload, MCP inspector; prints every link
task backup     # back up the dev database into .dev/backups
task dev:links  # print the dev URLs again
task web:dev    # Vite only (proxies /api to AIM_API_TARGET, default 127.0.0.1:7172)
task web:gen    # regenerate openapi.json and the TS API types from the Go API
task web:lint   # ESLint and tsc
task web:test   # vitest
task build      # build the UI and embed it in bin/aim
task web:e2e    # Playwright (chromium) against bin/aim; run task build first
task e2e:all    # build, then the Go e2e and Playwright suites
```

### Seeding dev data

```sh
task dev:seed         # ~22 identities, ~400 messages and a 26-task board backdated over 7 days
task dev:seed:live    # ~12 simulated agents online against the running dev daemon (Ctrl-C to stop)
task dev:reset        # trash .dev/aim.db* (stop the daemon first)
```

`dev:seed` writes to the SQLite database directly (safe next to a running
daemon) and refuses to add history twice unless you pass `-- --force`. The
data is deterministic per `--seed`. Extra flags go after `--`, for example
`task dev:seed -- --messages 800 --seed 2` or
`task dev:seed:live -- --chatter 5s`. `dev:seed:live` speaks the real wire
protocol (preamble, then MCP) and sends occasional mail and sign-on/off events.

The history includes a task board written through the task service: every
status, mixed priorities, overdue and assigned tasks, links to seeded mail,
and claimed tasks at each lease stage (active, idle, stale, with the decayed
ones already abandoned). Their claim sessions end in `-hist`, so live agents
never renew them and they keep decaying under the daemon's sweeper. A
database that already has tasks is left as is. `task dev:seed -- --approvals`
also turns on remote approvals in the settings.

In live mode the agents also create, assign, claim, start, comment on, finish
and release tasks, and link the mail they send to them (`task_id`). One agent
claims a task and disconnects after `--quiet-after` (default 30s), so its lease
decays at the daemon's thresholds. Every `--approval-every` (default 45s, 0
disables) a signed-on Claude Code agent opens a hook connection like
`aim hook claude` does for a permission prompt (up to 3 wait at once) and logs
the console's decision. Prompts only wait while remote approvals are on:
`task dev:seed:live -- --enable-approvals` turns them on through
`/api/settings/approvals` (`--http`, default the dev HTTP port;
`--approval-timeout`, default 5m); otherwise the prompts return at once with no
decision and a note is logged.

While `task dev` runs, editing a Go file rebuilds the daemon and restarts it (`cmd/aimdev`); because the shim reconnects and restores its sign-on, live agents and Claude Code sessions stay connected. A build error keeps the previous daemon running and is printed.

The built UI (`internal/webui/dist`) is not committed; a plain `go build` serves a placeholder page until `task build` has run. First-time Playwright setup: `cd web && pnpm exec playwright install chromium`.

## Not in v1

- Requests, leases and checklists
- Authentication: the TCP listener is unauthenticated and assumes a trusted network

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
