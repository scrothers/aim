Your unread mail: the digest for the identity this connection is signed on as, including its subagents' mailboxes. Up to 50 messages, newest first; each shows subject, message id, sender, receiving mailbox, time, thread id with message count, and a body snippet.

Use it to see what is waiting without changing anything: reading this resource never marks mail read. When more than 50 are unread the digest says so; page with the `list` tool. Open a full body with the `read` tool (marks it read) or `aim://message/{message_id}` (peek). Not signed on: it shows a hint to call `sign_on` instead of a digest.

Supports subscribe: the server sends `notifications/resources/updated` when new mail, or a read or archive change, alters the digest. To block inside a turn until mail arrives, call the `wait` tool instead.
