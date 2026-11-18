---
title: Reply to a message
description: Draft and send a threaded reply to one message, then archive it.
arguments:
  - name: message_id
    required: true
    description: Id of the message to reply to (msg-...)
---
# Reply to {{.Args.message_id}}

The full conversation containing `{{.Args.message_id}}` is attached below as a
transcript resource, oldest first. Read all of it before you write anything:
the latest message is often not the whole question.

{{if not .SignedOn -}}
You are not signed on, so `send` will fail. Call `sign_on` with your registered
identity first (`roster` lists identities; `register` creates one).

{{end -}}

## Steps

1. **Understand the ask.** Identify who wrote to you, what they want, and by
   when. If the message asks several things, list them.
2. **Do any work the reply depends on** (read code, run a check, look up
   another thread with `search`). Reply with results, not promises.
3. **Draft the reply.** Put the answer or decision in the first sentence. Then
   give only the reasons and details the reader needs. Quote nothing the
   thread already shows. If you cannot answer, say what is missing and who can
   supply it.
4. **Choose recipients.** Reply to the sender. Set `reply_all` to true when
   other people were in `to` or `cc` and they need the answer too; leave it
   false for a private answer.
5. **Send.** Call `send` with:
   - `to`: the sender's address (add others only if needed)
   - `subject`: keep the thread's subject; a `Re:` prefix is optional
   - `body`: your reply
   - `in_reply_to`: `{{.Args.message_id}}`
   - `reply_all`: as decided above
6. **Archive the original.** Call `mark` with `message_ids`
   `["{{.Args.message_id}}"]` and `state` `archived`.

## What good looks like

- The first line answers the question.
- The reply is threaded, so the sender sees it with the conversation.
- It names concrete next steps with owners when work remains.
- It is short enough to read on a phone.

If the right answer needs a human, use the `escalate` prompt with this
message's `thread_id` instead of guessing.
