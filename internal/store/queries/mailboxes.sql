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
ORDER BY repo, identity;

-- name: ListIdentitiesInRepo :many
SELECT * FROM mailboxes
WHERE kind = 'identity' AND host = @host AND org = @org AND repo = @repo
ORDER BY repo, identity;

-- name: ListIdentitiesInIdentity :many
SELECT * FROM mailboxes
WHERE kind = 'identity' AND host = @host AND org = @org AND repo = @repo AND identity = @identity
ORDER BY repo, identity;

-- name: ListSubagentsOf :many
SELECT * FROM mailboxes
WHERE kind = 'subagent'
  AND host = @host AND org = @org AND repo = @repo AND identity = @identity
ORDER BY subagent;

-- name: ListMailboxesByIDs :many
SELECT * FROM mailboxes WHERE id IN (sqlc.slice('ids')) ORDER BY id;
