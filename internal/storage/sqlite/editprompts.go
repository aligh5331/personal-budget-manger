package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

var _ storage.EditPrompts = (*Store)(nil)

// SaveEditPrompt implements storage.EditPrompts.
func (s *Store) SaveEditPrompt(ctx context.Context, p storage.EditPrompt) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO edit_prompts (
		chat_id, prompt_message_id, transaction_id, field, view, host_message_id, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (chat_id, prompt_message_id) DO UPDATE SET
		transaction_id = excluded.transaction_id, field = excluded.field, view = excluded.view,
		host_message_id = excluded.host_message_id, created_at = excluded.created_at`,
		p.ChatID, p.PromptMessageID, p.TransactionID, p.Field, p.View, p.HostMessageID, p.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("save edit prompt: %w", err)
	}
	return nil
}

// EditPromptByMessage implements storage.EditPrompts.
func (s *Store) EditPromptByMessage(ctx context.Context, chatID, messageID int64) (storage.EditPrompt, bool, error) {
	p := storage.EditPrompt{ChatID: chatID, PromptMessageID: messageID}
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT transaction_id, field, view, host_message_id, created_at
		FROM edit_prompts WHERE chat_id = ? AND prompt_message_id = ?`, chatID, messageID,
	).Scan(&p.TransactionID, &p.Field, &p.View, &p.HostMessageID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.EditPrompt{}, false, nil
	}
	if err != nil {
		return storage.EditPrompt{}, false, fmt.Errorf("read edit prompt: %w", err)
	}
	p.CreatedAt = time.Unix(created, 0).UTC()
	return p, true, nil
}
