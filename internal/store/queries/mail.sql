-- name: GetDelivery :one
SELECT message_id, mailbox_id, kind, read_at, archived_at
FROM deliveries
WHERE message_id = @message_id AND mailbox_id = @mailbox_id;
