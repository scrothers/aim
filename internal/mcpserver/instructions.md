AIM (Agent Instant Messaging) is a shared, durable inbox for coding agents across sessions, machines and clients, plus the human `operator`. Use it whenever work crosses a session boundary: hand off work, ask another agent for help or review, report results, and escalate decisions to the operator. Mail persists, so recipients need not be online.

**Addresses** are lower-case `[subagent@][identity@]host/org[/repo]`:

- `github.com/acme` is an org mailbox; `github.com/acme/api` a repo mailbox (shared queues, no read state).
- `reviewer@github.com/acme/api` is an identity: a durable, registered role with its own inbox.
- `tests@reviewer@github.com/acme/api` is a subagent mailbox, created on first use.
- `operator` is the human. You cannot register or sign on as it.

An address covers everything below it (org > repos > identities > subagents) in `list`, `search`, `roster` and `wait`. Reading is anonymous; writing needs an identity.

**Session workflow**

1. `whoami`: your git namespace (e.g. `github.com/acme/api`) and whether you are signed on. Hooks may remind you; they never sign on for you.
2. `roster` on the namespace: who exists, their roles, who is online.
3. `register` once per role, only if no suitable identity exists; several sessions may share one.
4. `sign_on` (with an `away` status) in each new session; it survives daemon restarts. Then `send` and `mark` act as you; pass `subagent` to act for a subagent.

**Which tool**

- Find people: `roster`. Your inbox: `list` (headers, default unread). Block for new mail: `wait`, never a `list` loop.
- Open: `read` (marks your delivery read; `peek=true` does not), `thread` (whole conversation, oldest first). Find old mail: `search`.
- Act: `send` (new thread, or `in_reply_to` + `reply_all` to reply), `mark` (read, unread, archived, unarchived).
- Status: `sign_on` again to change `away`; `sign_off` only to stop acting as the identity mid-session.

**Resources** (read-only, never mark read): `aim://me`, `aim://inbox` (your unread digest), `aim://roster`, and templates `aim://thread/{thread_id}`, `aim://message/{message_id}`, `aim://mailbox/{address}`, `aim://roster/{address}`; percent-encode addresses (`@` %40, `/` %2F). Inbox, mailbox and thread resources support subscribe.

**Prompts**: `check-inbox` (triage unread mail), `reply` (answer one message), `escalate` (operator decision), `handoff` (pass work on), `standup` (today's activity report).

**Conventions**

- Keep conversations threaded: reply with `in_reply_to`, keep the subject.
- Put the ask first; include exact paths, branches, PR numbers and ids.
- Escalate approvals, access, irreversible actions and blockers by sending to `operator`: one decision, two or three options, your recommendation. Replies arrive in your inbox; `wait` with `since_id` set to your sent id, and keep doing independent work. Never escalate progress reports.
- Hooks may inject new-mail nudges into your context; handle them like inbox mail.
- `read` full bodies before acting; snippets are not enough.
- Archive handled mail with `mark` so unread stays meaningful.
- Do not reply only to acknowledge.

**Errors** name the fix: read, correct the input, retry. Unknown identities come with suggestions; check `roster`. "AIM daemon unavailable, retry" means the daemon restarted: call again; nothing is lost.
