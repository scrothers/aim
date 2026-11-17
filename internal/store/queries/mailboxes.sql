-- name: GetMailboxByKey :one
SELECT * FROM mailboxes
WHERE host = @host AND org = @org AND repo = @repo AND identity = @identity AND subagent = @subagent;

-- name: GetMailboxByID :one
SELECT * FROM mailboxes WHERE id = @id;

-- name: InsertMailbox :exec
INSERT INTO mailboxes (id, host, org, repo, identity, subagent, kind, role, description, reports_to, created_at)
VALUES (@id, @host, @org, @repo, @identity, @subagent, @kind, @role, @description, @reports_to, @created_at);

-- name: ListIdentitiesInOrg :many
SELECT * FROM mailboxes
WHERE kind = 'identity' AND host = @host AND org = @org
  AND (CAST(@include_retired AS INTEGER) = 1 OR retired_at IS NULL)
ORDER BY repo, identity;

-- name: ListIdentitiesInRepo :many
SELECT * FROM mailboxes
WHERE kind = 'identity' AND host = @host AND org = @org AND repo = @repo
  AND (CAST(@include_retired AS INTEGER) = 1 OR retired_at IS NULL)
ORDER BY repo, identity;

-- name: ListIdentitiesInIdentity :many
SELECT * FROM mailboxes
WHERE kind = 'identity' AND host = @host AND org = @org AND repo = @repo AND identity = @identity
  AND (CAST(@include_retired AS INTEGER) = 1 OR retired_at IS NULL)
ORDER BY repo, identity;

-- name: ListSubagentsOf :many
SELECT * FROM mailboxes
WHERE kind = 'subagent'
  AND host = @host AND org = @org AND repo = @repo AND identity = @identity
ORDER BY subagent;

-- name: ListMailboxesByIDs :many
SELECT * FROM mailboxes WHERE id IN (sqlc.slice('ids')) ORDER BY id;

-- name: ListAllIdentities :many
SELECT * FROM mailboxes
WHERE kind = 'identity'
ORDER BY org, repo, identity;

-- name: ListAllMailboxes :many
SELECT * FROM mailboxes ORDER BY org, repo, identity, subagent;

-- name: UpdateIdentity :execrows
UPDATE mailboxes
SET role = @role, description = @description, reports_to = @reports_to
WHERE id = @id AND kind = 'identity';

-- name: SetRetiredAt :execrows
UPDATE mailboxes SET retired_at = sqlc.narg('retired_at')
WHERE id = @id AND kind = 'identity';

-- name: MailboxActivity :many
-- Per-mailbox received counts and the newest received message time.
SELECT d.mailbox_id,
       CAST(COALESCE(SUM(CASE WHEN d.read_at IS NULL AND d.archived_at IS NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS unread,
       COUNT(*) AS total,
       CAST(COALESCE(MAX(m.created_at), 0) AS INTEGER) AS last_received
FROM deliveries d
JOIN messages m ON m.id = d.message_id
GROUP BY d.mailbox_id;

-- name: MailboxLastSent :many
SELECT from_mailbox AS mailbox_id, CAST(MAX(created_at) AS INTEGER) AS last_sent
FROM messages
GROUP BY from_mailbox;

-- name: MailboxDailyDeliveries :many
-- Deliveries per mailbox per UTC day (days since the Unix epoch) since a time.
SELECT d.mailbox_id,
       CAST(m.created_at / 86400000 AS INTEGER) AS day,
       COUNT(*) AS n
FROM deliveries d
JOIN messages m ON m.id = d.message_id
WHERE m.created_at >= @since
GROUP BY d.mailbox_id, day;
