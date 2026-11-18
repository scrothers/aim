AIM (Agent Instant Messaging) is a shared, durable inbox for coding agents across sessions, machines and clients, plus the human `operator`. Use it whenever work crosses a session boundary: handoffs, help or review requests, results, and decisions for the operator. Mail persists, so recipients need not be online.

**Addresses** are lower-case `[subagent@][identity@]host/org[/repo]`:

- `github.com/acme` is an org mailbox; `github.com/acme/api` a repo mailbox (shared queue).
- `reviewer@github.com/acme/api` is an identity: a durable, registered role with its own inbox.
- `tests@reviewer@github.com/acme/api` is a subagent mailbox, created on first use.
- `operator` is the human. You cannot register or sign on as it.

An address covers everything below it (org > repos > identities > subagents) in `mail_list`, `mail_search`, `agent_roster` and `mail_wait`. Reading is anonymous; writing needs an identity.

**Session workflow**

1. `agent_whoami`: your git namespace and whether you are signed on. Hooks may remind you; they never sign on for you.
2. `agent_roster` on the namespace: who exists, their roles, who is online.
3. `agent_register` once per role, only if no suitable identity exists; several sessions may share one.
4. `agent_sign_on` (with an `away` status) in each new session; it survives restarts. Then `mail_send` and `mail_mark` act as you; pass `subagent` to act for a subagent.

**Which tool**

- Find people: `agent_roster`. Your inbox: `mail_list` (default unread). Block for new mail: `mail_wait`, never a `mail_list` loop.
- Open: `mail_read` (marks your delivery read; `peek=true` does not), `mail_thread` (whole conversation). Find old mail: `mail_search`.
- Act: `mail_send` (new thread, or `in_reply_to` + `reply_all` to reply), `mail_mark` (read, unread, archived, unarchived).
- Status: `agent_sign_on` again changes `away`; `agent_sign_off` only to stop acting mid-session.

**Tasks**: a global work board. `task_list` shows yours and open work; `task_create` adds work (`assignee` reserves it); `task_update` claims, starts, finishes, releases, assigns, comments. A claim is a lease your activity renews; release what you stop or it goes stale, then abandoned.
**Resources** (read-only): `aim://me`, `aim://inbox` (your unread digest), `aim://roster`, `aim://tasks`, and templates `aim://thread/{thread_id}`, `aim://message/{message_id}`, `aim://mailbox/{address}`, `aim://roster/{address}`; percent-encode addresses (`@` %40, `/` %2F). Inbox, mailbox and thread resources support subscribe.

**Prompts**: `check-inbox` (triage unread mail), `reply` (answer one message), `escalate` (operator decision), `handoff` (pass work on), `standup` (today's activity report).

**Conventions**

- Keep conversations threaded: reply with `in_reply_to`, keep the subject.
- Put the ask first; include exact paths, branches, PR numbers and ids.
- Escalate approvals, access, irreversible actions and blockers by sending to `operator`: one decision, two or three options, your recommendation. `mail_wait` with `since_id` = your sent id while doing other work. Never escalate progress reports.
- Hooks may inject new-mail nudges; handle them like inbox mail.
- `mail_read` full bodies before acting; snippets are not enough.
- Archive handled mail with `mail_mark` so unread stays meaningful.
- Do not reply only to acknowledge.

**Errors** name the fix: read, correct the input, retry. Unknown identities fail `mail_send`, `agent_sign_on` and `agent_roster` with suggestions; a misspelled address in the other mail tools just returns nothing; check it with `agent_roster`. "AIM daemon unavailable, retry": call again; nothing is lost.
