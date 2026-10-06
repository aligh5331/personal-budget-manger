package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// TransactionExists implements storage.Transactions.
func (s *Store) TransactionExists(ctx context.Context, amountToman int64, direction storage.Direction, from, to time.Time) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions
		WHERE amount_toman = ? AND direction = ? AND occurred_at >= ? AND occurred_at < ?`,
		amountToman, direction, from.Unix(), to.Unix()).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check duplicate: %w", err)
	}
	return n > 0, nil
}

// HoldDuplicate implements storage.Transactions.
func (s *Store) HoldDuplicate(ctx context.Context, t storage.Transaction) (int64, error) {
	payload, err := json.Marshal(t)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `INSERT INTO held_duplicates (created_at, payload) VALUES (?, ?) RETURNING id`,
		t.CreatedAt.Unix(), string(payload)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("hold duplicate: %w", err)
	}
	return id, nil
}

// TakeHeldDuplicate implements storage.Transactions.
func (s *Store) TakeHeldDuplicate(ctx context.Context, id int64) (storage.Transaction, bool, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `DELETE FROM held_duplicates WHERE id = ? RETURNING payload`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Transaction{}, false, nil
	}
	if err != nil {
		return storage.Transaction{}, false, fmt.Errorf("take held duplicate %d: %w", id, err)
	}
	var t storage.Transaction
	if err := json.Unmarshal([]byte(payload), &t); err != nil {
		return storage.Transaction{}, false, fmt.Errorf("decode held duplicate %d: %w", id, err)
	}
	return t, true, nil
}
