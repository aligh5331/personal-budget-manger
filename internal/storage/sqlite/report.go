package sqlite

import (
	"context"
	"fmt"

	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// ReportSummary implements storage.Transactions.
func (s *Store) ReportSummary(ctx context.Context, r storage.ReportRange) (storage.ReportSummary, error) {
	var sum storage.ReportSummary
	from, to := r.From.Unix(), r.To.Unix()

	rows, err := s.db.QueryContext(ctx, `SELECT t.category_id, COALESCE(c.name, ''), t.direction, SUM(t.amount_toman)
		FROM transactions t LEFT JOIN categories c ON c.id = t.category_id
		WHERE t.occurred_at >= ? AND t.occurred_at < ? AND t.flagged = 0 AND t.direction IN (?, ?)
		GROUP BY t.category_id, t.direction`,
		from, to, storage.DirectionOut, storage.DirectionIn)
	if err != nil {
		return sum, fmt.Errorf("report totals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			l   storage.ReportLine
			cat *int64
		)
		if err := rows.Scan(&cat, &l.CategoryName, &l.Direction, &l.Total); err != nil {
			return sum, fmt.Errorf("report totals: %w", err)
		}
		if cat != nil {
			l.CategoryID = *cat
		}
		sum.Lines = append(sum.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return sum, fmt.Errorf("report totals: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(amount_toman), 0)
		FROM transactions
		WHERE occurred_at >= ? AND occurred_at < ? AND flagged = 0 AND direction = ?`,
		from, to, storage.DirectionInternal).Scan(&sum.InternalCount, &sum.InternalToman); err != nil {
		return sum, fmt.Errorf("report internal transfers: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions
		WHERE occurred_at >= ? AND occurred_at < ? AND flagged = 1`,
		from, to).Scan(&sum.FlaggedCount); err != nil {
		return sum, fmt.Errorf("report flagged: %w", err)
	}
	return sum, nil
}
