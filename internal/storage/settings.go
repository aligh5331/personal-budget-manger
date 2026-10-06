package storage

import "context"

// Settings holds the bot's small persisted state.
type Settings interface {
	// LastUpdateID returns the highest Bale update_id already processed.
	// ok is false when no update was ever processed.
	LastUpdateID(ctx context.Context) (id int64, ok bool, err error)
	// SaveLastUpdateID records id as the highest processed update_id.
	SaveLastUpdateID(ctx context.Context, id int64) error
	// UpdateMode returns the update mode the Owner chose with /mode
	// ("polling" or "webhook"). ok is false when none was ever chosen.
	UpdateMode(ctx context.Context) (mode string, ok bool, err error)
	// SaveUpdateMode records the Owner's chosen update mode.
	SaveUpdateMode(ctx context.Context, mode string) error
}
