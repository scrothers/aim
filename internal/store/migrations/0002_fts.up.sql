CREATE VIRTUAL TABLE messages_fts USING fts5 (
    subject, body, content = 'messages', content_rowid = 'rowid'
);

CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
    INSERT INTO messages_fts (rowid, subject, body) VALUES (new.rowid, new.subject, new.body);
END;

CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
    INSERT INTO messages_fts (messages_fts, rowid, subject, body) VALUES ('delete', old.rowid, old.subject, old.body);
END;

CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
    INSERT INTO messages_fts (messages_fts, rowid, subject, body) VALUES ('delete', old.rowid, old.subject, old.body);
    INSERT INTO messages_fts (rowid, subject, body) VALUES (new.rowid, new.subject, new.body);
END;
