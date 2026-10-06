package sqlite

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

var _ storage.CategorizeRetries = (*Store)(nil)

// QueueRetry implements storage.CategorizeRetries.
func (s *Store) QueueRetry(ctx context.Context, r storage.Retry) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO categorize_retries (transaction_id, chat_id, message_id, listed, tries, next_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (transaction_id) DO UPDATE SET chat_id = excluded.chat_id,
			message_id = excluded.message_id, listed = excluded.listed, tries = excluded.tries, next_at = excluded.next_at`,
		r.TransactionID, r.ChatID, r.MessageID, joinIDs(r.Listed), r.Tries, r.NextAt.Unix())
	if err != nil {
		return fmt.Errorf("queue retry of transaction %d: %w", r.TransactionID, err)
	}
	return nil
}

// DueRetries implements storage.CategorizeRetries.
func (s *Store) DueRetries(ctx context.Context, now time.Time) ([]storage.Retry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT transaction_id, chat_id, message_id, listed, tries, next_at
		FROM categorize_retries WHERE next_at <= ? ORDER BY transaction_id`, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("list due retries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []storage.Retry
	for rows.Next() {
		var r storage.Retry
		var next int64
		var listed string
		if err := rows.Scan(&r.TransactionID, &r.ChatID, &r.MessageID, &listed, &r.Tries, &next); err != nil {
			return nil, fmt.Errorf("list due retries: %w", err)
		}
		r.NextAt = time.Unix(next, 0)
		r.Listed = splitIDs(listed)
		out = append(out, r)
	}
	return out, rows.Err()
}

// RescheduleRetry implements storage.CategorizeRetries.
func (s *Store) RescheduleRetry(ctx context.Context, id int64, tries int, nextAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE categorize_retries SET tries = ?, next_at = ? WHERE transaction_id = ?`,
		tries, nextAt.Unix(), id)
	if err != nil {
		return fmt.Errorf("reschedule retry of transaction %d: %w", id, err)
	}
	return nil
}

// FinishRetry implements storage.CategorizeRetries.
func (s *Store) FinishRetry(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE transactions SET categorize_pending = 0 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("clear pending of transaction %d: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM categorize_retries WHERE transaction_id = ?`, id); err != nil {
		return fmt.Errorf("finish retry of transaction %d: %w", id, err)
	}
	return tx.Commit()
}

// ApplyBackgroundCategory implements storage.CategorizeRetries.
func (s *Store) ApplyBackgroundCategory(ctx context.Context, id, categoryID int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE transactions SET category_id = ?, categorize_pending = 0
		WHERE id = ? AND categorize_pending = 1
		AND category_id = (SELECT id FROM categories WHERE builtin = 1 ORDER BY id LIMIT 1)`, categoryID, id)
	if err != nil {
		return false, fmt.Errorf("apply category to transaction %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM categorize_retries WHERE transaction_id = ?`, id); err != nil {
		return false, fmt.Errorf("finish retry of transaction %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

func splitIDs(s string) []int64 {
	var ids []int64
	for _, p := range strings.Split(s, ",") {
		if id, err := strconv.ParseInt(p, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
