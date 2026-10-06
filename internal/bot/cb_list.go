package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// Buttons of /transactions (#40). List callback_data is
//
//	l:p:<page>      go to a page of the list the message shows
//	l:f:<code>      switch filter (m, l, a, x), back to page 0
//	l:c             open the Category filter picker
//	l:c:<cat id>    filter by that Category
//	l:r:<tx id>     open the record view
//	l:b             back from the record view to the same page and filter
//
// All of them need the message's in-memory listState; without it (after a
// restart) they answer staleListToast. The record view is a TransactionView
// with key "r", so [Edit] and [Category] work on it; Delete uses
// t:<id>:del@r, del.yes@r and del.no@r.

const (
	listPrefix     = "l"
	staleListToast = "This list expired, send /transactions"
	recordViewKey  = "r"
	actDelete      = "del"
	actDeleteYes   = "del.yes"
	actDeleteNo    = "del.no"
)

func init() {
	RegisterCallback(Callback{Prefix: listPrefix, Run: runListCallback})
	RegisterTransactionView(TransactionView{Key: recordViewKey, Render: renderRecord})
	registerCategoryPicker(recordViewKey)
	for act, fn := range map[string]func(*Bot, context.Context, *bale.CallbackQuery, int64) (string, error){
		actDelete:    (*Bot).askDelete,
		actDeleteYes: (*Bot).confirmDelete,
		actDeleteNo:  (*Bot).cancelDelete,
	} {
		RegisterTransactionAction(editAction(act, recordViewKey), func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
			return fn(b, ctx, q, id)
		})
	}
}

func runListCallback(ctx context.Context, b *Bot, q *bale.CallbackQuery) (string, error) {
	if q.Message == nil {
		return staleListToast, nil
	}
	st, ok := b.listStates().get(q.Message.Chat.ID, q.Message.MessageID)
	if !ok {
		return staleListToast, nil
	}
	data := ParseCallbackData(q.Data)
	if data.Len() < 2 {
		return staleListToast, nil
	}
	arg := data.Rest(2)
	switch data.Part(1) {
	case "p":
		page, err := strconv.Atoi(arg)
		if err != nil || page < 0 {
			return staleListToast, nil
		}
		st.page = page
	case "f":
		switch arg {
		case filterThisMonth, filterLastMonth, filterAll, filterFlagged:
			st.filter, st.page = arg, 0
		default:
			return staleListToast, nil
		}
	case "c":
		if arg == "" {
			return b.openListCategoryPicker(ctx, q, st)
		}
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return staleListToast, nil
		}
		if _, ok, err := b.Categories.CategoryByID(ctx, id); err != nil || !ok {
			return unavailableText, err
		}
		st.filter, st.categoryID, st.page = filterCategory, id, 0
	case "r":
		id, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			return staleListToast, nil
		}
		return b.openRecord(ctx, q, st, id)
	case "b":
	default:
		return staleListToast, nil
	}
	return "", b.showList(ctx, q, st)
}

// showList redraws the tapped message as the list for st.
func (b *Bot) showList(ctx context.Context, q *bale.CallbackQuery, st *listState) error {
	text, markup, err := b.renderList(ctx, st)
	if err != nil {
		return err
	}
	b.editMessage(ctx, q, text, markup)
	return nil
}

func (b *Bot) openListCategoryPicker(ctx context.Context, q *bale.CallbackQuery, st *listState) (string, error) {
	var cats []storage.Category
	for _, kind := range []string{storage.KindExpense, storage.KindIncome} {
		cs, err := b.Categories.ActiveCategories(ctx, kind)
		if err != nil {
			return "Something went wrong, try again.", err
		}
		cats = append(cats, cs...)
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
		if st.filter == filterCategory && st.categoryID == c.ID {
			label = currentMarker + label
		}
		row = append(row, bale.InlineKeyboardButton{Text: label, CallbackData: BuildCallbackData(listPrefix, "c", c.ID)})
		if len(row) == pickerPerRow {
			rows, row = append(rows, row), nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []bale.InlineKeyboardButton{{Text: "Back", CallbackData: BuildCallbackData(listPrefix, "p", st.page)}})
	b.editMessage(ctx, q, "Show which Category?", &bale.InlineKeyboardMarkup{InlineKeyboard: rows})
	return "", nil
}

func (b *Bot) openRecord(ctx context.Context, q *bale.CallbackQuery, st *listState, id int64) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil {
		return "Something went wrong, try again.", err
	}
	if !ok {
		// Deleted since the list was drawn: show the list as it is now.
		return removedToast, b.showList(ctx, q, st)
	}
	b.editView(ctx, q, tx, recordViewKey)
	return "", nil
}

// renderRecord is the full record: what was saved, the Owner's original
// text and the Flagged state, with [Edit] [Category] [Delete] and [Back to
// list].
func renderRecord(ctx context.Context, b *Bot, tx storage.Transaction) (string, *bale.InlineKeyboardMarkup) {
	return b.recordText(ctx, tx), recordMarkup(tx)
}

func (b *Bot) recordText(ctx context.Context, tx storage.Transaction) string {
	var sb strings.Builder
	amount := "amount unknown"
	if tx.AmountToman != nil {
		amount = FormatToman(*tx.AmountToman) + " toman"
	}
	fmt.Fprintf(&sb, "%s · %s\n", amount, directionText(tx))
	fmt.Fprintf(&sb, "Category: %s\n", b.categoryText(ctx, tx))
	fmt.Fprintf(&sb, "Date: %s\n", FormatDate(tx.OccurredAt))
	if tx.Description != "" {
		fmt.Fprintf(&sb, "Note: %s\n", tx.Description)
	}
	if tx.BankLabel != "" {
		fmt.Fprintf(&sb, "Bank text: «%s»\n", tx.BankLabel)
	}
	if tx.Flagged {
		fmt.Fprintf(&sb, "Flagged: %s (not counted until fixed)\n", flagReasonText(tx.FlagReason))
	}
	fmt.Fprintf(&sb, "Original text:\n%s", tx.RawText)
	return sb.String()
}

func recordMarkup(tx storage.Transaction) *bale.InlineKeyboardMarkup {
	row := []bale.InlineKeyboardButton{EditButton(tx.ID, recordViewKey)}
	if tx.Direction != storage.DirectionInternal {
		row = append(row, bale.InlineKeyboardButton{Text: "Category", CallbackData: TransactionData(tx.ID, editAction(catAction, recordViewKey))})
	}
	row = append(row, bale.InlineKeyboardButton{Text: "Delete", CallbackData: TransactionData(tx.ID, editAction(actDelete, recordViewKey))})
	return &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{
		row,
		{{Text: "Back to list", CallbackData: BuildCallbackData(listPrefix, "b")}},
	}}
}

// askDelete turns the record's buttons into "Delete? [Yes] [No]".
func (b *Bot) askDelete(ctx context.Context, q *bale.CallbackQuery, id int64) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil || !ok {
		return goneToast, err
	}
	b.editMessage(ctx, q, "Delete? This can't be undone.\n\n"+b.recordText(ctx, tx), &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{{
		{Text: "Yes", CallbackData: TransactionData(id, editAction(actDeleteYes, recordViewKey))},
		{Text: "No", CallbackData: TransactionData(id, editAction(actDeleteNo, recordViewKey))},
	}}})
	return "", nil
}

func (b *Bot) cancelDelete(ctx context.Context, q *bale.CallbackQuery, id int64) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil || !ok {
		return goneToast, err
	}
	b.editView(ctx, q, tx, recordViewKey)
	return "", nil
}

// confirmDelete hard-deletes the Transaction and returns the message to the
// list it came from (or "Removed" when that list is gone).
func (b *Bot) confirmDelete(ctx context.Context, q *bale.CallbackQuery, id int64) (string, error) {
	if _, err := b.Transactions.DeleteTransaction(ctx, id); err != nil {
		return "Couldn't delete it, try again.", err
	}
	if q.Message != nil {
		if st, ok := b.listStates().get(q.Message.Chat.ID, q.Message.MessageID); ok {
			return "Deleted", b.showList(ctx, q, st)
		}
	}
	b.editMessage(ctx, q, "Removed", nil)
	return "Deleted", nil
}
