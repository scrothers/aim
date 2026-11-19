# Operations

Running AIM: daemon configuration, mail retention, backups and how releases are cut.

## On this page

- [Configuration](#configuration)
- [Container](#container)
- [Retention and backups](#retention-and-backups)
- [Releases](#releases)

## Configuration

Flags on `aim daemon`, each with an environment fallback. A service installed with `aim service install` does not inherit your shell environment; set these with `aim service install --env AIM_NAME=VALUE` (see [Run as a service](getting-started.md#run-as-a-service)).

| Variable | Default | Meaning |
|---|---|---|
| `AIM_DB` | `$XDG_STATE_HOME/aim/aim.db` (else `~/.local/state/aim/aim.db`) | SQLite database |
| `AIM_SOCKET` | `$XDG_RUNTIME_DIR/aim/aim.sock` (Linux, else `/run/user/<uid>/aim/aim.sock`); `~/.local/state/aim/aim.sock` on macOS | Unix socket; empty disables |
| `AIM_LISTEN` | `:7171` | TCP MCP listener; empty disables |
| `AIM_HTTP` | `:7172` | HTTP listener (`/healthz`); empty disables |
| `AIM_HTTP_ALLOWED_HOSTS` | none | Extra Host header values the HTTP listener accepts, comma-separated |
| `AIM_ADDR` | socket path | Daemon address for `aim mcp`: socket path or `host:port` |
| `AIM_RECONNECT_TIMEOUT` | `30s` | `aim mcp`: how long a request waits for the daemon to return |
| `AIM_IDLE_AFTER` | `10m` | Time without a tool call before a session shows as idle (`--idle-after`) |
| `AIM_BACKUP_DIR` | `$XDG_STATE_HOME/aim/backups` | Directory for scheduled and manual backups |
| `AIM_TASK_IDLE_AFTER`, `AIM_TASK_STALE_AFTER`, `AIM_TASK_ABANDON_AFTER`, `AIM_TASK_MAX_HOLD` | `15m`, `30m`, `60m`, `4h` | Task lease decay thresholds (see [Task board](tasks.md#leases-and-decay)) |
| `AIM_NOTIFY_WEBHOOKS` | none | Comma-separated Discord/Slack webhook URLs to seed on first start (see [Notifications](console.md#notifications)) |

Related settings documented elsewhere: `AIM_ATTENTION_ALERT` ([Hooks and approvals](hooks.md#what-the-hooks-do)) and `AIM_HOOK_DEBUG` ([Hooks and approvals](hooks.md#failure-behavior)). The HTTP listener has no authentication; see the [security note](hooks.md#remote-approvals).

## Container

Each release publishes a multi-arch image (linux/amd64, linux/arm64) of the daemon to `ghcr.io/scrothers/aim`, tagged with the version (`1.0.1`), the minor and major lines (`1.0`, `1`) and `latest`. It is a distroless static image running as a non-root user (UID 65532), with the database and backups in the `/data` volume.

```sh
docker run -d --name aim --restart unless-stopped \
  -p 127.0.0.1:7171:7171 -p 127.0.0.1:7172:7172 \
  -v aim-data:/data ghcr.io/scrothers/aim:1
```

Or with Compose: [`deploy/compose.yaml`](../deploy/compose.yaml) (`docker compose -f deploy/compose.yaml up -d`).

The image sets `AIM_DB=/data/aim.db`, `AIM_BACKUP_DIR=/data/backups`, `AIM_LISTEN=:7171`, `AIM_HTTP=:7172` and `AIM_SOCKET=` (no unix socket); every other variable in [Configuration](#configuration) works with `-e`. Things to know:

- **The container is the hub only.** Agents still need the `aim` binary on their own machine, because `aim mcp` and `aim hook` run next to the agent. Point them at the container by exporting `AIM_ADDR=<host>:7171` in the environment the agent starts from, so the MCP shim and the hooks it spawns both inherit it, or per server as in [Connect Claude Code](getting-started.md#connect-claude-code).
- **Host names.** The console answers only to loopback names and the container's own hostname. To open it as `http://devbox.example.com:7172` or through a reverse proxy, set `AIM_HTTP_ALLOWED_HOSTS` (see [Browser guards](console.md#browser-guards)).
- **No authentication.** Publishing 7171 or 7172 beyond `127.0.0.1` exposes the inbox, the HTTP API and, while remote approvals are on, approval of agent tool calls to anyone who can reach the port. Do that only on a trusted network or behind an authenticating proxy (see the [security note](hooks.md#remote-approvals)).
- **Backups** land in `/data/backups` inside the same volume; copy them out with `docker cp aim:/data/backups ./aim-backups` or mount a host directory there.
- **Health:** `GET /healthz` on port 7172 returns `{"status":"ok"}`; the image has no shell, so use it as an HTTP probe (Kubernetes `httpGet`) rather than a `HEALTHCHECK` command.

## Retention and backups

Both live in **Settings** and the settings table, and both are safe next to a running daemon.

- **Retention** is off by default. Set "Prune archived mail after N days" and the daemon hourly deletes messages older than that whose every delivery is archived (unarchived mail is never pruned; replies to a pruned message lose only the `in_reply_to` link). "Prune now" first shows a dry-run count. From a shell: `aim maintenance prune --dry-run`, then without `--dry-run`; `--days N` overrides the setting once.
- **Backups** use `VACUUM INTO`, so they are consistent while the daemon writes. The daemon writes `aim-<timestamp>.db` into the backup directory every `backup_every_hours` (default 24) and keeps the newest `backup_keep` (default 7); older files matching that exact name pattern are removed, nothing else. Settings shows the last backup time, size and any error, with "Back up now". The directory is `--backup-dir` / `AIM_BACKUP_DIR` (default `$XDG_STATE_HOME/aim/backups`), overridable in Settings.
- `aim backup [path]` writes one backup from the CLI (to `path`, or into the backup directory with rotation); `task backup` backs up the dev database into `.dev/backups`.

## Releases

Maintainers cut a release by pushing a `v*` tag (for example `git tag -s v0.1.0 && git push origin v0.1.0`); the Release workflow runs GoReleaser for that tag, which uploads the archives and pushes the [container image](#container). CI runs on branches and pull requests only. Users install from the resulting archives; see [Getting started](getting-started.md#install).
