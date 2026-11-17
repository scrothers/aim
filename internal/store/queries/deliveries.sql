-- name: InsertDelivery :exec
INSERT INTO deliveries (message_id, mailbox_id, kind) VALUES (@message_id, @mailbox_id, @kind);

-- name: ListDeliveriesForMessage :many
SELECT d.message_id, d.mailbox_id, d.kind, d.read_at, d.archived_at,
       mb.host, mb.org, mb.repo, mb.identity, mb.subagent
FROM deliveries d
JOIN mailboxes mb ON mb.id = d.mailbox_id
WHERE d.message_id = @message_id
ORDER BY d.kind DESC, mb.host, mb.org, mb.repo, mb.identity, mb.subagent;

-- name: ListInboxOrg :many
SELECT d.message_id, d.mailbox_id, d.kind, d.read_at, d.archived_at,
       m.thread_id, m.in_reply_to, m.from_mailbox, m.from_session, m.subject, m.created_at,
       mb.host, mb.org, mb.repo, mb.identity, mb.subagent
FROM deliveries d
JOIN messages m ON m.id = d.message_id
JOIN mailboxes mb ON mb.id = d.mailbox_id
WHERE mb.host = @host AND mb.org = @org
  AND (CAST(sqlc.narg('from_mailbox') AS TEXT) IS NULL OR m.from_mailbox = sqlc.narg('from_mailbox'))
  AND (CAST(sqlc.narg('since') AS INTEGER) IS NULL OR m.created_at >= sqlc.narg('since'))
  AND (CAST(sqlc.narg('until') AS INTEGER) IS NULL OR m.created_at <= sqlc.narg('until'))
  AND (CAST(sqlc.narg('before_message') AS TEXT) IS NULL
       OR m.id < sqlc.narg('before_message')
       OR (m.id = sqlc.narg('before_message') AND d.mailbox_id < sqlc.narg('before_mailbox')))
  AND CASE CAST(@state AS TEXT)
        WHEN 'unread' THEN d.read_at IS NULL AND d.archived_at IS NULL
        WHEN 'read' THEN d.read_at IS NOT NULL AND d.archived_at IS NULL
        WHEN 'inbox' THEN d.archived_at IS NULL
        WHEN 'archived' THEN d.archived_at IS NOT NULL
        ELSE 1
      END
ORDER BY m.id DESC, d.mailbox_id DESC
LIMIT @page_limit;

-- name: ListInboxRepo :many
SELECT d.message_id, d.mailbox_id, d.kind, d.read_at, d.archived_at,
       m.thread_id, m.in_reply_to, m.from_mailbox, m.from_session, m.subject, m.created_at,
       mb.host, mb.org, mb.repo, mb.identity, mb.subagent
FROM deliveries d
JOIN messages m ON m.id = d.message_id
JOIN mailboxes mb ON mb.id = d.mailbox_id
WHERE mb.host = @host AND mb.org = @org AND mb.repo = @repo
  AND (CAST(sqlc.narg('from_mailbox') AS TEXT) IS NULL OR m.from_mailbox = sqlc.narg('from_mailbox'))
  AND (CAST(sqlc.narg('since') AS INTEGER) IS NULL OR m.created_at >= sqlc.narg('since'))
  AND (CAST(sqlc.narg('until') AS INTEGER) IS NULL OR m.created_at <= sqlc.narg('until'))
  AND (CAST(sqlc.narg('before_message') AS TEXT) IS NULL
       OR m.id < sqlc.narg('before_message')
       OR (m.id = sqlc.narg('before_message') AND d.mailbox_id < sqlc.narg('before_mailbox')))
  AND CASE CAST(@state AS TEXT)
        WHEN 'unread' THEN d.read_at IS NULL AND d.archived_at IS NULL
        WHEN 'read' THEN d.read_at IS NOT NULL AND d.archived_at IS NULL
        WHEN 'inbox' THEN d.archived_at IS NULL
        WHEN 'archived' THEN d.archived_at IS NOT NULL
        ELSE 1
      END
ORDER BY m.id DESC, d.mailbox_id DESC
LIMIT @page_limit;

-- name: ListInboxIdentity :many
SELECT d.message_id, d.mailbox_id, d.kind, d.read_at, d.archived_at,
       m.thread_id, m.in_reply_to, m.from_mailbox, m.from_session, m.subject, m.created_at,
       mb.host, mb.org, mb.repo, mb.identity, mb.subagent
FROM deliveries d
JOIN messages m ON m.id = d.message_id
JOIN mailboxes mb ON mb.id = d.mailbox_id
WHERE mb.host = @host AND mb.org = @org AND mb.repo = @repo AND mb.identity = @identity
  AND (CAST(sqlc.narg('from_mailbox') AS TEXT) IS NULL OR m.from_mailbox = sqlc.narg('from_mailbox'))
  AND (CAST(sqlc.narg('since') AS INTEGER) IS NULL OR m.created_at >= sqlc.narg('since'))
  AND (CAST(sqlc.narg('until') AS INTEGER) IS NULL OR m.created_at <= sqlc.narg('until'))
  AND (CAST(sqlc.narg('before_message') AS TEXT) IS NULL
       OR m.id < sqlc.narg('before_message')
       OR (m.id = sqlc.narg('before_message') AND d.mailbox_id < sqlc.narg('before_mailbox')))
  AND CASE CAST(@state AS TEXT)
        WHEN 'unread' THEN d.read_at IS NULL AND d.archived_at IS NULL
        WHEN 'read' THEN d.read_at IS NOT NULL AND d.archived_at IS NULL
        WHEN 'inbox' THEN d.archived_at IS NULL
        WHEN 'archived' THEN d.archived_at IS NOT NULL
        ELSE 1
      END
ORDER BY m.id DESC, d.mailbox_id DESC
LIMIT @page_limit;

-- name: ListInboxSubagent :many
SELECT d.message_id, d.mailbox_id, d.kind, d.read_at, d.archived_at,
       m.thread_id, m.in_reply_to, m.from_mailbox, m.from_session, m.subject, m.created_at,
       mb.host, mb.org, mb.repo, mb.identity, mb.subagent
FROM deliveries d
JOIN messages m ON m.id = d.message_id
JOIN mailboxes mb ON mb.id = d.mailbox_id
WHERE mb.host = @host AND mb.org = @org AND mb.repo = @repo AND mb.identity = @identity
  AND mb.subagent = @subagent
  AND (CAST(sqlc.narg('from_mailbox') AS TEXT) IS NULL OR m.from_mailbox = sqlc.narg('from_mailbox'))
  AND (CAST(sqlc.narg('since') AS INTEGER) IS NULL OR m.created_at >= sqlc.narg('since'))
  AND (CAST(sqlc.narg('until') AS INTEGER) IS NULL OR m.created_at <= sqlc.narg('until'))
  AND (CAST(sqlc.narg('before_message') AS TEXT) IS NULL
       OR m.id < sqlc.narg('before_message')
       OR (m.id = sqlc.narg('before_message') AND d.mailbox_id < sqlc.narg('before_mailbox')))
  AND CASE CAST(@state AS TEXT)
        WHEN 'unread' THEN d.read_at IS NULL AND d.archived_at IS NULL
        WHEN 'read' THEN d.read_at IS NOT NULL AND d.archived_at IS NULL
        WHEN 'inbox' THEN d.archived_at IS NULL
        WHEN 'archived' THEN d.archived_at IS NOT NULL
        ELSE 1
      END
ORDER BY m.id DESC, d.mailbox_id DESC
LIMIT @page_limit;

-- name: SetReadAt :execrows
UPDATE deliveries SET read_at = @now
WHERE message_id = @message_id AND mailbox_id = @mailbox_id;

-- name: ClearReadAt :execrows
UPDATE deliveries SET read_at = NULL
WHERE message_id = @message_id AND mailbox_id = @mailbox_id;

-- name: SetArchivedAt :execrows
UPDATE deliveries SET archived_at = @now
WHERE message_id = @message_id AND mailbox_id = @mailbox_id;

-- name: ClearArchivedAt :execrows
UPDATE deliveries SET archived_at = NULL
WHERE message_id = @message_id AND mailbox_id = @mailbox_id;

-- name: ListInboxAll :many
SELECT d.message_id, d.mailbox_id, d.kind, d.read_at, d.archived_at,
       m.thread_id, m.in_reply_to, m.from_mailbox, m.from_session, m.subject, m.created_at,
       mb.host, mb.org, mb.repo, mb.identity, mb.subagent
FROM deliveries d
JOIN messages m ON m.id = d.message_id
JOIN mailboxes mb ON mb.id = d.mailbox_id
WHERE (CAST(sqlc.narg('from_mailbox') AS TEXT) IS NULL OR m.from_mailbox = sqlc.narg('from_mailbox'))
  AND (CAST(sqlc.narg('since') AS INTEGER) IS NULL OR m.created_at >= sqlc.narg('since'))
  AND (CAST(sqlc.narg('until') AS INTEGER) IS NULL OR m.created_at <= sqlc.narg('until'))
  AND (CAST(sqlc.narg('before_message') AS TEXT) IS NULL
       OR m.id < sqlc.narg('before_message')
       OR (m.id = sqlc.narg('before_message') AND d.mailbox_id < sqlc.narg('before_mailbox')))
  AND CASE CAST(@state AS TEXT)
        WHEN 'unread' THEN d.read_at IS NULL AND d.archived_at IS NULL
        WHEN 'read' THEN d.read_at IS NOT NULL AND d.archived_at IS NULL
        WHEN 'inbox' THEN d.archived_at IS NULL
        WHEN 'archived' THEN d.archived_at IS NOT NULL
        ELSE 1
      END
ORDER BY m.id DESC, d.mailbox_id DESC
LIMIT @page_limit;
