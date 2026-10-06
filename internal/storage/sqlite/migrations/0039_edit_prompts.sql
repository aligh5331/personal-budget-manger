-- Edit prompts (#39): questions the bot sent while the Owner edits a
-- Transaction. A reply to prompt_message_id is the new value of field.
-- view and host_message_id name the message to redraw afterwards.
CREATE TABLE edit_prompts (
    chat_id           INTEGER NOT NULL,
    prompt_message_id INTEGER NOT NULL,
    transaction_id    INTEGER NOT NULL,
    field             TEXT    NOT NULL,
    view              TEXT    NOT NULL DEFAULT '',
    host_message_id   INTEGER NOT NULL,
    created_at        BIGINT  NOT NULL,
    PRIMARY KEY (chat_id, prompt_message_id)
);
