package storage

import (
	"context"
	"time"
)

// Retry is one Transaction waiting for a background re-categorize (#36),
// with the confirmation message to edit when it succeeds.
type Retry struct {
	TransactionID int64
	ChatID        int64
	MessageID     int64
	// Tries is how many background tries have failed so far.
	Tries  int
	NextAt time.Time
}

// CategorizeRetries is the queue of the background re-categorize job.
type CategorizeRetries interface {
	// QueueRetry adds (or replaces) the queue entry of a Transaction.
	QueueRetry(ctx context.Context, r Retry) error
	// DueRetries returns the entries whose NextAt is at or before now.
	DueRetries(ctx context.Context, now time.Time) ([]Retry, error)
	// RescheduleRetry records a failed try and the time of the next one.
	RescheduleRetry(ctx context.Context, transactionID int64, tries int, nextAt time.Time) error
	// FinishRetry removes the entry and clears the Transaction's
	// categorize_pending mark.
	FinishRetry(ctx context.Context, transactionID int64) error
	// ApplyBackgroundCategory sets the Category only while the Transaction
	// is still Uncategorized and still categorize_pending, and then clears
	// the mark and the queue entry. applied is false when the Owner (or an
	// edit) got there first.
	ApplyBackgroundCategory(ctx context.Context, transactionID, categoryID int64) (applied bool, err error)
}
