-- Transactions: one row per money movement (#32).
-- Times are UTC unix seconds. amount_toman is NULL only when flagged.
-- category_id is NULL for internal transfers (the categories table and its
-- seed come with #35, so no foreign key here).
CREATE TABLE transactions (
    id                 INTEGER PRIMARY KEY,
    created_at         BIGINT  NOT NULL,
    occurred_at        BIGINT  NOT NULL,
    amount_toman       BIGINT,
    direction          TEXT    NOT NULL,
    category_id        INTEGER,
    description        TEXT    NOT NULL DEFAULT '',
    bank_label         TEXT,
    raw_text           TEXT    NOT NULL,
    input_id           TEXT    NOT NULL,
    flagged            INTEGER NOT NULL DEFAULT 0,
    flag_reason        TEXT    NOT NULL DEFAULT '',
    categorize_pending INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX transactions_occurred_at ON transactions (occurred_at);
