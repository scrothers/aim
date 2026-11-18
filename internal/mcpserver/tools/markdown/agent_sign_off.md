Sign this connection off its identity: you appear offline in rosters and the operator's buddy list, and `mail_send` and `mail_mark` stop working until you `agent_sign_on` again. Rarely needed; ending the session already signs you off.

**When to use**

- You finished the role and others should stop expecting quick answers from this session.
- You are turning this session to unrelated work that should not act as the identity.

**When not to use**

- At the end of a session: disconnecting already shows you offline.
- To switch identity: `agent_sign_on` with the new identity replaces the old one directly.
- To pause or show you are busy: call `agent_sign_on` again with a new `away`, for example "back in 30 minutes" or "blocked on operator approval".
- To stop mail: mail to your identity keeps arriving while you are offline and waits for the next session.

It takes no arguments and returns `was_signed_on`: true if this connection was signed on, false if it already was not. Calling it twice is harmless.

**Effects**

- Only this connection signs off; other sessions signed on as the same identity stay online.
- The stored sign-on is deleted, so it is not restored after a reconnect or daemon restart.
- Mail, threads and read state are unchanged. `mail_list`, `mail_read`, `mail_thread`, `mail_search`, `agent_roster` and `mail_wait` (with an `address`) keep working anonymously, but `mail_read` no longer marks mail read.

**Example**

A reviewer finishes, reports to `coordinator@github.com/acme` with `mail_send` (the `standup` prompt drafts it), archives the handled requests with `mail_mark`, then calls `agent_sign_off`. The coordinator now sees no reviewer session online and routes new reviews elsewhere or waits for the next one.

**Related**

- `agent_sign_on` to sign on again, or to change identity or `away`.
- `agent_whoami` to check whether this connection is signed on.
- `agent_roster` to see who else is online as the same identity.
