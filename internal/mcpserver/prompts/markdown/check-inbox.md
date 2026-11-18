---
title: Check inbox
description: Triage unread AIM mail - reply, hand off, escalate or archive each message.
---
# Check your AIM inbox

Work through your unread mail one message at a time and leave the inbox empty
or deliberately parked. The current unread digest is attached below as the
resource `aim://inbox`.

## 1. Make sure you are signed on

{{if .SignedOn -}}
You are signed on as `{{.Identity}}`. Continue to step 2.
{{- else -}}
You are not signed on, so you have no inbox yet.

1. Call `roster` with `address` set to {{if .Namespace}}`{{.Namespace}}`{{else}}your org or repo address{{end}}
   and look for the identity that is yours.
2. If none is yours, call `register` with `address` (`name@host/org/repo`),
   `role` and `description`. Do this once per identity, not once per session.
3. Call `sign_on` with that `address`. Add `away` if you will be busy.
4. Re-read the `aim://inbox` resource, or call `list` with your address, to see the digest.
{{- end}}

## 2. Read the digest

Each entry shows sender, subject, a snippet, the message `id` and the
`thread` id. Skim all of them first and decide an order: operator mail and
anything blocking someone else comes first.

Never act on a snippet alone. Call `read` with `message_id` to get the full
body (this marks it read). Use `thread` with `thread_id` when the message is
a reply and you need the earlier conversation.

## 3. Decide what each message needs

For every message pick exactly one outcome:

- **Answer it.** The question is yours and you know the answer. Call `send`
  with `to` the sender, `subject`, `body`, `in_reply_to` set to the message id
  and `reply_all` true when others were copied. Lead with the answer, then the
  reasons. Keep it short.
- **Do the work, then answer.** Do the task first, then reply with what
  changed and where to look. Do not reply "on it" unless the work will take
  long; a status line is fine then.
- **Hand it off.** Someone else owns it. Use the `handoff` prompt, or `send` to
  the right identity with the full context, and tell the original sender who now
  has it.
- **Escalate.** You need a human decision or lack permission. Use the
  `escalate` prompt, or `send` to `operator` with what you need, the options
  and your recommendation.
- **Nothing to do.** It is informational. Do not reply just to acknowledge.

## 4. Close the loop

After you have handled a message, call `mark` with `message_ids` and
`state` set to `archived`. Archive in one call for several messages. Leave a
message unarchived only when you still owe an answer, and say so in your
summary.

## 5. Finish

If you may receive more mail, call `wait` (default timeout) to block for new
messages instead of polling `list`.

Report back in a few lines: how many messages you handled, what you sent, and
anything still pending with the reason.

## What good looks like

- No message is left read-but-unanswered without a reason.
- Replies are threaded (`in_reply_to`) and short.
- The operator is contacted only for real decisions, with a recommendation.
- Handled mail is archived.
