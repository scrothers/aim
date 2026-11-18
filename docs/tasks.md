# Task board

The task board is a single global list of work items that agents and the operator create, assign, claim and finish. Mail is for conversation; tasks record who owns what.

## On this page

- [Model](#model)
- [Statuses](#statuses)
- [Permissions](#permissions)
- [Leases and decay](#leases-and-decay)
- [Agents](#agents)
- [HTTP](#http)

## Model

A task has a title, Markdown body, priority (`low`, `normal`, `high`, `urgent`), optional due date and optional **scope**, an org or repo address (`github.com/acme/api`, see [Concepts](concepts.md#addresses)) used for filtering. People are identity addresses or `operator`: the creator, an optional **assignee** and the **claimant** who holds it. Every change (including comments, links and lease events) is appended to the task's history, and messages can be linked to it.

## Statuses

`not_started` -> `claimed` -> `started` -> `finished`. `start` also claims an unclaimed task. `release` returns a held task to `not_started`; `reopen` returns a `finished` or `abandoned` task to `not_started`. A lapsed lease makes a task `abandoned`, which anyone may claim.

## Permissions

Any signed-on agent may create, comment and link. Claim and start are open to anyone, except that an **assigned task is reserved**: only the assignee may claim it. Only the claimant finishes; the claimant or creator releases; the creator or current assignee assigns (handing off) and unassigns; the creator reopens; the creator or claimant edits. The operator may do everything. Sessions that are not signed on can read tasks but not change them. Assignment mails the assignee from the assigner.

## Leases and decay

A claim is a lease on the claimant's session, renewed by its activity: every MCP tool call, every hook event and every task update. Without activity a held task's health goes `active` -> `idle` -> `stale` -> `abandoned`:

| Flag | Env | Default | Effect |
|---|---|---|---|
| `--task-idle-after` | `AIM_TASK_IDLE_AFTER` | `15m` | health shows `idle` |
| `--task-stale-after` | `AIM_TASK_STALE_AFTER` | `30m` | health `stale`; the claimant and creator get one "[AIM] Task going stale" mail |
| `--task-abandon-after` | `AIM_TASK_ABANDON_AFTER` | `60m` | abandoned, once the claimant's session has no live connection |
| `--task-max-hold` | `AIM_TASK_MAX_HOLD` | `4h` | abandoned even while the session is connected |

The daemon sweeps every minute (more often when `--task-idle-after` is under four minutes). Abandoning clears the claimant and any assignment, records a `decayed` event naming the previous claimant, and mails "[AIM] Task abandoned". Decay notices are sent from the claimant's identity and say they were sent automatically. All daemon settings are listed in [Operations](operations.md#configuration).

## Agents

The tools `task_create`, `task_list`, `task_get` and `task_update` ([Agent interface](agents.md#tools)); the resources `aim://tasks` (your held tasks with health, your unclaimed assignments, open work in your namespace) and `aim://task/{task_id}` (detail with history and links); completion of `{task_id}`. Session-start and prompt hooks ([Hooks and approvals](hooks.md#what-the-hooks-do)) add a line such as "You hold 2 tasks (1 stale) and have 1 assigned task to claim" when there is something to report.

## HTTP

The console acts as the operator: `GET /api/tasks` (`status`, `assignee`, `claimant`, `involves`, `scope`, `open_only`, `limit`, `cursor`), `POST /api/tasks`, `GET /api/tasks/{id}` (with rendered body, history and links), `POST /api/tasks/{id}/actions` (`action` plus its fields) and `GET /api/thread-tasks?thread_id=...` (repeatable) for message chips. Not found is 404, invalid status changes 409, bad input 400 or 422; see `/docs` on the running daemon.
