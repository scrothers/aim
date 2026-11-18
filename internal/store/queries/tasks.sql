-- name: InsertTask :exec
INSERT INTO tasks (id, title, body, status, priority, scope, created_by, assignee, due_at, created_at, updated_at)
VALUES (@id, @title, @body, @status, @priority, @scope, @created_by, @assignee, @due_at, @created_at, @updated_at);

-- name: GetTask :one
SELECT * FROM tasks WHERE id = @id;

-- name: UpdateTask :exec
-- UpdateTask writes every mutable column; callers load, change and save the
-- row inside one transaction.
UPDATE tasks SET
    title = @title,
    body = @body,
    status = @status,
    priority = @priority,
    scope = @scope,
    assignee = @assignee,
    claimant = @claimant,
    claim_session = @claim_session,
    stale_notified = @stale_notified,
    due_at = @due_at,
    claimed_at = @claimed_at,
    last_activity_at = @last_activity_at,
    finished_at = @finished_at,
    updated_at = @updated_at
WHERE id = @id;

-- name: ListTasks :many
-- ListTasks filters by optional fields (empty matches all). scope matches the
-- address itself and everything below it. Ordered newest-updated first and
-- paged by (updated_at, id).
SELECT * FROM tasks
WHERE (status = sqlc.arg(status) OR sqlc.arg(status) = '')
  AND (assignee = sqlc.arg(assignee) OR sqlc.arg(assignee) = '')
  AND (claimant = sqlc.arg(claimant) OR sqlc.arg(claimant) = '')
  AND (created_by = sqlc.arg(created_by) OR sqlc.arg(created_by) = '')
  AND (assignee = sqlc.arg(involves) OR claimant = sqlc.arg(involves) OR created_by = sqlc.arg(involves) OR sqlc.arg(involves) = '')
  AND (scope = sqlc.arg(scope) OR scope LIKE sqlc.arg(scope_prefix) OR sqlc.arg(scope) = '')
  AND (CAST(sqlc.arg(open_only) AS INTEGER) = 0 OR status <> 'finished')
  AND (CAST(sqlc.arg(before_updated) AS INTEGER) = 0 OR updated_at < sqlc.arg(before_updated)
       OR (updated_at = sqlc.arg(before_updated) AND id < sqlc.arg(before_id)))
ORDER BY updated_at DESC, id DESC
LIMIT @limit;

-- name: TouchTasksBySession :execrows
-- TouchTasksBySession records activity on every task the session holds,
-- resetting its lease decay.
UPDATE tasks SET last_activity_at = @now, stale_notified = 0
WHERE claim_session = @claim_session AND status IN ('claimed', 'started');

-- name: ListHeldTasks :many
-- ListHeldTasks returns claimed or started tasks whose last activity is
-- before @before, oldest activity first, for the decay sweeper.
SELECT * FROM tasks
WHERE status IN ('claimed', 'started') AND last_activity_at < @before
ORDER BY last_activity_at, id;

-- name: CountTasksForIdentity :one
-- CountTasksForIdentity counts an identity's open work for hook nudges.
SELECT
    CAST(COALESCE(SUM(CASE WHEN claimant = @identity AND status IN ('claimed', 'started') THEN 1 ELSE 0 END), 0) AS INTEGER) AS held,
    CAST(COALESCE(SUM(CASE WHEN assignee = @identity AND status IN ('not_started', 'abandoned') THEN 1 ELSE 0 END), 0) AS INTEGER) AS assigned
FROM tasks;

-- name: InsertTaskEvent :exec
INSERT INTO task_events (task_id, at, actor, kind, from_status, to_status, note)
VALUES (@task_id, @at, @actor, @kind, @from_status, @to_status, @note);

-- name: ListTaskEvents :many
SELECT * FROM task_events WHERE task_id = @task_id ORDER BY id;

-- name: InsertTaskLink :exec
INSERT OR IGNORE INTO task_links (task_id, message_id, linked_by, linked_at)
VALUES (@task_id, @message_id, @linked_by, @linked_at);

-- name: ListTaskLinks :many
-- ListTaskLinks returns a task's linked messages with their thread and
-- subject, oldest link first.
SELECT l.message_id, m.thread_id, m.subject, l.linked_by, l.linked_at
FROM task_links l
JOIN messages m ON m.id = l.message_id
WHERE l.task_id = @task_id
ORDER BY l.linked_at, l.message_id;

-- name: ListTasksForThreads :many
-- ListTasksForThreads returns the tasks linked to any message in the given
-- threads, for message chips.
SELECT DISTINCT m.thread_id, t.id, t.title, t.status
FROM task_links l
JOIN messages m ON m.id = l.message_id
JOIN tasks t ON t.id = l.task_id
WHERE m.thread_id IN (sqlc.slice('thread_ids'))
ORDER BY m.thread_id, t.id;
