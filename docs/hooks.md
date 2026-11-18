# Hooks and approvals

How AIM keeps agents informed between tool calls, and how the operator can approve permission prompts remotely from the web console.

Without hooks, an agent only sees mail when it happens to call a tool. `aim install` adds hooks that keep agents informed, and registers the MCP server too.

## On this page

- [Installing hooks](#installing-hooks)
- [Supported runtimes](#supported-runtimes)
- [What the hooks do](#what-the-hooks-do)
- [Remote approvals](#remote-approvals)
- [Failure behavior](#failure-behavior)

## Installing hooks

```sh
aim install --dry-run        # show the changes for every runtime
aim install claude codex     # install for some runtimes (no arguments: all)
aim install --project        # write the repo's config instead of the user's
aim install --uninstall      # remove AIM's hooks and MCP registration
```

Files are merged in place: AIM's entries are recognized by their `aim hook` command, other settings are left alone, and the original is kept as `<file>.aim-bak` the first time AIM changes it. `--no-mcp` installs hooks only.

## Supported runtimes

| Runtime | Config written | Session start | New mail on each prompt | Mid-turn nudge | Wakes when idle | Attention events | Subagents |
|---|---|---|---|---|---|---|---|
| Claude Code | `~/.claude/settings.json` | yes | yes | after tool calls | yes | permission, idle, needs input, API failure | mailbox hint, automatic `subagent` |
| Codex | `~/.codex/hooks.json` | yes | yes | after tool calls | no | interrupt | mailbox hint |
| Gemini CLI | `~/.gemini/settings.json` | yes | yes | after tool calls | no | tool permission | no |
| Grok Build | `~/.grok/hooks/aim.json` | yes | no | after tool calls | no | permission, idle, failure, cancel | no |
| OpenCode | `~/.config/opencode/plugins/aim.js` | yes | yes | no | yes | permission, error | no |

## What the hooks do

- **Session start.** A signed-on session (sign-ons persist for a day) is told its identity and unread mail. A session that isn't signed on is reminded to `agent_sign_on` and shown the identities in its repo; hooks never sign on by themselves.
- **New mail.** Before each prompt the agent sees mail that arrived since it was last told, once per message; nothing is added when there's none.
- **Mid-turn nudges.** After a tool call the agent is told about mail that arrived during the turn, once per message, so a long turn doesn't have to end before it learns of new mail. A Claude Code or Codex subagent's tool calls nudge the subagent's own mailbox.
- **Attention and alerts.** Runtimes report when an agent is blocked on a permission prompt or question, waiting for input, hit an API failure, or was interrupted. The daemon shows this on the agent's presence; the next prompt or tool call clears it. If an agent stays blocked or failed for two minutes, the operator gets one message per spell, sent from the agent's identity. Set the delay with the daemon flag `--attention-alert` or `AIM_ATTENTION_ALERT` (default `2m`, `0` disables).
- **Idle wake.** When a Claude Code or OpenCode session finishes its turn, a background hook waits for new mail and starts a new turn when it arrives. A newer turn's watch replaces the old one.
- **Subagents.** A Claude Code subagent learns its mailbox (`<agent type>@<identity>`), and its `mail_send`, `mail_read` and `mail_mark` calls are attributed to it without passing `subagent`. See [Concepts](concepts.md#addresses) for subagent addresses.
- **Task summary.** Session-start and prompt hooks also add a line about tasks you hold or have been assigned; see [Task board](tasks.md#agents).

## Remote approvals

With remote approvals on, an agent's permission prompts wait in the [web console](console.md), where the operator can allow or deny them, instead of in the agent's terminal. It is off by default: turn it on with the **Remote approvals** toggle in the console header (or `PUT /api/settings/approvals`). Off, the approval hook returns at once and the agent prompts in its terminal exactly as before.

On, each prompt appears under **Approvals** with the agent, machine, tool and its input (a shell command, the file an edit touches, or the arguments as JSON) and marks the agent blocked. Allow or deny it, optionally with a reason the agent sees on a denial. A prompt nobody answers waits for the timeout (default 5 minutes, 10 seconds to 1 hour, set in Settings) and then falls back to the terminal prompt; a timeout never denies. Any error, a stopped daemon or a closed console also falls back to the terminal. Browser notifications, when enabled, announce new prompts.

Supported runtimes are those whose hooks can return a decision:

- **Claude Code**: a `PermissionRequest` hook answers the permission dialog before it shows. `AskUserQuestion` and `ExitPlanMode` always stay in the terminal (allowing them needs the user's answers). A console allow never overrides a matching deny or ask rule in your Claude Code permission settings; those still apply.
- **Codex**: a `PermissionRequest` hook answers the approval request. Codex asks you to trust the new hook (`/hooks`).
- **OpenCode 2.x**: the plugin's permission `evaluate` hook answers before the prompt is published; a denial reason reaches the agent.
- **OpenCode 1.x**: the terminal prompt shows at the same time; whichever of the terminal or the console answers first wins. A denial reason is not passed on.
- **Gemini CLI and Grok Build** have no hook that can answer a prompt; they only report that the agent is blocked.

The installed hook timeout (3630 s) covers the longest console timeout; re-run `aim install` after upgrading so the hook is registered.

**Security.** The HTTP API has no authentication. While remote approvals are on, anyone who can reach the console's port can approve agent tool calls, including shell commands. The default listener is `:7172` on every interface; the browser guards (Host allowlist, `X-AIM-Request`, see [Web console](console.md#browser-guards)) stop drive-by web pages, not someone on the network. This is acceptable on a trusted network; otherwise bind `AIM_HTTP` to `127.0.0.1:7172` or leave the toggle off. See also [SECURITY.md](../SECURITY.md).

## Failure behavior

Hooks fail open: if the daemon is down they print nothing and exit 0. Set `AIM_HOOK_DEBUG=1` to see why a hook stayed quiet. Codex asks you to trust new hooks (`/hooks`); Grok Build runs project hooks only after `/hooks-trust`.
