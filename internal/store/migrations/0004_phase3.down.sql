BEGIN;

DROP TABLE thread_mutes;
DROP TABLE settings;
DROP INDEX session_signons_updated_idx;
DROP TABLE session_signons;

COMMIT;
