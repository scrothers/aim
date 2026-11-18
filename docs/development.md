# Development

Working on AIM itself: Taskfile targets, the dev loop, tests and seeding dev data. Contribution rules (prerequisites, commit style, licensing) are in [CONTRIBUTING.md](../CONTRIBUTING.md).

## On this page

- [Go and test tasks](#go-and-test-tasks)
- [Web UI](#web-ui)
- [Seeding dev data](#seeding-dev-data)
- [Dev loop notes](#dev-loop-notes)

## Go and test tasks

```sh
task gen        # sqlc code generation
task lint       # golangci-lint
task test       # unit tests, -race
task test:e2e   # builds aim, runs real daemon and shim processes
```

## Web UI

React + Vite in `web/`, pnpm:

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

## Seeding dev data

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

The history includes a task board ([Task board](tasks.md)) written through the task service: every
status, mixed priorities, overdue and assigned tasks, links to seeded mail,
and claimed tasks at each lease stage (active, idle, stale, with the decayed
ones already abandoned). Their claim sessions end in `-hist`, so live agents
never renew them and they keep decaying under the daemon's sweeper. A
database that already has tasks is left as is. `task dev:seed -- --approvals`
also turns on remote approvals in the settings ([Hooks and approvals](hooks.md#remote-approvals)).

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

## Dev loop notes

While `task dev` runs, editing a Go file rebuilds the daemon and restarts it (`cmd/aimdev`); because the shim reconnects and restores its sign-on ([Concepts](concepts.md#reconnect-behavior)), live agents and Claude Code sessions stay connected. A build error keeps the previous daemon running and is printed.

The built UI (`internal/webui/dist`) is not committed; a plain `go build` serves a placeholder page until `task build` has run. First-time Playwright setup: `cd web && pnpm exec playwright install chromium`.
