package bot

import (
	"context"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// The Input pipeline: one Text note from the Owner becomes Transactions.
//
//	extract (LLM) -> inputrules.Apply (pure) -> Category (category.go) -> save -> confirmation
//
// Each step is its own function so later tickets can extend one step
// (Category, duplicates, Follow-ups, batches) without rewriting the others.

const (
	noTransactionReply    = "No transaction found. Send a bank message, with an optional note."
	extractionFailedReply = "Couldn't read that right now. Please send it again in a minute."
)

func init() { SetTextHandler(handleInput) }

func handleInput(ctx context.Context, b *Bot, m *bale.Message) error {
	now := b.Clock.Now()
	reading, err := b.Extractor.Extract(ctx, m.Text, now)
	if err != nil {
		b.Log.Warn("extraction failed", "err", err)
		return b.Reply(ctx, m, extractionFailedReply, nil)
	}
	out := inputrules.Apply(inputrules.Input{Text: m.Text, Extraction: reading, Now: now})
	if !out.IsTransaction {
		return b.Reply(ctx, m, noTransactionReply, nil)
	}
	for i, d := range out.Drafts {
		tx := transactionFromDraft(d, m, i)
		tx.CreatedAt = now
		if err := b.categorizeTransaction(ctx, &tx); err != nil {
			return err
		}
		id, err := b.Transactions.SaveTransaction(ctx, tx)
		if err != nil {
			return err
		}
		tx.ID = id
		if err := b.sendConfirmation(ctx, m.Chat.ID, tx); err != nil {
			return err
		}
	}
	return nil
}

// transactionFromDraft maps a draft to the row to save. A draft that still
// needs a Follow-up is saved as a Flagged transaction for now, so the Input
// is never lost.
func transactionFromDraft(d inputrules.Draft, m *bale.Message, index int) storage.Transaction {
	tx := storage.Transaction{
		OccurredAt:  d.OccurredAt,
		Direction:   storage.DirectionOut,
		Description: d.Description,
		BankLabel:   d.BankLabel,
		RawText:     m.Text,
		InputID:     storage.NewInputID(m.MessageID, index),
	}
	if d.HasAmount {
		amount := d.AmountToman
		tx.AmountToman = &amount
	}
	if d.Direction == inputrules.In {
		tx.Direction = storage.DirectionIn
	}
	switch d.FollowUp {
	case inputrules.FieldAmount:
		tx.Flagged, tx.FlagReason = true, storage.FlagAmountMissing
	case inputrules.FieldDirection:
		tx.Flagged, tx.FlagReason = true, storage.FlagDirectionAmbiguous
	}
	return tx
}
