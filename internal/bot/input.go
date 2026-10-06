package bot

import (
	"context"
	"fmt"
	"time"

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
	extractionFailedReply = "Couldn't read that right now, so I saved your message as flagged with its text. Fix it later with /flagged."
)

var tooManyReply = fmt.Sprintf("That's more than %d bank messages in one go, so nothing was saved. "+
	"Please resend them in smaller batches of up to %d.", inputrules.MaxBankMessages, inputrules.MaxBankMessages)

func init() { SetTextHandler(handleInput) }

func handleInput(ctx context.Context, b *Bot, m *bale.Message) error {
	// A new Input closes any open Follow-up as Flagged (#37).
	if err := b.closeOpenFollowUps(ctx, m.Chat.ID); err != nil {
		return err
	}
	now := b.Clock.Now()
	reading, err := b.Extractor.Extract(ctx, m.Text, now)
	if err != nil {
		b.Log.Warn("extraction failed", "err", err)
		return b.saveUnreadInput(ctx, m, now)
	}
	out := inputrules.Apply(inputrules.Input{Text: m.Text, Extraction: reading, Now: now})
	switch {
	case !out.IsTransaction:
		return b.Reply(ctx, m, noTransactionReply, nil)
	case out.TooMany:
		return b.Reply(ctx, m, tooManyReply, nil)
	}
	var saved, held []storage.Transaction
	queued := false
	for i, d := range out.Drafts {
		tx := transactionFromDraft(d, m, i)
		tx.CreatedAt = now
		if d.FollowUp != inputrules.FieldNone {
			field := storage.FollowUpAmount
			if d.FollowUp == inputrules.FieldDirection {
				field = storage.FollowUpDirection
			}
			unclear := field == storage.FollowUpAmount && d.Direction == inputrules.Ambiguous
			if err := b.queueFollowUp(ctx, m.Chat.ID, tx, field, d.HasTime, unclear); err != nil {
				return err
			}
			queued = true
			continue
		}
		tx, isHeld, err := b.saveDraft(ctx, tx, d.HasTime)
		if err != nil {
			return err
		}
		if isHeld {
			held = append(held, tx)
		} else {
			saved = append(saved, tx)
		}
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
	if queued {
		return b.askNextFollowUp(ctx, m.Chat.ID)
	}
	return nil
}

// saveDraft runs the last steps for a complete draft: duplicate check,
// Category step, save. A duplicate is held, not saved (held is true and the
// returned Transaction is the held one, with no id).
func (b *Bot) saveDraft(ctx context.Context, tx storage.Transaction, hasTime bool) (_ storage.Transaction, held bool, _ error) {
	dup, err := b.isDuplicate(ctx, tx, hasTime)
	if err != nil {
		return tx, false, err
	}
	if dup {
		return tx, true, nil
	}
	if err := b.categorizeTransaction(ctx, &tx); err != nil {
		return tx, false, err
	}
	id, err := b.Transactions.SaveTransaction(ctx, tx)
	if err != nil {
		return tx, false, err
	}
	tx.ID = id
	return tx, false, nil
}

// saveUnreadInput keeps an Input the extraction service could not read (it
// failed twice) as a Flagged transaction with its raw text and no amount.
func (b *Bot) saveUnreadInput(ctx context.Context, m *bale.Message, now time.Time) error {
	tx := storage.Transaction{
		CreatedAt:  now,
		OccurredAt: now,
		Direction:  storage.DirectionOut,
		RawText:    m.Text,
		InputID:    storage.NewInputID(m.MessageID, 0),
		Flagged:    true,
		FlagReason: storage.FlagAmountMissing,
	}
	tx, _, err := b.saveDraft(ctx, tx, false)
	if err != nil {
		return err
	}
	return b.sendFlagged(ctx, m.Chat.ID, extractionFailedReply, []storage.Transaction{tx})
}

// transactionFromDraft maps a draft to a Transaction. A draft that still
// needs a Follow-up becomes the Flagged placeholder that is saved if the
// Owner does not answer (an ambiguous Direction is stored as out).
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
