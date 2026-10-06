-- Duplicates held back for [Save anyway] (#34). payload is the Transaction
-- as JSON; the row is deleted when the Owner taps the button.
CREATE TABLE held_duplicates (
    id         INTEGER PRIMARY KEY,
    created_at BIGINT NOT NULL,
    payload    TEXT   NOT NULL
);
