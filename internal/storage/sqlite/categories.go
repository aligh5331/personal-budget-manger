package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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

// ArchivedCategories implements storage.Categories.
func (s *Store) ArchivedCategories(ctx context.Context) ([]storage.Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+categoryColumns+` FROM categories
		WHERE archived = 1 AND builtin = 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list archived categories: %w", err)
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

// nameTaken reports whether a Category other than exceptID has name, ignoring
// case.
func (s *Store) nameTaken(ctx context.Context, name string, exceptID int64) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM categories WHERE id <> ?`, exceptID)
	if err != nil {
		return false, fmt.Errorf("check category name: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return false, err
		}
		if strings.EqualFold(other, name) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// AddCategory implements storage.Categories.
func (s *Store) AddCategory(ctx context.Context, name, hint, kind string) (storage.Category, error) {
	if taken, err := s.nameTaken(ctx, name, 0); err != nil {
		return storage.Category{}, err
	} else if taken {
		return storage.Category{}, storage.ErrCategoryNameTaken
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO categories (name, hint, kind) VALUES (?, ?, ?)`, name, hint, kind)
	if err != nil {
		return storage.Category{}, fmt.Errorf("add category: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return storage.Category{}, err
	}
	return storage.Category{ID: id, Name: name, Hint: hint, Kind: kind}, nil
}

// RenameCategory implements storage.Categories.
func (s *Store) RenameCategory(ctx context.Context, id int64, name string) (bool, error) {
	if taken, err := s.nameTaken(ctx, name, id); err != nil {
		return false, err
	} else if taken {
		return false, storage.ErrCategoryNameTaken
	}
	res, err := s.db.ExecContext(ctx, `UPDATE categories
		SET hint = CASE WHEN hint = name THEN ? ELSE hint END, name = ?
		WHERE id = ? AND builtin = 0`, name, name, id)
	if err != nil {
		return false, fmt.Errorf("rename category %d: %w", id, err)
	}
	return rowsChanged(res)
}

// SetCategoryArchived implements storage.Categories.
func (s *Store) SetCategoryArchived(ctx context.Context, id int64, archived bool) (bool, error) {
	v := 0
	if archived {
		v = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE categories SET archived = ? WHERE id = ? AND builtin = 0`, v, id)
	if err != nil {
		return false, fmt.Errorf("archive category %d: %w", id, err)
	}
	return rowsChanged(res)
}

func rowsChanged(res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	return n > 0, err
}

// SaveCategoryPrompt implements storage.Categories.
func (s *Store) SaveCategoryPrompt(ctx context.Context, p storage.CategoryPrompt) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO category_prompts (
		chat_id, prompt_message_id, action, category_id, host_message_id, created_at
	) VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT (chat_id, prompt_message_id) DO UPDATE SET
		action = excluded.action, category_id = excluded.category_id,
		host_message_id = excluded.host_message_id, created_at = excluded.created_at`,
		p.ChatID, p.PromptMessageID, p.Action, p.CategoryID, p.HostMessageID, p.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("save category prompt: %w", err)
	}
	return nil
}

// CategoryPromptByMessage implements storage.Categories.
func (s *Store) CategoryPromptByMessage(ctx context.Context, chatID, messageID int64) (storage.CategoryPrompt, bool, error) {
	p := storage.CategoryPrompt{ChatID: chatID, PromptMessageID: messageID}
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT action, category_id, host_message_id, created_at
		FROM category_prompts WHERE chat_id = ? AND prompt_message_id = ?`, chatID, messageID,
	).Scan(&p.Action, &p.CategoryID, &p.HostMessageID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.CategoryPrompt{}, false, nil
	}
	if err != nil {
		return storage.CategoryPrompt{}, false, fmt.Errorf("read category prompt: %w", err)
	}
	p.CreatedAt = time.Unix(created, 0).UTC()
	return p, true, nil
}
