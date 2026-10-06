package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

var _ storage.Transactions = (*Store)(nil)

const transactionColumns = `id, created_at, occurred_at, amount_toman, direction, category_id,
	description, bank_label, raw_text, input_id, flagged, flag_reason, categorize_pending`

// SaveTransaction implements storage.Transactions.
func (s *Store) SaveTransaction(ctx context.Context, t storage.Transaction) (int64, error) {
	var bankLabel sql.NullString
	if t.BankLabel != "" {
		bankLabel = sql.NullString{String: t.BankLabel, Valid: true}
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO transactions (
		created_at, occurred_at, amount_toman, direction, category_id,
		description, bank_label, raw_text, input_id, flagged, flag_reason, categorize_pending
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		t.CreatedAt.Unix(), t.OccurredAt.Unix(), t.AmountToman, t.Direction, t.CategoryID,
		t.Description, bankLabel, t.RawText, t.InputID, boolInt(t.Flagged), t.FlagReason, boolInt(t.CategorizePending),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save transaction: %w", err)
	}
	return id, nil
}

// TransactionByID implements storage.Transactions.
func (s *Store) TransactionByID(ctx context.Context, id int64) (storage.Transaction, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+transactionColumns+` FROM transactions WHERE id = ?`, id)
	t, err := scanTransaction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Transaction{}, false, nil
	}
	if err != nil {
		return storage.Transaction{}, false, fmt.Errorf("read transaction %d: %w", id, err)
	}
	return t, true, nil
}

// DeleteTransaction implements storage.Transactions.
func (s *Store) DeleteTransaction(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM transactions WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete transaction %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// UpdateTransaction implements storage.Transactions.
func (s *Store) UpdateTransaction(ctx context.Context, t storage.Transaction) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE transactions SET
		occurred_at = ?, amount_toman = ?, direction = ?, category_id = ?,
		description = ?, flagged = ?, flag_reason = ?, categorize_pending = ?
	WHERE id = ?`,
		t.OccurredAt.Unix(), t.AmountToman, t.Direction, t.CategoryID,
		t.Description, boolInt(t.Flagged), t.FlagReason, boolInt(t.CategorizePending), t.ID,
	)
	if err != nil {
		return false, fmt.Errorf("update transaction %d: %w", t.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// scanTransaction reads one row selected with transactionColumns.
func scanTransaction(row interface{ Scan(...any) error }) (storage.Transaction, error) {
	var (
		t                          storage.Transaction
		created, occurred          int64
		amount, category           sql.NullInt64
		bankLabel                  sql.NullString
		flagged, categorizePending int
	)
	if err := row.Scan(&t.ID, &created, &occurred, &amount, &t.Direction, &category,
		&t.Description, &bankLabel, &t.RawText, &t.InputID, &flagged, &t.FlagReason, &categorizePending); err != nil {
		return storage.Transaction{}, err
	}
	t.CreatedAt = time.Unix(created, 0).UTC()
	t.OccurredAt = time.Unix(occurred, 0).UTC()
	if amount.Valid {
		t.AmountToman = &amount.Int64
	}
	if category.Valid {
		t.CategoryID = &category.Int64
	}
	t.BankLabel = bankLabel.String
	t.Flagged = flagged != 0
	t.CategorizePending = categorizePending != 0
	return t, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
