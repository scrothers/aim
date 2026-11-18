---
title: Escalate to the operator
description: Ask the human operator for a decision you cannot or should not make yourself, such as access, money, an irreversible action, conflicting instructions or a blocker no agent can clear. Guides you to write one decision with context, two or three options and a recommendation, send it to operator, then wait for the reply while you continue independent work. With thread_id it attaches that thread and replies in it. Not for progress reports.
arguments:
  - name: topic
    required: true
    description: What needs deciding, as one hyphenated token because slash-command arguments split on spaces, for example deploy-approval or drop-legacy-table. The subject becomes Decision needed: followed by the topic.
  - name: thread_id
    description: Optional id of the thread to continue (msg-...), such as the conversation where the question came up. Its transcript is attached and your message replies in it. Omit to start a new thread.
---
# Escalate: {{.Args.topic}}

The operator is the human running this system. You reach them by sending mail
to the address `operator`. Escalate only for a decision you cannot or should
not make yourself: money, access, irreversible actions, conflicting
instructions, or a blocker no agent can clear. Do not escalate to report
progress or to ask for approval of work that is plainly within your task.

{{if .Args.thread_id -}}
The thread `{{.Args.thread_id}}` is attached below. Read it first so your
message does not repeat what the operator already knows.
{{- else -}}
There is no thread to continue, so this starts a new one.
{{- end}}

{{if not .SignedOn -}}
You are not signed on, so `send` will fail. Call `sign_on` with your registered
identity first.

{{end -}}

## Steps

1. **Define the decision.** Write one sentence: "I need you to decide X." If
   you cannot say it in a sentence, you are not ready to escalate. Finish
   investigating first.
2. **Gather the facts the operator needs** and nothing else: what you were
   doing, what happened, what you tried, and the cost of waiting.
3. **List two or three options.** For each, give the effect and the main
   risk. Include "do nothing" if it is a real choice.
4. **Make a recommendation** and say why in one or two sentences. The operator
   should be able to answer with "yes" or "option 2".
5. **Send** with `send`:
   - `to`: `["operator"]`
   - `subject`: `Decision needed: {{.Args.topic}}`
   - `body`: use the layout below
{{- if .Args.thread_id}}
   - `in_reply_to`: the id of the latest message in thread `{{.Args.thread_id}}`
{{- end}}
6. **Wait for the answer.** Call `wait` with `since_id` set to the `id` that
   `send` returned and `timeout_seconds` up to 300; `address` defaults to your
   identity. Repeat while it reports `timed_out` and you have nothing else to do. While you wait, continue any work that does not
   depend on the answer.
7. **Act on the reply**, then confirm briefly what you did and archive the
   exchange with `mark` (`archived`).

## Body layout

```
Decision: <one sentence>

Context: <two or three sentences>

Options:
1. <option> - <effect>; risk: <risk>
2. <option> - <effect>; risk: <risk>

Recommendation: <option> because <reason>.

Needed by: <time, or "not urgent">
```

## What good looks like

- One decision per message.
- The operator can answer in a single line.
- Everything needed to decide is in the message; no one has to open other
  threads.
- You keep working on independent tasks while waiting.
