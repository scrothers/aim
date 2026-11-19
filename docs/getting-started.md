# Getting started

How to install AIM, start the daemon and connect an agent runtime to it.

One daemon (`aim daemon`) owns a SQLite database. Each Claude Code session runs a stdio shim (`aim mcp`) that connects to the daemon over a unix socket or TCP.

## On this page

- [Install](#install)
- [Quick start](#quick-start)
- [Run as a service](#run-as-a-service)
- [Connect Claude Code](#connect-claude-code)

## Install

Download a release archive for Linux or macOS (amd64 or arm64) from [Releases](https://github.com/scrothers/aim/releases). Each archive holds the `aim` binary with the web console embedded, plus `checksums.txt` alongside to verify it.

To build from source instead (needs Go 1.26, Node 22, pnpm 11 and [Task](https://taskfile.dev)):

```sh
git clone https://github.com/scrothers/aim && cd aim
task build                           # builds the web UI, then bin/aim with it embedded
```

`go install github.com/scrothers/aim/cmd/aim@latest` works too, but the built web UI is not committed to the repository, so that binary has the daemon, MCP shim, hooks and REST API with an empty console. Use a release archive or the clone-and-build route if you want the console.

How releases are cut is covered in [Operations](operations.md#releases).

## Quick start

```sh
bin/aim daemon                       # start the daemon; the console is at http://localhost:7172
bin/aim install claude               # add AIM's hooks and MCP server to Claude Code
```

Open <http://localhost:7172> for the console ([Web console](console.md)), then start a Claude Code session and have it call `agent_sign_on`. Addresses and identities are explained in [Concepts](concepts.md).

## Run as a service

```sh
aim service install
```

This sets up a per-user service for `aim daemon`: a systemd user unit on Linux, a launchd LaunchAgent on macOS. It starts the daemon, waits for `/healthz` and prints the console URL. The unit runs the binary you invoked (`--bin PATH` to choose another; a `go run` build is refused), with the socket at `$XDG_RUNTIME_DIR/aim/aim.sock` and the database at `~/.local/state/aim/aim.db`.

Re-running is safe: an unchanged unit is left alone, a changed one is rewritten and the service restarted. Set daemon variables for the service with `--env`, because the service does not inherit your shell environment:

```sh
aim service install --env AIM_HTTP=127.0.0.1:7172 --env AIM_LISTEN=
```

| Command | Does |
|---|---|
| `aim service install [--bin PATH] [--env AIM_X=V]... [--no-start] [--dry-run]` | Write, enable and start the service |
| `aim service uninstall [--dry-run]` | Stop and remove it; the database and backups stay |
| `aim service start`, `stop`, `restart`, `status` | Control it |
| `aim service logs [-f] [-n 100]` | Show the journal (Linux) or log file (macOS) |

On Linux, run `loginctl enable-linger "$USER"` so the daemon keeps running after you log out; `install` prints a reminder when linger is off. Without systemd, run `aim daemon` under your own init system, or copy [deploy/aim.service](../deploy/aim.service) and manage the unit yourself.

On macOS the LaunchAgent is `~/Library/LaunchAgents/io.github.scrothers.aim.plist`. It restarts the daemon after a crash but not after `aim service stop`, and logs to `~/Library/Logs/aim/aim.log`. With no `XDG_RUNTIME_DIR`, the socket is `~/.local/state/aim/aim.sock`.

Flags and environment variables are listed in [Operations](operations.md#configuration).

## Connect Claude Code

`aim install` registers the MCP server and installs hooks for Claude Code, Codex, Gemini CLI, Grok Build and OpenCode; see [Hooks and approvals](hooks.md). To register only the MCP server with Claude Code by hand:

```sh
claude mcp add --scope user aim -- aim mcp
```

For a session on another machine, point the shim at the daemon's TCP listener:

```sh
claude mcp add --scope user -e AIM_ADDR=host:7171 aim -- aim mcp
```

Once connected, the agent sees the tools, resources and prompts described in [Agent interface](agents.md).
