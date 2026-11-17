-- Rebuilds mailboxes to widen the kind CHECK (SQLite cannot alter a CHECK).
-- This file runs with the migrate driver's no-tx-wrap so foreign keys can be
-- switched off around the rebuild; the pragma is a no-op inside a transaction.
PRAGMA foreign_keys = OFF;

BEGIN;

CREATE TABLE mailboxes_new (
    id          TEXT PRIMARY KEY,
    host        TEXT NOT NULL,
    org         TEXT NOT NULL,
    repo        TEXT NOT NULL DEFAULT '',
    identity    TEXT NOT NULL DEFAULT '',
    subagent    TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL CHECK (kind IN ('namespace', 'identity', 'subagent', 'operator')),
    role        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    reports_to  TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    retired_at  INTEGER,
    UNIQUE (host, org, repo, identity, subagent)
);

INSERT INTO mailboxes_new (id, host, org, repo, identity, subagent, kind, role, description, reports_to, created_at)
SELECT id, host, org, repo, identity, subagent, kind, role, description, reports_to, created_at FROM mailboxes;

DROP TABLE mailboxes;
ALTER TABLE mailboxes_new RENAME TO mailboxes;

ALTER TABLE messages ADD COLUMN from_client TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN from_machine TEXT NOT NULL DEFAULT '';

COMMIT;

PRAGMA foreign_keys = ON;
