package bot

import (
	"context"
	"strconv"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// The [Category] button (#35). Callback data:
//
//	t:<id>:cat            open the picker on the confirmation
//	t:<id>:cat:<cat id>   pick that Category
//	t:<id>:cat:back       close the picker, keep the Category
//
// The picker is the confirmation edited in place; picking or going back
// restores the confirmation with its buttons.

const (
	catAction       = "cat"
	catBack         = "back"
	pickerPerRow    = 2
	currentMarker   = "• "
	removedToast    = "This Transaction was removed."
	unavailableText = "That Category is no longer available."
)

func init() {
	RegisterConfirmationButton(ConfirmationButton{
		Order: 20, Label: "Category", Action: catAction,
		Shown: func(tx storage.Transaction) bool { return tx.Direction != storage.DirectionInternal },
	})
	registerCategoryPicker("")
}

// registerCategoryPicker adds the [Category] actions for the view viewKey:
// the picker is that view redrawn in place (the confirmation, or the
// /transactions record view, whose actions are "cat@r").
func registerCategoryPicker(viewKey string) {
	RegisterTransactionAction(editAction(catAction, viewKey), func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
		return b.runCategory(ctx, q, id, viewKey)
	})
}

func (b *Bot) runCategory(ctx context.Context, q *bale.CallbackQuery, id int64, viewKey string) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil {
		return "Something went wrong, try again.", err
	}
	if !ok {
		return removedToast, nil
	}
	kind, hasKind := storage.CategoryKindFor(tx.Direction)
	if !hasKind {
		return "Internal transfers have no Category.", nil
	}
	switch arg := TransactionArg(q); arg {
	case "":
		return b.openCategoryPicker(ctx, q, tx, kind, viewKey)
	case catBack:
		b.editView(ctx, q, tx, viewKey)
		return "", nil
	default:
		catID, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return staleButtonToast, nil
		}
		return b.pickCategory(ctx, q, tx, kind, catID, viewKey)
	}
}

func (b *Bot) openCategoryPicker(ctx context.Context, q *bale.CallbackQuery, tx storage.Transaction, kind, viewKey string) (string, error) {
	cats, err := b.Categories.ActiveCategories(ctx, kind)
	if err != nil {
		return "Something went wrong, try again.", err
	}
	un, err := b.Categories.Uncategorized(ctx)
	if err != nil {
		return "Something went wrong, try again.", err
	}
	cats = append(cats, un)

	var rows [][]bale.InlineKeyboardButton
	var row []bale.InlineKeyboardButton
	for _, c := range cats {
		label := c.Name
		if tx.CategoryID != nil && *tx.CategoryID == c.ID {
			label = currentMarker + label
		}
		row = append(row, bale.InlineKeyboardButton{Text: label, CallbackData: TransactionData(tx.ID, editAction(catAction, viewKey)+":"+strconv.FormatInt(c.ID, 10))})
		if len(row) == pickerPerRow {
			rows, row = append(rows, row), nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []bale.InlineKeyboardButton{{Text: "Back", CallbackData: TransactionData(tx.ID, editAction(catAction, viewKey)+":"+catBack)}})
	text, _ := transactionViews[viewKey].Render(ctx, b, tx)
	b.editMessage(ctx, q, text, &bale.InlineKeyboardMarkup{InlineKeyboard: rows})
	return "", nil
}

func (b *Bot) pickCategory(ctx context.Context, q *bale.CallbackQuery, tx storage.Transaction, kind string, catID int64, viewKey string) (string, error) {
	c, ok, err := b.Categories.CategoryByID(ctx, catID)
	if err != nil {
		return "Something went wrong, try again.", err
	}
	if !ok || c.Archived || (c.Kind != kind && !c.BuiltIn) {
		return unavailableText, nil
	}
	updated, err := b.Transactions.SetOwnerCategory(ctx, tx.ID, c.ID)
	if err != nil {
		return "Couldn't save that, try again.", err
	}
	if !updated {
		return removedToast, nil
	}
	tx.CategoryID = &c.ID
	tx.CategorizePending = false
	b.editView(ctx, q, tx, viewKey)
	return "Category: " + c.Name, nil
}

// editView redraws tx in the tapped message with the view's usual buttons.
func (b *Bot) editView(ctx context.Context, q *bale.CallbackQuery, tx storage.Transaction, viewKey string) {
	text, markup := transactionViews[viewKey].Render(ctx, b, tx)
	b.editMessage(ctx, q, text, markup)
}

// editConfirmation redraws tx's confirmation, with its buttons, on the
// message the Owner tapped.
func (b *Bot) editConfirmation(ctx context.Context, q *bale.CallbackQuery, tx storage.Transaction) {
	b.editView(ctx, q, tx, "")
}

// editMessage edits the tapped message; a failed edit is only logged, since
// the stored data is already right.
func (b *Bot) editMessage(ctx context.Context, q *bale.CallbackQuery, text string, markup *bale.InlineKeyboardMarkup) {
	if q.Message == nil {
		return
	}
	if err := b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{
		ChatID: q.Message.Chat.ID, MessageID: q.Message.MessageID, Text: text, ReplyMarkup: markup,
	}); err != nil {
		b.Log.Warn("edit confirmation", "err", err)
	}
}
