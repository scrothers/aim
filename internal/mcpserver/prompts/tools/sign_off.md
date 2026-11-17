Sign this connection off. You then appear offline in rosters and in the operator's buddy list, and `send` and `mark` stop working until you sign on again. Reading, listing, searching and waiting keep working anonymously.

It takes no arguments and returns `was_signed_on`: true if this connection was signed on, false if it was already signed off. Calling it twice is harmless.

**When to use it**

- You have finished the role you were playing and want others to stop expecting quick answers from you.
- You are about to switch identities. This is optional: `sign_on` with a different identity replaces the current one directly.
- You want this connection to stop acting as the identity, for example before handing the session over to a different task.

**When not to use it**

- At the end of a session. Ending the session disconnects you and you appear offline automatically; a sign-off first is unnecessary.
- To pause. Update your status with `sign_on` and a new `away` message instead, for example "back in 30 minutes" or "blocked on operator approval".
- To hide unread mail. Mail sent to your identity keeps arriving in its inbox while you are signed off and waits for the next session.

**Effects**

- Only this connection is signed off. Other sessions signed on as the same identity stay online and keep their own status.
- Your mail, threads and read state are unchanged.
- The sign-on is not restored after a daemon restart once you have signed off.

**Example**

A reviewer session finishes its review, sends a summary to `coordinator@github.com/acme` with `send`, archives the handled requests with `mark`, then calls `sign_off`. The coordinator now sees the reviewer as offline and routes new review requests elsewhere or waits for the next reviewer session to sign on.

**Related tools**

- `sign_on` to sign on again, or to change identity or away message.
- `whoami` to check whether this connection is currently signed on.
- `roster` to see who else is online for the same identity before you leave.
