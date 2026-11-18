---
title: Hand off work
description: Pass work you own to another identity so it can continue without asking you anything. Run it when you stop mid-task, when the work belongs to another role or repo, or when a request should go elsewhere. Guides you to check the recipient with roster, send a self-contained handoff (goal, state, next steps, exact paths and ids, risks), set your away message to say where the work went, and tell whoever is waiting. Attaches nothing.
arguments:
  - name: to
    required: true
    description: Identity address of the new owner, name@host/org/repo or name@host/org, for example reviewer@github.com/acme/api. Completions offer identities in your org, online ones first.
  - name: summary
    description: Optional topic hint such as migrations. Use one word or hyphenate it, because slash-command arguments split on spaces; the script has you write the full context anyway.
---
# Hand off to {{.Args.to}}

You are passing work to `{{.Args.to}}`.{{if .Args.summary}} Topic hint: {{.Args.summary}}.{{end}} They have none of
your context, so the message must stand alone.

{{if not .SignedOn -}}
You are not signed on, so `mail_send` will fail. Call `agent_sign_on` with your registered
identity first.

{{end -}}

## Steps

1. **Check the recipient.** Call `agent_roster` with `address` `{{.Args.to}}` to
   confirm the identity exists and see whether they are online. If the call
   fails with "not found", read the suggested addresses in the error.
2. **Collect the facts.** Before writing, establish:
   - the goal, in one sentence
   - the current state: what is done, what is half-done, what is untouched
   - the next steps, in order, with the first one concrete enough to start now
   - where things live: repo, branch, files, commit ids, ticket or message
     ids (`msg-...` or thread ids from `mail_list` and `mail_thread`)
   - open questions, risks and anything you tried that did not work
3. **Write the message** using the layout below.
4. **Send it** with `mail_send`:
   - `to`: `["{{.Args.to}}"]`
   - `subject`: `Handoff: <short topic>`
   - `body`: the layout below
   - `cc`: add your own `reports_to` identity only if they should know
5. **Update your status.** Call `agent_sign_on` again with the same `address` and an
   `away` message such as `handed off <topic> to {{.Args.to}}`, so others see
   where the work went.
6. **Tell the people waiting.** If the work came from a message you still
   hold, reply to that thread (`mail_send` with `in_reply_to`) naming the new owner,
   and archive it with `mail_mark`.

## Body layout

```
Goal: <one sentence>

State:
- done: ...
- in progress: ...
- not started: ...

Next steps:
1. <first step, concrete>
2. ...

Where things are: <repo/branch/files/ids>

Open questions and risks: <list, or "none">

Tried and failed: <list, or "nothing notable">
```

## What good looks like

- The recipient can start without asking you a question.
- Every id, path and branch is exact, not "the usual one".
- The first next step is something they can run immediately.
- The original requester knows who owns the work now.
