package storage

import "context"

// Settings holds the bot's small persisted state.
type Settings interface {
	// LastUpdateID returns the highest Bale update_id already processed.
	// ok is false when no update was ever processed.
	LastUpdateID(ctx context.Context) (id int64, ok bool, err error)
	// SaveLastUpdateID records id as the highest processed update_id.
	SaveLastUpdateID(ctx context.Context, id int64) error
}
