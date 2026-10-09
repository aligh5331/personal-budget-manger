package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

const (
	settingLastUpdateID = "last_update_id"
	settingUpdateMode   = "update_mode"
)

func (s *Store) getSetting(ctx context.Context, name string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE name = ?`, name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read setting %s: %w", name, err)
	}
	return v, true, nil
}

func (s *Store) putSetting(ctx context.Context, name, value string) error {
	// Portable upsert: Postgres and SQLite >= 3.24 both accept ON CONFLICT.
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (name, value) VALUES (?, ?)
		 ON CONFLICT (name) DO UPDATE SET value = excluded.value`, name, value)
	if err != nil {
		return fmt.Errorf("write setting %s: %w", name, err)
	}
	return nil
}

// LastUpdateID implements storage.Settings.
func (s *Store) LastUpdateID(ctx context.Context) (int64, bool, error) {
	v, ok, err := s.getSetting(ctx, settingLastUpdateID)
	if err != nil || !ok {
		return 0, false, err
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("setting %s: %w", settingLastUpdateID, err)
	}
	return id, true, nil
}

// UpdateMode implements storage.Settings.
func (s *Store) UpdateMode(ctx context.Context) (string, bool, error) {
	return s.getSetting(ctx, settingUpdateMode)
}

// SaveUpdateMode implements storage.Settings.
func (s *Store) SaveUpdateMode(ctx context.Context, mode string) error {
	return s.putSetting(ctx, settingUpdateMode, mode)
}

// SaveLastUpdateID implements storage.Settings.
func (s *Store) SaveLastUpdateID(ctx context.Context, id int64) error {
	return s.putSetting(ctx, settingLastUpdateID, strconv.FormatInt(id, 10))
}
