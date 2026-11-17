PRAGMA foreign_keys = OFF;

BEGIN;

-- Remove everything the operator mailbox owns, in FK order. The messages_ad
-- trigger (0002) keeps messages_fts in step with the message deletes.
DELETE FROM deliveries WHERE message_id IN (
    SELECT id FROM messages WHERE from_mailbox IN (SELECT id FROM mailboxes WHERE kind = 'operator')
);
DELETE FROM deliveries WHERE mailbox_id IN (SELECT id FROM mailboxes WHERE kind = 'operator');
UPDATE messages SET in_reply_to = NULL WHERE in_reply_to IN (
    SELECT id FROM messages WHERE from_mailbox IN (SELECT id FROM mailboxes WHERE kind = 'operator')
);
DELETE FROM messages WHERE from_mailbox IN (SELECT id FROM mailboxes WHERE kind = 'operator');
DELETE FROM mailboxes WHERE kind = 'operator';

CREATE TABLE mailboxes_old (
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

INSERT INTO mailboxes_old (id, host, org, repo, identity, subagent, kind, role, description, reports_to, created_at)
SELECT id, host, org, repo, identity, subagent, kind, role, description, reports_to, created_at FROM mailboxes;

DROP TABLE mailboxes;
ALTER TABLE mailboxes_old RENAME TO mailboxes;

ALTER TABLE messages DROP COLUMN from_client;
ALTER TABLE messages DROP COLUMN from_machine;

COMMIT;

PRAGMA foreign_keys = ON;
