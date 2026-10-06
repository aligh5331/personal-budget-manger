package bot

import (
	"context"
	"strconv"
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// One confirmation lists every Transaction saved from an Input. A single
// Transaction is shown as confirmationText renders it. A batch numbers each
// one ("1) ...") and gives it its own row of buttons labelled with its number
// ("Undo 1"). The message's own buttons record which Transactions it lists,
// so refreshConfirmation can redraw it after one of them changes without
// storing anything else.

const removedText = "Removed"

// sendConfirmations sends one confirmation listing txs (at least one).
func (b *Bot) sendConfirmations(ctx context.Context, chatID int64, txs []storage.Transaction) error {
	text, markup := b.renderConfirmations(ctx, txs)
	_, err := b.Send(ctx, chatID, text, markup)
	return err
}

// renderConfirmations renders the confirmation for txs; with none left it
// is "Removed" with no buttons.
func (b *Bot) renderConfirmations(ctx context.Context, txs []storage.Transaction) (string, *bale.InlineKeyboardMarkup) {
	switch len(txs) {
	case 0:
		return removedText, nil
	case 1:
		return b.confirmationText(ctx, txs[0]), confirmationMarkup(txs[0].ID)
	}
	parts := make([]string, len(txs))
	var rows [][]bale.InlineKeyboardButton
	for i, tx := range txs {
		n := strconv.Itoa(i + 1)
		parts[i] = n + ") " + b.confirmationText(ctx, tx)
		row := make([]bale.InlineKeyboardButton, 0, len(confirmationButtons))
		for _, c := range confirmationButtons {
			row = append(row, bale.InlineKeyboardButton{Text: c.Label + " " + n, CallbackData: TransactionData(tx.ID, c.Action)})
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	var markup *bale.InlineKeyboardMarkup
	if len(rows) > 0 {
		markup = &bale.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	return strings.Join(parts, "\n\n"), markup
}

// refreshConfirmation redraws the confirmation msg from the Transactions it
// lists that are still stored. When none is left (or msg carries no
// Transaction buttons) it becomes "Removed" with no buttons.
func (b *Bot) refreshConfirmation(ctx context.Context, msg *bale.Message) error {
	var left []storage.Transaction
	for _, id := range listedTransactions(msg) {
		tx, ok, err := b.Transactions.TransactionByID(ctx, id)
		if err != nil {
			return err
		}
		if ok {
			left = append(left, tx)
		}
	}
	text, markup := b.renderConfirmations(ctx, left)
	// Without reply_markup the edit also drops the buttons.
	return b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{
		ChatID: msg.Chat.ID, MessageID: msg.MessageID, Text: text, ReplyMarkup: markup,
	})
}

// listedTransactions returns the Transaction ids a confirmation's buttons
// refer to (t:<id>:<action>), once each, in the order they are listed.
func listedTransactions(msg *bale.Message) []int64 {
	if msg == nil || msg.ReplyMarkup == nil {
		return nil
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, row := range msg.ReplyMarkup.InlineKeyboard {
		for _, btn := range row {
			parts := strings.SplitN(btn.CallbackData, ":", 3)
			if len(parts) != 3 || parts[0] != "t" {
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil || id <= 0 || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
