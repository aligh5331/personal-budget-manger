package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

var _ storage.Categories = (*Store)(nil)

const categoryColumns = `id, name, hint, kind, archived, builtin`

// Uncategorized implements storage.Categories.
func (s *Store) Uncategorized(ctx context.Context) (storage.Category, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+categoryColumns+` FROM categories WHERE builtin = 1 ORDER BY id LIMIT 1`)
	c, err := scanCategory(row)
	if err != nil {
		return storage.Category{}, fmt.Errorf("read Uncategorized: %w", err)
	}
	return c, nil
}

// ActiveCategories implements storage.Categories.
func (s *Store) ActiveCategories(ctx context.Context, kind string) ([]storage.Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+categoryColumns+` FROM categories
		WHERE kind = ? AND archived = 0 AND builtin = 0 ORDER BY id`, kind)
	if err != nil {
		return nil, fmt.Errorf("list %s categories: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	var out []storage.Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CategoryByID implements storage.Categories.
func (s *Store) CategoryByID(ctx context.Context, id int64) (storage.Category, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+categoryColumns+` FROM categories WHERE id = ?`, id)
	c, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Category{}, false, nil
	}
	if err != nil {
		return storage.Category{}, false, fmt.Errorf("read category %d: %w", id, err)
	}
	return c, true, nil
}

func scanCategory(row interface{ Scan(...any) error }) (storage.Category, error) {
	var (
		c                 storage.Category
		archived, builtin int
	)
	if err := row.Scan(&c.ID, &c.Name, &c.Hint, &c.Kind, &archived, &builtin); err != nil {
		return storage.Category{}, err
	}
	c.Archived = archived != 0
	c.BuiltIn = builtin != 0
	return c, nil
}
