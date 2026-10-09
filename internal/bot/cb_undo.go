package bot

import (
	"context"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
)

func init() {
	RegisterConfirmationButton(ConfirmationButton{Order: 10, Label: "Undo", Action: "undo"})
	RegisterTransactionAction("undo", runUndo)
}

// runUndo hard-deletes the Transaction and redraws its confirmation: a
// single one becomes "Removed", a batch keeps the Transactions still saved.
// It never expires: it needs only the id in the button.
func runUndo(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
	deleted, err := b.Transactions.DeleteTransaction(ctx, id)
	if err != nil {
		return "Couldn't remove it, try again.", err
	}
	if q.Message != nil {
		if err := b.refreshConfirmation(ctx, q.Message); err != nil {
			b.Log.Warn("undo: edit confirmation", "err", err)
		}
	}
	if !deleted {
		return "Already removed", nil
	}
	return "Removed", nil
}
