package bot

import (
	"context"
	"fmt"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// The Input pipeline: one Text note from the Owner becomes Transactions.
//
//	extract (LLM) -> inputrules.Apply (pure) -> save -> confirmation
//
// Each step is its own function so later tickets can extend one step
// (Category, duplicates, Follow-ups, batches) without rewriting the others.

const (
	noTransactionReply    = "No transaction found. Send a bank message, with an optional note."
	extractionFailedReply = "Couldn't read that right now. Please send it again in a minute."
)

var tooManyReply = fmt.Sprintf("That's more than %d bank messages in one go, so nothing was saved. "+
	"Please resend them in smaller batches of up to %d.", inputrules.MaxBankMessages, inputrules.MaxBankMessages)

func init() { SetTextHandler(handleInput) }

func handleInput(ctx context.Context, b *Bot, m *bale.Message) error {
	now := b.Clock.Now()
	reading, err := b.Extractor.Extract(ctx, m.Text, now)
	if err != nil {
		b.Log.Warn("extraction failed", "err", err)
		return b.Reply(ctx, m, extractionFailedReply, nil)
	}
	out := inputrules.Apply(inputrules.Input{Text: m.Text, Extraction: reading, Now: now})
	switch {
	case !out.IsTransaction:
		return b.Reply(ctx, m, noTransactionReply, nil)
	case out.TooMany:
		return b.Reply(ctx, m, tooManyReply, nil)
	}
	var saved, held []storage.Transaction
	for i, d := range out.Drafts {
		tx := transactionFromDraft(d, m, i)
		tx.CreatedAt = now
		dup, err := b.isDuplicate(ctx, tx, d.HasTime)
		if err != nil {
			return err
		}
		if dup {
			held = append(held, tx)
			continue
		}
		id, err := b.Transactions.SaveTransaction(ctx, tx)
		if err != nil {
			return err
		}
		tx.ID = id
		saved = append(saved, tx)
	}
	if len(saved) > 0 {
		if err := b.sendConfirmations(ctx, m.Chat.ID, saved); err != nil {
			return err
		}
	}
	for _, tx := range held {
		if err := b.replyDuplicate(ctx, m.Chat.ID, tx); err != nil {
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
	switch d.Direction {
	case inputrules.In:
		tx.Direction = storage.DirectionIn
	case inputrules.Internal:
		tx.Direction = storage.DirectionInternal
	}
	switch d.FollowUp {
	case inputrules.FieldAmount:
		tx.Flagged, tx.FlagReason = true, storage.FlagAmountMissing
	case inputrules.FieldDirection:
		tx.Flagged, tx.FlagReason = true, storage.FlagDirectionAmbiguous
	}
	return tx
}
