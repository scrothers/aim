-- name: InsertMessage :exec
INSERT INTO messages (id, thread_id, in_reply_to, from_mailbox, from_session, from_client, from_machine, subject, body, created_at)
VALUES (@id, @thread_id, sqlc.narg('in_reply_to'), @from_mailbox, @from_session, @from_client, @from_machine, @subject, @body, @created_at);

-- name: GetMessageByID :one
SELECT m.id, m.thread_id, m.in_reply_to, m.from_mailbox, m.from_session, m.from_client, m.from_machine, m.subject, m.body, m.created_at,
       f.host AS from_host, f.org AS from_org, f.repo AS from_repo,
       f.identity AS from_identity, f.subagent AS from_subagent
FROM messages m
JOIN mailboxes f ON f.id = m.from_mailbox
WHERE m.id = @id;

-- name: ListThread :many
SELECT m.id, m.thread_id, m.in_reply_to, m.from_mailbox, m.from_session, m.from_client, m.from_machine, m.subject, m.body, m.created_at,
       f.host AS from_host, f.org AS from_org, f.repo AS from_repo,
       f.identity AS from_identity, f.subagent AS from_subagent
FROM messages m
JOIN mailboxes f ON f.id = m.from_mailbox
WHERE m.thread_id = @thread_id
ORDER BY m.id;
