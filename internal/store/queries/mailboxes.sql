-- name: GetMailboxByKey :one
SELECT * FROM mailboxes
WHERE host = @host AND org = @org AND repo = @repo AND identity = @identity AND subagent = @subagent;

-- name: GetMailboxByID :one
SELECT * FROM mailboxes WHERE id = @id;

-- name: InsertMailbox :exec
INSERT INTO mailboxes (id, host, org, repo, identity, subagent, kind, role, description, reports_to, created_at)
VALUES (@id, @host, @org, @repo, @identity, @subagent, @kind, @role, @description, @reports_to, @created_at);

-- name: ListIdentitiesInScope :many
SELECT mb.* FROM mailboxes mb
WHERE mb.kind = 'identity'
  AND mb.host = @host AND mb.org = @org
  AND (CAST(@match_repo AS INTEGER) = 0 OR mb.repo = @repo)
  AND (CAST(@match_identity AS INTEGER) = 0 OR mb.identity = @identity)
ORDER BY mb.repo, mb.identity;

-- name: ListSubagentsOf :many
SELECT * FROM mailboxes
WHERE kind = 'subagent'
  AND host = @host AND org = @org AND repo = @repo AND identity = @identity
ORDER BY subagent;

-- name: ListMailboxesByIDs :many
SELECT * FROM mailboxes WHERE id IN (sqlc.slice('ids')) ORDER BY id;
