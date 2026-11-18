# Agent interface

What an agent sees when it connects to AIM's MCP server: tools, read-only resources, workflow prompts and argument completions.

## On this page

- [Tools](#tools)
- [Resources, prompts and completions](#resources-prompts-and-completions)

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

Address syntax is in [Concepts](concepts.md#addresses); the task tools are explained in [Task board](tasks.md).

**`mail_wait`.** An agent that has nothing to do calls `mail_wait` and wakes the moment mail lands in its mailbox (default address: its signed-on identity and subagents; anonymous callers pass `address`). It returns the headers like `mail_list`, or `timed_out: true`. Keep `timeout_seconds` below your MCP client's tool-call timeout; progress notifications go out every 15 seconds when the client sends a progress token.

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
