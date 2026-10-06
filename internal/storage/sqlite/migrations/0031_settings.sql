-- Key/value settings: update mode, highest processed update_id, ...
CREATE TABLE settings (
    name  TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
