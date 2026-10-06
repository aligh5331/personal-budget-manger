package sqlite

import (
	"context"
	"fmt"
)

// SnapshotTo writes a consistent copy of the database to path with VACUUM
// INTO. path must not exist yet. This is SQLite-only maintenance, not part of
// any storage interface.
func (s *Store) SnapshotTo(ctx context.Context, path string) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("vacuum into %s: %w", path, err)
	}
	return nil
}
