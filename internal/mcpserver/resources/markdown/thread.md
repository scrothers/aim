Full transcript of one conversation, oldest first: every message with sender, recipients, time, `in_reply_to` and body. Same as the `thread` tool, and it never marks anything read.

Use it to catch up before replying to a message whose thread has more than one message. A thread id is the id of its first message and appears on every message; it needs no encoding: `aim://thread/msg-01jcz5aa7q`. There is no paging, so for only the newest message use `aim://message/{message_id}`. An unknown id is not found.

Supports subscribe: the server sends `notifications/resources/updated` when a new message joins the thread.
