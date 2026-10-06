-- Per-message text for the Categorizer (#30): one bank message plus the
-- shared note. Empty on older rows and on unread Inputs (raw_text is used).
ALTER TABLE transactions ADD COLUMN categorizer_text TEXT NOT NULL DEFAULT '';
ALTER TABLE followups ADD COLUMN d_categorizer_text TEXT NOT NULL DEFAULT '';
