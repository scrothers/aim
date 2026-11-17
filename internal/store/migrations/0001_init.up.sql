CREATE TABLE mailboxes (
    id          TEXT PRIMARY KEY,
    host        TEXT NOT NULL,
    org         TEXT NOT NULL,
    repo        TEXT NOT NULL DEFAULT '',
    identity    TEXT NOT NULL DEFAULT '',
    subagent    TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL CHECK (kind IN ('namespace', 'identity', 'subagent')),
    role        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    reports_to  TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    UNIQUE (host, org, repo, identity, subagent)
);

CREATE TABLE messages (
    rowid        INTEGER PRIMARY KEY,
    id           TEXT NOT NULL UNIQUE,
    thread_id    TEXT NOT NULL,
    in_reply_to  TEXT REFERENCES messages (id),
    from_mailbox TEXT NOT NULL REFERENCES mailboxes (id),
    from_session TEXT NOT NULL DEFAULT '',
    subject      TEXT NOT NULL,
    body         TEXT NOT NULL,
    created_at   INTEGER NOT NULL
);

CREATE INDEX messages_thread_idx ON messages (thread_id);

CREATE TABLE deliveries (
    message_id  TEXT NOT NULL REFERENCES messages (id),
    mailbox_id  TEXT NOT NULL REFERENCES mailboxes (id),
    kind        TEXT NOT NULL CHECK (kind IN ('to', 'cc')),
    read_at     INTEGER,
    archived_at INTEGER,
    PRIMARY KEY (message_id, mailbox_id)
);

CREATE INDEX deliveries_mailbox_idx ON deliveries (mailbox_id, message_id);
