-- Background re-categorize queue (#36): Transactions saved Uncategorized
-- during a Jev outage, retried every 10 minutes. message_id is the
-- confirmation to edit on success.
CREATE TABLE categorize_retries (
    transaction_id INTEGER PRIMARY KEY REFERENCES transactions(id) ON DELETE CASCADE,
    chat_id        INTEGER NOT NULL,
    message_id     INTEGER NOT NULL,
    listed         TEXT    NOT NULL DEFAULT '',
    tries          INTEGER NOT NULL DEFAULT 0,
    next_at        BIGINT  NOT NULL
);
