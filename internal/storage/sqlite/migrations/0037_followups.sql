-- Pending Follow-ups (#37). prompt_message_id is 0 while queued behind
-- another Follow-up of the same batch Input. transaction_id is set when the
-- question is about an existing Flagged transaction; otherwise the d_*
-- columns hold the Transaction that has not been saved yet.
CREATE TABLE followups (
    id                INTEGER PRIMARY KEY,
    chat_id           INTEGER NOT NULL,
    prompt_message_id INTEGER NOT NULL DEFAULT 0,
    field             TEXT    NOT NULL,
    expires_at        BIGINT  NOT NULL DEFAULT 0,
    transaction_id    INTEGER NOT NULL DEFAULT 0,
    has_time          INTEGER NOT NULL DEFAULT 0,
    direction_unclear INTEGER NOT NULL DEFAULT 0,
    created_at        BIGINT  NOT NULL,
    d_occurred_at     BIGINT  NOT NULL DEFAULT 0,
    d_amount_toman    BIGINT,
    d_direction       TEXT    NOT NULL DEFAULT 'out',
    d_description     TEXT    NOT NULL DEFAULT '',
    d_bank_label      TEXT,
    d_raw_text        TEXT    NOT NULL DEFAULT '',
    d_input_id        TEXT    NOT NULL DEFAULT '',
    d_flag_reason     TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX followups_prompt ON followups (chat_id, prompt_message_id);
CREATE INDEX followups_transaction ON followups (transaction_id);
