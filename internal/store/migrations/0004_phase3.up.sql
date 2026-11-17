-- Runs with the migrate driver's no-tx-wrap, so the transaction is explicit.
BEGIN;

CREATE TABLE session_signons (
    session_id TEXT PRIMARY KEY,
    identity   TEXT NOT NULL,
    away       TEXT NOT NULL DEFAULT '',
    client     TEXT NOT NULL DEFAULT '',
    machine    TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

CREATE INDEX session_signons_updated_idx ON session_signons (updated_at);

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE thread_mutes (
    thread_id TEXT PRIMARY KEY,
    muted_at  INTEGER NOT NULL
);

COMMIT;
