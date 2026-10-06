-- Category prompts (#42): questions the bot sent from /categories (add a
-- Category, rename one). A reply to prompt_message_id answers it.
CREATE TABLE category_prompts (
    chat_id           INTEGER NOT NULL,
    prompt_message_id INTEGER NOT NULL,
    action            TEXT    NOT NULL,
    category_id       INTEGER NOT NULL DEFAULT 0,
    host_message_id   INTEGER NOT NULL,
    created_at        BIGINT  NOT NULL,
    PRIMARY KEY (chat_id, prompt_message_id)
);
