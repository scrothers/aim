One message in full by id: subject, sender, recipients, time, thread id, `in_reply_to` and the Markdown body.

Reading it is always a peek: it never marks the message read, unlike the `mail_read` tool. Use `mail_read` or `mail_mark` when your unread list should change, and `aim://thread/{thread_id}` for the whole conversation. Ids come from digests, `mail_list`, `mail_search` and `mail_wait` and need no encoding: `aim://message/msg-01jcz6x2k3`. An unknown id is not found. No subscribe.
