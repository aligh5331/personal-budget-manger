package storage

import (
	"context"
	"strconv"
	"time"
)

// Direction values stored on a Transaction.
const (
	DirectionOut      = "out"
	DirectionIn       = "in"
	DirectionInternal = "internal"
)

// Flag reasons stored on a Flagged transaction.
const (
	FlagAmountMissing      = "amount_missing"
	FlagDirectionAmbiguous = "direction_ambiguous"
	FlagFollowupTimeout    = "followup_timeout"
	FlagFollowupUnparsed   = "followup_unparsed"
)

// Transaction is one saved money movement.
type Transaction struct {
	ID         int64
	CreatedAt  time.Time
	OccurredAt time.Time
	// AmountToman is nil only on a Flagged transaction with no amount.
	AmountToman *int64
	// Direction is DirectionOut, DirectionIn or DirectionInternal.
	Direction string
	// CategoryID is nil for internal transfers (and until Categories exist).
	CategoryID *int64
	// Description is the Owner's own words only.
	Description string
	// BankLabel is the bank's own label ("" is stored as NULL).
	BankLabel string
	// RawText is the whole Input as the Owner sent it.
	RawText string
	// InputID identifies the Input: see NewInputID.
	InputID           string
	Flagged           bool
	FlagReason        string
	CategorizePending bool
}

// NewInputID builds a Transaction's input_id: the Bale message id of the
// Input plus the index of the bank message within it.
func NewInputID(messageID int64, index int) string {
	return strconv.FormatInt(messageID, 10) + ":" + strconv.Itoa(index)
}

// Transactions stores Transactions.
type Transactions interface {
	// SaveTransaction stores a new Transaction and returns its id. t.ID is
	// ignored; t.CreatedAt comes from the caller's clock.
	SaveTransaction(ctx context.Context, t Transaction) (int64, error)
	// TransactionByID returns a Transaction; ok is false if there is none.
	TransactionByID(ctx context.Context, id int64) (t Transaction, ok bool, err error)
	// DeleteTransaction hard-deletes a Transaction by id. deleted is false
	// when it was already gone.
	DeleteTransaction(ctx context.Context, id int64) (deleted bool, err error)
	// TransactionExists reports whether a Transaction with this amount and
	// Direction occurred in [from, to). It is the duplicate check.
	TransactionExists(ctx context.Context, amountToman int64, direction string, from, to time.Time) (bool, error)
	// HoldDuplicate keeps a Transaction that was not saved because it looks
	// like a duplicate, and returns the id for [Save anyway].
	HoldDuplicate(ctx context.Context, t Transaction) (id int64, err error)
	// TakeHeldDuplicate returns and removes a held duplicate; ok is false
	// if it was already taken.
	TakeHeldDuplicate(ctx context.Context, id int64) (t Transaction, ok bool, err error)
	// AllTransactions returns every Transaction, oldest id first (/export).
	AllTransactions(ctx context.Context) ([]Transaction, error)
}
