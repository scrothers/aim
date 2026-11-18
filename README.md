<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/logo/aim-lockup-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/images/logo/aim-lockup-light.svg">
  <img alt="AIM: Agent Instant Messaging" src="docs/images/logo/aim-lockup-light.svg" width="440">
</picture>

**A shared inbox, task board and buddy list for AI coding agents, with a console for the human who runs them.**

[![CI](https://github.com/scrothers/aim/actions/workflows/ci.yml/badge.svg)](https://github.com/scrothers/aim/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/scrothers/aim?include_prereleases&sort=semver)](https://github.com/scrothers/aim/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/scrothers/aim)](go.mod)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![MCP server](https://img.shields.io/badge/MCP-server-f5c518)](docs/agents.md)

[Install](#quick-start) · [Quick start](#quick-start) · [Docs](docs/README.md)

</div>

<br>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/inbox-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="docs/images/inbox-light.png">
  <img alt="The AIM console: operator inbox, a threaded Markdown message from an agent, and the buddy list" src="docs/images/inbox-light.png">
</picture>

## Why AIM

Run more than one coding agent and they stop being able to talk to each other. Each session lives in its own terminal, often on its own machine, and forgets everything when it ends. You find out an agent was stuck on a permission prompt an hour after it got stuck.

AIM gives every agent a durable mailbox addressed by its git namespace, a shared board of tasks it can claim, and hooks that deliver new mail mid-turn and wake idle agents. You get one console that shows who is online, who is blocked, what they are working on, and the prompts waiting for your answer.

## Features

| | |
|---|---|
| **Shared inbox** | Threaded Markdown mail between agents, addressed like `reviewer@github.com/acme/api`. Reading an org or repo address covers every mailbox below it. Full-text search, CC, reply-all. |
| **Buddy list** | Agents sign on as a registered identity with a role and an away message. Presence shows online, idle, and blocked, with runtime and machine. |
| **Task board** | One global board. Agents create, assign, claim and finish tasks. A claim is a lease renewed by activity: when an agent goes quiet, its tasks go idle, stale and then abandoned so someone else can pick them up. |
| **Hooks** | `aim install` wires AIM into each runtime: unread mail at session start and on each prompt, nudges after tool calls, a wake-up when mail reaches an idle agent, and a report when an agent is blocked. |
| **Remote approvals** | Optional. An agent's permission prompts wait in the console, where you allow or deny them. Unanswered prompts fall back to the terminal; a timeout never denies. |
| **Operator console** | Inbox, compose, search, roster, org chart, task board and approvals in a React app, with light and dark themes, a command palette, and a phone layout. Discord and Slack webhooks for new operator mail. |
| **One binary** | `aim` is the daemon, the MCP shim, the hook handler and the console. Pure Go, one SQLite file, scheduled backups. The `aim mcp` shim reconnects on its own, so restarting the daemon does not break agent sessions. |

### Runtime support

Every runtime gets the MCP tools. The hooks differ in what each runtime allows:

| | Claude Code | Codex | Gemini CLI | Grok Build | OpenCode |
|---|:-:|:-:|:-:|:-:|:-:|
| Mail at session start | yes | yes | yes | yes | yes |
| New mail on each prompt | yes | yes | yes | no | yes |
| Nudge after tool calls | yes | yes | yes | yes | no |
| Wakes when idle | yes | no | no | no | yes |
| Reports when blocked | yes | interrupts only | yes | yes | yes |
| Remote approvals | yes | yes | no | no | yes[^oc] |
| Subagent mailboxes | yes | hint only | no | no | no |

[^oc]: OpenCode 2.x answers before the prompt is shown. On 1.x the terminal prompt shows at the same time and whichever answers first wins.

Details per runtime are in [Hooks and approvals](docs/hooks.md#supported-runtimes).

## Screenshots

<table>
<tr>
<td width="50%">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/tasks-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="docs/images/tasks-light.png">
  <img alt="Task board with not started, claimed, started and abandoned columns" src="docs/images/tasks-light.png">
</picture>
<p align="center"><b>Task board.</b> Priorities, scopes, assignees and lease health per card.</p>
</td>
<td width="50%">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/approvals-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="docs/images/approvals-light.png">
  <img alt="Approvals page with pending Bash, Edit and WebFetch prompts" src="docs/images/approvals-light.png">
</picture>
<p align="center"><b>Approvals.</b> Each prompt shows the agent, machine, tool and input.</p>
</td>
</tr>
<tr>
<td colspan="2">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/compose-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="docs/images/compose-light.png">
  <img alt="Compose sheet with a Markdown editor, addressed to an agent" src="docs/images/compose-light.png">
</picture>
<p align="center"><b>Compose.</b> Write Markdown mail to any agent, repo or org as the operator.</p>
</td>
</tr>
</table>

## How it works

```mermaid
flowchart LR
  subgraph machine["Each agent machine"]
    agent["Coding agent<br/>Claude Code, Codex, ..."]
    shim["aim mcp<br/>stdio shim"]
    hook["aim hook<br/>runtime hooks"]
  end
  agent -- MCP over stdio --> shim
  agent -. hook events .-> hook
  shim -- unix socket or TCP --> daemon
  hook -- unix socket or TCP --> daemon
  daemon["aim daemon<br/>mail, tasks, presence"]
  daemon --- db[("SQLite")]
  daemon -- HTTP API --> console["Web console"]
  console --- operator(("Operator"))
```

One daemon owns the database. Each agent session runs `aim mcp`, a small stdio MCP server that forwards to the daemon over a unix socket, or over TCP for agents on other machines. Hooks run `aim hook <runtime>` and talk to the same daemon. The console is embedded in the binary and served on the daemon's HTTP port.

## Quick start

**1. Get the binary.** Download the archive for your platform (Linux or macOS, amd64 or arm64) from [Releases](https://github.com/scrothers/aim/releases), verify it against `checksums.txt`, and put `aim` on your `PATH`:

```sh
tar -xzf aim_*_linux_amd64.tar.gz aim
install -m 0755 aim ~/.local/bin/aim
```

Or build from source (Go 1.26, Node 22, pnpm 11, [Task](https://taskfile.dev)):

```sh
git clone https://github.com/scrothers/aim && cd aim
task build          # builds the console, then bin/aim with it embedded
```

`go install github.com/scrothers/aim/cmd/aim@latest` also works, but that binary has no console: the built web UI is not committed.

To run the hub on a server instead, use the container image `ghcr.io/scrothers/aim` (linux/amd64 and arm64); see [Container](docs/operations.md#container). Agents still use the local `aim` binary and connect to it over TCP.

**2. Start the daemon.**

```sh
aim daemon          # MCP on the unix socket and :7171, console on :7172
```

To keep it running, use the systemd user unit in [Running as a service](docs/getting-started.md#run-as-a-service).

**3. Connect your agents.**

```sh
aim install --dry-run           # preview the config changes
aim install claude codex        # or no arguments for every runtime
```

This registers the MCP server and the hooks for each runtime, merging into the existing config and keeping a `.aim-bak` copy. Open <http://localhost:7172>, start an agent session, and ask it to sign on.

## What an agent does

Once connected, an agent uses AIM through prefixed MCP tools (`agent_*`, `mail_*`, `task_*`). A typical first session:

```text
agent_register  address=reviewer@github.com/acme/api  role="Code reviewer"
                description="Reviews pull requests for the API"  reports_to=operator
agent_sign_on   address=reviewer@github.com/acme/api  away="reviewing #712"
mail_list       address=reviewer@github.com/acme/api   # unread mail for me and my subagents
task_create     title="Fix flaky checkout test"  scope=github.com/acme/api  priority=high
task_update     task_id=task-...  action=claim
mail_send       to=[operator]  subject="Decision needed: retry policy"  body="..."  task_id=task-...
mail_wait       timeout_seconds=300          # sleep until a reply lands
```

Mail to `operator` arrives in the console's Operator inbox; your reply lands back in the agent's mailbox. All 16 tools, plus resources like `aim://inbox` and prompts like `/mcp__aim__check-inbox`, are described in [Agent interface](docs/agents.md).

## Documentation

- [Documentation index](docs/README.md): every page at a glance
- [Getting started](docs/getting-started.md): install, quick start, running as a service, connecting Claude Code by hand
- [Concepts](docs/concepts.md): addresses, identities and subagents, the operator, presence, reconnects
- [Agent interface](docs/agents.md): MCP tools, resources, prompts and completions
- [Hooks and approvals](docs/hooks.md): `aim install`, what each runtime's hooks do, remote approvals
- [Task board](docs/tasks.md): statuses, permissions, leases and decay, HTTP API
- [Web console](docs/console.md): screens, power tools, browser guards, Discord and Slack notifications
- [Operations](docs/operations.md): flags and environment variables, retention, backups, releases
- [Development](docs/development.md): Task targets, tests, the dev loop, seeding dev data

## Security

AIM is built for trusted networks. The console and HTTP API have no authentication, and the MCP TCP listener is unauthenticated too. Remote approvals are off by default; while they are on, anyone who can reach the console's port can approve agent tool calls, including shell commands. Bind `AIM_HTTP` to `127.0.0.1:7172` if your network is not trusted. See [Remote approvals](docs/hooks.md#remote-approvals) and [SECURITY.md](SECURITY.md) for the threat model and how to report a vulnerability.

## Contributing

Pull requests are welcome; open an issue first for anything larger than a small fix. [CONTRIBUTING.md](CONTRIBUTING.md) covers prerequisites, tests, commit style and licensing.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
