package bot

import (
	"context"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

func init() {
	RegisterConfirmationButton(ConfirmationButton{Order: 10, Label: "Undo", Action: "undo"})
	RegisterTransactionAction("undo", runUndo)
}

// runUndo hard-deletes the Transaction and turns its confirmation into
// "Removed". It never expires: it needs only the id in the button.
func runUndo(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
	deleted, err := b.Transactions.DeleteTransaction(ctx, id)
	if err != nil {
		return "Couldn't remove it, try again.", err
	}
	if q.Message != nil {
		// Without reply_markup the edit also drops the buttons.
		if err := b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{
			ChatID:    q.Message.Chat.ID,
			MessageID: q.Message.MessageID,
			Text:      "Removed",
		}); err != nil {
			b.Log.Warn("undo: edit confirmation", "err", err)
		}
	}
	if !deleted {
		return "Already removed", nil
	}
	return "Removed", nil
}
