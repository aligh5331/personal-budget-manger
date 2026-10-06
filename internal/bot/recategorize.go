package bot

import (
	"context"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// Background re-categorize (#36). A Transaction saved Uncategorized while
// Jev was down carries categorize_pending and a queue entry (storage
// CategorizeRetries) holding its confirmation message. Every RetryInterval
// the job asks the Categorizer again, at most MaxRetries times, then clears
// the mark. A result is applied only while the row is still Uncategorized
// and still pending, so the Owner's hand pick always wins.

const (
	RetryInterval = 10 * time.Minute
	MaxRetries    = 6
	// retryPoll is how often the production loop looks for due entries.
	retryPoll = time.Minute
)

// queueRecategorize puts a pending Transaction on the queue, remembering the
// confirmation to edit later.
func (b *Bot) queueRecategorize(ctx context.Context, tx storage.Transaction, confirmation bale.Message) error {
	if !tx.CategorizePending {
		return nil
	}
	return b.Retries.QueueRetry(ctx, storage.Retry{
		TransactionID: tx.ID,
		ChatID:        confirmation.Chat.ID,
		MessageID:     confirmation.MessageID,
		NextAt:        b.Clock.Now().Add(RetryInterval),
	})
}

// RunRecategorize runs RecategorizeDue until ctx ends.
func (b *Bot) RunRecategorize(ctx context.Context) {
	t := time.NewTicker(retryPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := b.RecategorizeDue(ctx); err != nil {
				b.Log.Warn("background re-categorize", "err", err)
			}
		}
	}
}

// RecategorizeDue tries every queued Transaction whose time has come.
func (b *Bot) RecategorizeDue(ctx context.Context) error {
	due, err := b.Retries.DueRetries(ctx, b.Clock.Now())
	if err != nil {
		return err
	}
	for _, r := range due {
		if err := b.retryCategory(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bot) retryCategory(ctx context.Context, r storage.Retry) error {
	tx, ok, err := b.Transactions.TransactionByID(ctx, r.TransactionID)
	if err != nil {
		return err
	}
	kind, hasKind := storage.CategoryKindFor(tx.Direction)
	if !ok || !tx.CategorizePending || !hasKind {
		// Deleted, picked by the Owner, or no longer categorizable.
		return b.Retries.FinishRetry(ctx, r.TransactionID)
	}
	un, err := b.Categories.Uncategorized(ctx)
	if err != nil {
		return err
	}
	options, err := b.categoryOptions(ctx, kind, un)
	if err != nil {
		return err
	}
	pick, err := b.Categorizer.Categorize(ctx, categorizerText(tx), options)
	if err != nil {
		tries := r.Tries + 1
		if tries >= MaxRetries {
			b.Log.Warn("re-categorize gave up", "transaction", tx.ID, "tries", tries, "err", err)
			return b.Retries.FinishRetry(ctx, tx.ID)
		}
		b.Log.Warn("re-categorize failed", "transaction", tx.ID, "tries", tries, "err", err)
		return b.Retries.RescheduleRetry(ctx, tx.ID, tries, b.Clock.Now().Add(RetryInterval))
	}
	known := false
	for _, o := range options {
		if o.ID == pick.ID && !o.None {
			known = true
		}
	}
	if !known || pick.Confidence < MinCategoryConfidence {
		b.Log.Info("re-categorize left Uncategorized", "transaction", tx.ID, "confidence", pick.Confidence)
		return b.Retries.FinishRetry(ctx, tx.ID)
	}
	applied, err := b.Retries.ApplyBackgroundCategory(ctx, tx.ID, pick.ID)
	if err != nil || !applied {
		return err
	}
	b.Log.Info("re-categorized", "transaction", tx.ID, "category", pick.ID, "confidence", pick.Confidence)
	tx.CategoryID, tx.CategorizePending = &pick.ID, false
	// The row is already right; a failed edit is only logged.
	b.editMessageAt(ctx, r.ChatID, r.MessageID, b.confirmationText(ctx, tx), confirmationMarkupFor(tx))
	return nil
}
