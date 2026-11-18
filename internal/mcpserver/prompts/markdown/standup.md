---
title: Standup summary
description: Write a read-only standup report of today's mail (since 00:00 UTC) under an address, as progress, in progress, blockers and next steps, each line with an owner and a message id. Run it when asked for status, at the start or end of a day, or before reporting to the operator. Attaches up to 200 read and unread, non-archived deliveries, newest first. It marks nothing read and sends nothing unless you are asked to.
arguments:
  - name: address
    description: Org, repo or identity address to cover, including everything below it, for example github.com/acme for the whole org. Defaults to this session's git namespace; required when the session has no git remote.
---
# Standup for {{.Args.address}}

Summarize what happened under `{{.Args.address}}` since the start of {{.Today}}
(UTC). The messages are attached below as a digest, newest first. The digest
includes read mail but not archived mail, and is capped at 200 entries. A
message delivered to several mailboxes appears once per mailbox.

## Steps

1. **Read the digest.** Group messages by thread, not by message: a thread with
   five replies is one story. Use `thread` with `thread_id` for any thread you
   cannot follow from the snippets, and `read` with `peek` set to true when you
   need a full body without marking it read.
2. **Sort each thread** into one of: finished, in progress, blocked, or
   decision pending. A thread is blocked when someone is waiting on someone
   else, or an agent has asked the operator a question that has no reply.
3. **Check who is around.** Call `roster` with `address`
   `{{.Args.address}}` to see which identities are online or away; mention
   anyone who is silent but holds work.
4. **Write the summary** in the layout below. Name identities and message ids so
   readers can follow up.

## Output layout

```
Standup for <address>, <date>

Progress
- <identity>: <what finished> (<thread or message id>)

In progress
- <identity>: <what is under way>

Blockers
- <what is stuck>, waiting on <identity or operator> since <time> (<id>)

Next
- <identity>: <next action>
```

## What good looks like

- Every line names an owner and a source id.
- Quiet days say so in one line instead of padding.
- Blockers are specific about who can unblock them.
- You do not mark mail read, archive or reply while writing a standup; this is
  a read-only report. Send it only if the user asks (`send`, to the address
  they name).
