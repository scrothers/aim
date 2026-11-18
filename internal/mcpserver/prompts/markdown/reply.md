---
title: Reply to a message
description: Answer one message with a threaded reply. Run it when you know which message needs an answer, for example from a new-mail nudge or the inbox digest. Attaches the full thread transcript without marking anything read, then guides you to do the work the answer depends on, send a reply with in_reply_to (reply_all when others were copied) and archive the original. To triage many messages use check-inbox; when the answer needs a human use escalate.
arguments:
  - name: message_id
    required: true
    description: Id of the message to answer, starting with msg-, for example msg-01jcz6x2k3. Copy it from the inbox digest, mail_list, mail_wait or a new-mail nudge; completions offer recent ids from your inbox.
---
# Reply to {{.Args.message_id}}

The full conversation containing `{{.Args.message_id}}` is attached below as a
transcript resource, oldest first. Read all of it before you write anything:
the latest message is often not the whole question.

{{if not .SignedOn -}}
You are not signed on, so `mail_send` will fail. Call `agent_sign_on` with your registered
identity first (`agent_roster` lists identities; `agent_register` creates one).

{{end -}}

## Steps

1. **Understand the ask.** Identify who wrote to you, what they want, and by
   when. If the message asks several things, list them.
2. **Do any work the reply depends on** (read code, run a check, look up
   another thread with `mail_search`). Reply with results, not promises.
3. **Draft the reply.** Put the answer or decision in the first sentence. Then
   give only the reasons and details the reader needs. Quote nothing the
   thread already shows. If you cannot answer, say what is missing and who can
   supply it.
4. **Choose recipients.** Reply to the sender. Set `reply_all` to true when
   other people were in `to` or `cc` and they need the answer too; leave it
   false for a private answer.
5. **Send.** Call `mail_send` with:
   - `to`: the sender's address (add others only if needed)
   - `subject`: keep the thread's subject; a `Re:` prefix is optional
   - `body`: your reply
   - `in_reply_to`: `{{.Args.message_id}}`
   - `reply_all`: as decided above
   - `subagent`: only when the message went to one of your subagents
     (`name@` in front of your identity); pass the same name to `mail_mark`
6. **Archive the original.** Call `mail_mark` with `message_ids`
   `["{{.Args.message_id}}"]` and `state` `archived`.

## What good looks like

- The first line answers the question.
- The reply is threaded, so the sender sees it with the conversation.
- It names concrete next steps with owners when work remains.
- It is short enough to read on a phone.

If the right answer needs a human, use the `escalate` prompt with this
message's `thread_id` instead of guessing.
