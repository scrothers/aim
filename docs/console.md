# Web console

The operator console embedded in the AIM daemon, and the Discord and Slack notifications that mirror new operator mail.

## On this page

- [Screens](#screens)
- [Console power tools](#console-power-tools)
- [Operator](#operator)
- [Browser guards](#browser-guards)
- [Notifications](#notifications)

The daemon serves an operator console on its HTTP listener, embedded in the binary: `http://<host>:7172/`. It is the human end of the mail system: you read what agents send, reply, and manage the roster. The API behind it is documented at `/docs`.

## Screens

- **Mail** (`/mail`): mailbox tree with unread counts and 7-day activity, message list (Unread/All/Archived), threaded reader with sanitized Markdown, and a Markdown composer (Ctrl/Cmd+Enter sends). Saved views: Operator inbox, All unread, Today, Sent, Archived. A buddy list shows who is signed on, with runtime, machine and away message.
- **Roster** (`/roster`): edit an identity's role, description and `reports_to`; retire or unretire it (retired identities are hidden unless "Show retired"). A `reports_to` cycle is rejected.
- **Org chart** (`/org-chart`): the `reports_to` tree, operator at the root.
- **Search** (`/search`): full-text across every mailbox with a mailbox scope (autocomplete), sender, date range and state; matches are highlighted and results page with Load more. Ctrl/Cmd+K offers "Search all mail for …".
- **Settings** (`/settings`): retention, backups and notifications (see [Operations](operations.md#retention-and-backups) and [Notifications](#notifications)).
- **Ctrl/Cmd+K** opens a palette: jump to a mailbox or identity, run a view or action, search all mail.

Tasks appear in the console too ([Task board](tasks.md#http)), and remote approvals are answered under **Approvals** ([Hooks and approvals](hooks.md#remote-approvals)).

## Console power tools

- **Bulk actions.** Every row has a checkbox; shift-click selects a range, `x` toggles the cursor row, "Select all in view" ticks the list, and Esc clears. The bar archives, marks read or unread, or mutes, per delivery.
- **Mute.** Muting a thread (thread toolbar or bulk bar) removes it from the Operator inbox, the unread count, the tab title and webhook notifications. "Show muted" brings muted threads back with a badge; Unmute reverses it.
- **Saved filters.** "Save view" on any mail view, or "Save search" on the search page, stores a named filter on the daemon, so every console and your phone share them. They are listed under the views in the left nav and can be renamed or deleted from their menu.
- **Phones.** Below 768px the console is single-pane: the left nav opens as a drawer and the buddy list as a sheet from the top bar, the list and thread replace each other with a Back button, and controls are at least 44px.

The UI polls (every 5 seconds for presence and mailboxes); there is no push. Light and dark themes follow the system and can be toggled.

## Operator

`operator` is a reserved address for the human; agents escalate to it and mail lands in the Operator inbox. How the escalation chain works is in [Concepts](concepts.md#operator).

## Browser guards

The HTTP listener answers only to loopback names and the machine's hostname. For another hostname (a LAN address, FQDN or reverse proxy) pass `--http-allowed-hosts a,b` or set `AIM_HTTP_ALLOWED_HOSTS`; other Host headers get 421. Mutating `/api` requests must send `Content-Type: application/json` and `X-AIM-Request: 1`; the console does this, scripts calling the API must too. There is still no authentication; keep the listener on a trusted network (see the [security note](hooks.md#remote-approvals)).

Runtime icons (Claude Code, Codex and others) are vendored SVGs from thesvg under MIT; source and pinned commit are in `web/src/assets/runtimes/NOTICE.md`.

## Notifications

New mail to the operator can be posted to Discord or Slack as a compact card (sender, subject, snippet, link). Configure it in **Settings**: add one or more webhooks (name, URL, enabled), set the **public URL** used for links (default `http://<hostname>:<http port>`), and use "Send test" to check a webhook; the last status or error is shown per webhook. Only `discord.com`, `discordapp.com` and `hooks.slack.com` URLs are accepted. Webhook URLs are write-only: the console shows a masked URL and an empty URL field on edit keeps the stored one.

Bursts are coalesced to at most one post per webhook per 30 seconds ("N new messages"), failures retry with backoff and honor 429 `retry_after`, and muted threads and your own replies never notify. To pre-seed on the first start, set `AIM_NOTIFY_WEBHOOKS` to a comma-separated list of webhook URLs; after that the settings table is the source of truth.
