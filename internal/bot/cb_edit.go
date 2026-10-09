package bot

import (
	"context"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// [Edit] on every confirmation; the flow itself is in edit.go.
func init() {
	RegisterConfirmationButton(ConfirmationButton{Order: 20, Label: "Edit", Action: editAction(actEdit, "")})
	RegisterTransactionView(TransactionView{
		Key: "",
		Render: func(ctx context.Context, b *Bot, tx storage.Transaction) (string, *bale.InlineKeyboardMarkup) {
			return b.confirmationText(ctx, tx), confirmationMarkupFor(tx)
		},
	})
}
