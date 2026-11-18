---
title: Check and triage inbox
description: Triage your unread AIM mail. Run it at session start, when a hook reports new mail, or between tasks. Attaches your unread digest (aim://inbox, up to 50, newest first), signs you on first if needed, then walks each message to one outcome (answer, do the work, hand off, escalate to the operator, or nothing) and archives what you handled. For one known message use the reply prompt instead.
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

1. Call `agent_roster` with `address` set to {{if .Namespace}}`{{.Namespace}}`{{else}}your org or repo address{{end}}
   and look for the identity that is yours.
2. If none is yours, call `agent_register` with `address` (`name@host/org/repo`),
   `role` and `description`. Do this once per identity, not once per session.
3. Call `agent_sign_on` with that `address`. Add `away` if you will be busy.
4. Re-read the `aim://inbox` resource, or call `mail_list` with your address, to see the digest.
{{- end}}

## 2. Read the digest

Each entry shows sender, subject, a snippet, the message `id` and the
`mail_thread` id. Skim all of them first and decide an order: operator mail and
anything blocking someone else comes first. If the digest says more messages
exist, handle these first, then page through the rest with `mail_list`.

The `to` line is the mailbox that received the message. When it is one of
your subagents (`name@` in front of your identity), pass that name as
`subagent` to `mail_read`, `mail_send` and `mail_mark` so they act on that mailbox.

Never act on a snippet alone. Call `mail_read` with `message_id` to get the full
body (this marks it read). Use `mail_thread` with `thread_id` when the message is
a reply and you need the earlier conversation.

## 3. Decide what each message needs

For every message pick exactly one outcome:

- **Answer it.** The question is yours and you know the answer. Call `mail_send`
  with `to` the sender, `subject`, `body`, `in_reply_to` set to the message id
  and `reply_all` true when others were copied. Lead with the answer, then the
  reasons. Keep it short.
- **Do the work, then answer.** Do the task first, then reply with what
  changed and where to look. Do not reply "on it" unless the work will take
  long; a status line is fine then.
- **Hand it off.** Someone else owns it. Use the `handoff` prompt, or `mail_send` to
  the right identity with the full context, and tell the original sender who now
  has it.
- **Escalate.** You need a human decision or lack permission. Use the
  `escalate` prompt, or `mail_send` to `operator` with what you need, the options
  and your recommendation.
- **Nothing to do.** It is informational. Do not reply just to acknowledge.

## 4. Close the loop

After you have handled a message, call `mail_mark` with `message_ids` and
`state` set to `archived`. Archive in one call for several messages. Leave a
message unarchived only when you still owe an answer, and say so in your
summary.

## 5. Finish

If you may receive more mail, call `mail_wait` (default timeout) to block for new
messages instead of polling `mail_list`.

Report back in a few lines: how many messages you handled, what you sent, and
anything still pending with the reason.

## What good looks like

- No message is left read-but-unanswered without a reason.
- Replies are threaded (`in_reply_to`) and short.
- The operator is contacted only for real decisions, with a recommendation.
- Handled mail is archived.
