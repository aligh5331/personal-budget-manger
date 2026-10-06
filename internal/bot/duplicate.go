package bot

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// Duplicates. With a bank time the key is amount, Direction and minute;
// without one it is amount, Direction and date (Asia/Tehran). A match saves
// nothing: the bot says so and offers [Save anyway], which saves the held
// Transaction (callback prefix "d", data d:<held id>:save).

func init() {
	RegisterCallback(Callback{Prefix: "d", Run: runSaveAnyway})
}

// isDuplicate reports whether tx matches a stored Transaction. hasTime is
// whether the bank printed a time.
func (b *Bot) isDuplicate(ctx context.Context, tx storage.Transaction, hasTime bool) (bool, error) {
	if tx.AmountToman == nil || tx.Flagged {
		return false, nil
	}
	from, to := duplicateWindow(tx.OccurredAt, hasTime)
	return b.Transactions.TransactionExists(ctx, *tx.AmountToman, tx.Direction, from, to)
}

func duplicateWindow(at time.Time, hasTime bool) (from, to time.Time) {
	if hasTime {
		from = at.Truncate(time.Minute)
		return from, from.Add(time.Minute)
	}
	y, m, d := at.In(clock.Tehran()).Date()
	from = time.Date(y, m, d, 0, 0, 0, 0, clock.Tehran())
	return from, from.AddDate(0, 0, 1)
}

// replyDuplicate holds tx and tells the Owner it was not saved.
func (b *Bot) replyDuplicate(ctx context.Context, chatID int64, tx storage.Transaction) error {
	id, err := b.Transactions.HoldDuplicate(ctx, tx)
	if err != nil {
		return err
	}
	text := "This looks like a duplicate, so I didn't save it:\n" + b.confirmationSummary(ctx, tx)
	_, err = b.Send(ctx, chatID, text, &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{{
		{Text: "Save anyway", CallbackData: "d:" + strconv.FormatInt(id, 10) + ":save"},
	}}})
	return err
}

// confirmationSummary is the one-line "amount · Direction, date" of tx.
func (b *Bot) confirmationSummary(_ context.Context, tx storage.Transaction) string {
	amount := "amount unknown"
	if tx.AmountToman != nil {
		amount = FormatToman(*tx.AmountToman) + " toman"
	}
	return amount + " · " + directionText(tx) + ", " + FormatDate(tx.OccurredAt)
}

func runSaveAnyway(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	parts := strings.SplitN(q.Data, ":", 3)
	if len(parts) != 3 || parts[2] != "save" {
		return staleButtonToast, nil
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return staleButtonToast, nil
	}
	tx, ok, err := b.Transactions.TakeHeldDuplicate(ctx, id)
	if err != nil {
		return "Couldn't save it, try again.", err
	}
	if !ok {
		return "Already saved", nil
	}
	tx.CreatedAt = b.Clock.Now()
	if err := b.categorizeTransaction(ctx, &tx); err != nil {
		return "Couldn't save it, try again.", err
	}
	if tx.ID, err = b.Transactions.SaveTransaction(ctx, tx); err != nil {
		return "Couldn't save it, try again.", err
	}
	if q.Message != nil {
		text, markup := b.renderConfirmations(ctx, []storage.Transaction{tx})
		if err := b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{
			ChatID: q.Message.Chat.ID, MessageID: q.Message.MessageID, Text: text, ReplyMarkup: markup,
		}); err != nil {
			b.Log.Warn("save anyway: edit warning", "err", err)
		}
	}
	return "Saved", nil
}
