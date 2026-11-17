-- name: UpsertSessionSignon :exec
INSERT INTO session_signons (session_id, identity, away, client, machine, updated_at)
VALUES (@session_id, @identity, @away, @client, @machine, @updated_at)
ON CONFLICT (session_id) DO UPDATE SET
    identity = excluded.identity,
    away = excluded.away,
    client = excluded.client,
    machine = excluded.machine,
    updated_at = excluded.updated_at;

-- name: GetSessionSignon :one
SELECT session_id, identity, away, client, machine, updated_at
FROM session_signons
WHERE session_id = @session_id AND updated_at >= @min_updated_at;

-- name: DeleteSessionSignon :exec
DELETE FROM session_signons WHERE session_id = @session_id;

-- name: PruneSessionSignons :execrows
DELETE FROM session_signons WHERE updated_at < @before;

-- name: GetSetting :one
SELECT key, value, updated_at FROM settings WHERE key = @key;

-- name: ListSettings :many
SELECT key, value, updated_at FROM settings ORDER BY key;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value, updated_at) VALUES (@key, @value, @updated_at)
ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = @key;

-- name: MuteThread :exec
INSERT OR IGNORE INTO thread_mutes (thread_id, muted_at) VALUES (@thread_id, @muted_at);

-- name: UnmuteThread :exec
DELETE FROM thread_mutes WHERE thread_id = @thread_id;

-- name: ListMutedThreads :many
SELECT thread_id, muted_at FROM thread_mutes ORDER BY muted_at DESC, thread_id;

-- name: IsThreadMuted :one
SELECT EXISTS (SELECT 1 FROM thread_mutes WHERE thread_id = @thread_id);

-- name: ListPrunableMessages :many
SELECT m.id FROM messages m
WHERE m.created_at < @before_ms
  AND EXISTS (SELECT 1 FROM deliveries d WHERE d.message_id = m.id)
  AND NOT EXISTS (SELECT 1 FROM deliveries d WHERE d.message_id = m.id AND d.archived_at IS NULL)
ORDER BY m.created_at, m.id
LIMIT @row_limit;

-- name: DeleteMessageDeliveries :exec
DELETE FROM deliveries WHERE message_id = @message_id;

-- name: DeleteMessage :exec
DELETE FROM messages WHERE id = @id;

-- name: NullDanglingReplies :execrows
UPDATE messages SET in_reply_to = NULL
WHERE in_reply_to IS NOT NULL AND NOT EXISTS (SELECT 1 FROM messages p WHERE p.id = messages.in_reply_to);

-- name: NullRepliesTo :execrows
UPDATE messages SET in_reply_to = NULL WHERE in_reply_to = @message_id;
