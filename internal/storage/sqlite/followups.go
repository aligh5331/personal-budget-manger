package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

var _ storage.FollowUps = (*Store)(nil)

const followUpColumns = `id, chat_id, prompt_message_id, field, expires_at, transaction_id, has_time,
	direction_unclear, created_at, d_occurred_at, d_amount_toman, d_direction, d_description,
	d_bank_label, d_raw_text, d_input_id, d_flag_reason`

// SaveFollowUp implements storage.FollowUps.
func (s *Store) SaveFollowUp(ctx context.Context, f storage.FollowUp) (int64, error) {
	var bankLabel sql.NullString
	if f.Draft.BankLabel != "" {
		bankLabel = sql.NullString{String: f.Draft.BankLabel, Valid: true}
	}
	var expires int64
	if !f.ExpiresAt.IsZero() {
		expires = f.ExpiresAt.Unix()
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO followups (
		chat_id, prompt_message_id, field, expires_at, transaction_id, has_time, direction_unclear, created_at,
		d_occurred_at, d_amount_toman, d_direction, d_description, d_bank_label, d_raw_text, d_input_id, d_flag_reason
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		f.ChatID, f.PromptMessageID, f.Field, expires, f.TransactionID, boolInt(f.HasTime), boolInt(f.DirectionUnclear), f.CreatedAt.Unix(),
		f.Draft.OccurredAt.Unix(), f.Draft.AmountToman, f.Draft.Direction, f.Draft.Description, bankLabel,
		f.Draft.RawText, f.Draft.InputID, f.Draft.FlagReason,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save follow-up: %w", err)
	}
	return id, nil
}

func (s *Store) oneFollowUp(ctx context.Context, where string, args ...any) (storage.FollowUp, bool, error) {
	f, err := scanFollowUp(s.db.QueryRowContext(ctx, `SELECT `+followUpColumns+` FROM followups WHERE `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return storage.FollowUp{}, false, nil
	}
	if err != nil {
		return storage.FollowUp{}, false, fmt.Errorf("read follow-up: %w", err)
	}
	return f, true, nil
}

func (s *Store) manyFollowUps(ctx context.Context, query string, args ...any) ([]storage.FollowUp, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list follow-ups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []storage.FollowUp
	for rows.Next() {
		f, err := scanFollowUp(rows)
		if err != nil {
			return nil, fmt.Errorf("list follow-ups: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FollowUpByID implements storage.FollowUps.
func (s *Store) FollowUpByID(ctx context.Context, id int64) (storage.FollowUp, bool, error) {
	return s.oneFollowUp(ctx, `id = ?`, id)
}

// FollowUpByPrompt implements storage.FollowUps.
func (s *Store) FollowUpByPrompt(ctx context.Context, chatID, messageID int64) (storage.FollowUp, bool, error) {
	if messageID == 0 {
		return storage.FollowUp{}, false, nil
	}
	return s.oneFollowUp(ctx, `chat_id = ? AND prompt_message_id = ?`, chatID, messageID)
}

// FollowUpForTransaction implements storage.FollowUps.
func (s *Store) FollowUpForTransaction(ctx context.Context, transactionID int64) (storage.FollowUp, bool, error) {
	return s.oneFollowUp(ctx, `transaction_id = ? AND transaction_id <> 0`, transactionID)
}

// MarkFollowUpAsked implements storage.FollowUps.
func (s *Store) MarkFollowUpAsked(ctx context.Context, id, promptMessageID int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE followups SET prompt_message_id = ?, expires_at = ? WHERE id = ?`,
		promptMessageID, expiresAt.Unix(), id)
	if err != nil {
		return fmt.Errorf("mark follow-up asked: %w", err)
	}
	return nil
}

// CloseFollowUp implements storage.FollowUps.
func (s *Store) CloseFollowUp(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM followups WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("close follow-up %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// OpenFollowUps implements storage.FollowUps.
func (s *Store) OpenFollowUps(ctx context.Context, chatID int64) ([]storage.FollowUp, error) {
	return s.manyFollowUps(ctx, `SELECT `+followUpColumns+` FROM followups WHERE chat_id = ? ORDER BY id`, chatID)
}

// ExpiredFollowUps implements storage.FollowUps.
func (s *Store) ExpiredFollowUps(ctx context.Context, now time.Time) ([]storage.FollowUp, error) {
	return s.manyFollowUps(ctx, `SELECT `+followUpColumns+` FROM followups
		WHERE prompt_message_id <> 0 AND expires_at <= ? ORDER BY id`, now.Unix())
}

// NextQueuedFollowUps implements storage.FollowUps.
func (s *Store) NextQueuedFollowUps(ctx context.Context) ([]storage.FollowUp, error) {
	return s.manyFollowUps(ctx, `SELECT `+followUpColumns+` FROM followups q
		WHERE q.prompt_message_id = 0
		AND q.id = (SELECT MIN(id) FROM followups WHERE chat_id = q.chat_id AND prompt_message_id = 0)
		AND NOT EXISTS (SELECT 1 FROM followups a WHERE a.chat_id = q.chat_id AND a.prompt_message_id <> 0 AND a.transaction_id = 0)
		ORDER BY q.id`)
}

func scanFollowUp(row interface{ Scan(...any) error }) (storage.FollowUp, error) {
	var (
		f                          storage.FollowUp
		expires, created, occurred int64
		amount                     sql.NullInt64
		bankLabel                  sql.NullString
		hasTime, unclear           int
	)
	if err := row.Scan(&f.ID, &f.ChatID, &f.PromptMessageID, &f.Field, &expires, &f.TransactionID, &hasTime,
		&unclear, &created, &occurred, &amount, &f.Draft.Direction, &f.Draft.Description,
		&bankLabel, &f.Draft.RawText, &f.Draft.InputID, &f.Draft.FlagReason); err != nil {
		return storage.FollowUp{}, err
	}
	if expires != 0 {
		f.ExpiresAt = time.Unix(expires, 0).UTC()
	}
	f.CreatedAt = time.Unix(created, 0).UTC()
	f.Draft.OccurredAt = time.Unix(occurred, 0).UTC()
	f.Draft.CreatedAt = f.CreatedAt
	if amount.Valid {
		f.Draft.AmountToman = &amount.Int64
	}
	f.Draft.BankLabel = bankLabel.String
	f.Draft.Flagged = true
	f.HasTime = hasTime != 0
	f.DirectionUnclear = unclear != 0
	return f, nil
}
