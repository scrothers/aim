-- name: GetDelivery :one
SELECT message_id, mailbox_id, kind, read_at, archived_at
FROM deliveries
WHERE message_id = @message_id AND mailbox_id = @mailbox_id;

-- name: CountUnreadDeliveries :one
SELECT COUNT(*) FROM deliveries WHERE read_at IS NULL AND archived_at IS NULL;

-- name: CountMessagesSince :one
SELECT COUNT(*) FROM messages WHERE created_at >= @since;

-- name: CountDeliveriesSince :one
SELECT COUNT(*) FROM deliveries d JOIN messages m ON m.id = d.message_id WHERE m.created_at >= @since;

-- name: CountMessages :one
SELECT COUNT(*) FROM messages;
