package storage

import (
	"context"
	"time"
)

// Follow-up fields.
const (
	FollowUpAmount    = "amount"
	FollowUpDirection = "direction"
)

// FollowUp is one pending question to the Owner (#37). It is either about a
// Transaction that is not saved yet (TransactionID 0, Draft holds it) or
// about an existing Flagged transaction (#38 [Fix]).
type FollowUp struct {
	ID     int64
	ChatID int64
	// PromptMessageID is the question's Bale message id; 0 while the
	// Follow-up is queued behind another one of the same batch Input.
	PromptMessageID int64
	// Field is FollowUpAmount or FollowUpDirection.
	Field string
	// ExpiresAt is set when the question is asked (zero while queued).
	ExpiresAt time.Time
	// TransactionID is the existing Flagged transaction being fixed, or 0.
	TransactionID int64
	// Draft is the Transaction to save when TransactionID is 0. It is the
	// Flagged placeholder that is saved if the Owner does not answer.
	Draft Transaction
	// HasTime is whether the bank printed a time (duplicate check).
	HasTime bool
	// DirectionUnclear is true when an amount Follow-up's draft also has an
	// ambiguous Direction. It stays Flagged after the amount is answered.
	DirectionUnclear bool
	CreatedAt        time.Time
}

// Asked reports whether the question has been sent.
func (f FollowUp) Asked() bool { return f.PromptMessageID != 0 }

// FollowUps stores pending Follow-ups, so they survive a restart.
type FollowUps interface {
	// SaveFollowUp stores f (ID ignored) and returns its id.
	SaveFollowUp(ctx context.Context, f FollowUp) (int64, error)
	// FollowUpByID returns a Follow-up; ok is false if it is closed.
	FollowUpByID(ctx context.Context, id int64) (f FollowUp, ok bool, err error)
	// FollowUpByPrompt returns the Follow-up asked as message messageID.
	FollowUpByPrompt(ctx context.Context, chatID, messageID int64) (f FollowUp, ok bool, err error)
	// FollowUpForTransaction returns the open Follow-up about an existing
	// Transaction.
	FollowUpForTransaction(ctx context.Context, transactionID int64) (f FollowUp, ok bool, err error)
	// MarkFollowUpAsked records that the question was sent as message
	// promptMessageID and expires at expiresAt.
	MarkFollowUpAsked(ctx context.Context, id, promptMessageID int64, expiresAt time.Time) error
	// CloseFollowUp removes a Follow-up. closed is false when someone else
	// already closed it, so only one caller resolves it.
	CloseFollowUp(ctx context.Context, id int64) (closed bool, err error)
	// OpenFollowUps returns the chat's Follow-ups, asked and queued, oldest first.
	OpenFollowUps(ctx context.Context, chatID int64) ([]FollowUp, error)
	// ExpiredFollowUps returns asked Follow-ups whose ExpiresAt is at or before now.
	ExpiredFollowUps(ctx context.Context, now time.Time) ([]FollowUp, error)
	// NextQueuedFollowUps returns, per chat that has queued Follow-ups but
	// none asked, the oldest queued one.
	NextQueuedFollowUps(ctx context.Context) ([]FollowUp, error)
}
