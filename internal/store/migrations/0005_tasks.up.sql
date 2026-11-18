-- Runs with the migrate driver's no-tx-wrap, so the transaction is explicit.
BEGIN;

-- tasks are global work items. People are stored as canonical identity
-- addresses (or "operator"); empty means unset. Times are unix milliseconds.
CREATE TABLE tasks (
    id               TEXT PRIMARY KEY,
    title            TEXT NOT NULL,
    body             TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'not_started'
                     CHECK (status IN ('not_started', 'claimed', 'started', 'finished', 'abandoned')),
    priority         TEXT NOT NULL DEFAULT 'normal'
                     CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    scope            TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL,
    assignee         TEXT NOT NULL DEFAULT '',
    claimant         TEXT NOT NULL DEFAULT '',
    claim_session    TEXT NOT NULL DEFAULT '',
    -- stale_notified is 1 once the current claim's stale notice was sent.
    stale_notified   INTEGER NOT NULL DEFAULT 0,
    due_at           INTEGER,
    claimed_at       INTEGER,
    last_activity_at INTEGER,
    finished_at      INTEGER,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
);

CREATE INDEX tasks_status_idx ON tasks (status, updated_at);
CREATE INDEX tasks_assignee_idx ON tasks (assignee) WHERE assignee <> '';
CREATE INDEX tasks_claimant_idx ON tasks (claimant) WHERE claimant <> '';
CREATE INDEX tasks_claim_session_idx ON tasks (claim_session) WHERE claim_session <> '';
CREATE INDEX tasks_scope_idx ON tasks (scope) WHERE scope <> '';

-- task_links ties messages (and so their threads) to tasks.
CREATE TABLE task_links (
    task_id    TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    message_id TEXT NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    linked_by  TEXT NOT NULL,
    linked_at  INTEGER NOT NULL,
    PRIMARY KEY (task_id, message_id)
);

CREATE INDEX task_links_message_idx ON task_links (message_id);

-- task_events is a task's full history, oldest first by id.
CREATE TABLE task_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    at          INTEGER NOT NULL,
    actor       TEXT NOT NULL,
    kind        TEXT NOT NULL,
    from_status TEXT NOT NULL DEFAULT '',
    to_status   TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT ''
);

CREATE INDEX task_events_task_idx ON task_events (task_id, id);

COMMIT;
