package bot

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/inputrules"
	"github.com/aligh5331/personal-budget-manger/internal/jalali"
	"github.com/aligh5331/personal-budget-manger/internal/storage"
)

// The [Edit] flow (#39). [Edit] turns the message it sits on (the host:
// a confirmation, or the /transactions record view) into a sub-menu of
// fields. Direction is answered with buttons on the host; Amount, Date and
// Description send a prompt the Owner replies to. Either way the
// Transaction is saved and the host is redrawn with the new values.
//
// A host is a TransactionView. Another message that shows a Transaction
// registers its own view with RegisterTransactionView and puts an
// EditButton on it; the whole flow then works on that message too.

// TransactionView is a message kind that shows one Transaction with
// buttons, so the edit flow can redraw it.
type TransactionView struct {
	// Key tells views apart in callback_data: one or two lowercase letters,
	// "" for the confirmation.
	Key string
	// Render returns the message text and its usual buttons for tx.
	Render func(ctx context.Context, b *Bot, tx storage.Transaction) (string, *bale.InlineKeyboardMarkup)
}

var transactionViews = map[string]TransactionView{}

// Edit actions, before the view suffix (see editAction).
const (
	actEdit         = "edit"
	actEditAmount   = "edit.amount"
	actEditDir      = "edit.dir"
	actEditDate     = "edit.date"
	actEditDesc     = "edit.desc"
	actEditBack     = "edit.back"
	actSetDirPrefix = "edit.dir."
)

var directionChoices = []struct {
	label string
	dir   storage.Direction
}{
	{"Expense", storage.DirectionOut},
	{"Income", storage.DirectionIn},
	{"Internal", storage.DirectionInternal},
}

// RegisterTransactionView adds a host view and the edit actions for it. It
// panics on a duplicate key; call it from init.
func RegisterTransactionView(v TransactionView) {
	if _, dup := transactionViews[v.Key]; dup {
		panic(fmt.Sprintf("bot: transaction view %q registered twice", v.Key))
	}
	transactionViews[v.Key] = v
	key := v.Key
	RegisterTransactionAction(editAction(actEdit, key), func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
		return b.OpenEditMenu(ctx, q.Message, id, key)
	})
	RegisterTransactionAction(editAction(actEditBack, key), func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
		return b.editBack(ctx, q.Message, id, key)
	})
	RegisterTransactionAction(editAction(actEditDir, key), func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
		return b.editDirectionMenu(ctx, q.Message, id, key)
	})
	for _, c := range directionChoices {
		dir := c.dir
		RegisterTransactionAction(editAction(actSetDirPrefix+string(dir), key), func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
			return b.setDirection(ctx, q.Message, id, key, dir)
		})
	}
	for act, field := range map[string]string{
		actEditAmount: storage.EditFieldAmount,
		actEditDate:   storage.EditFieldDate,
		actEditDesc:   storage.EditFieldDescription,
	} {
		RegisterTransactionAction(editAction(act, key), func(ctx context.Context, b *Bot, q *bale.CallbackQuery, id int64) (string, error) {
			return b.askEditValue(ctx, q.Message, id, key, field)
		})
	}
}

// editAction is the Transaction action for act on view key: "edit" on the
// confirmation, "edit@r" on view "r".
func editAction(act, viewKey string) string {
	if viewKey == "" {
		return act
	}
	return act + "@" + viewKey
}

// EditButton is the [Edit] button for Transaction id shown in view viewKey.
func EditButton(id int64, viewKey string) bale.InlineKeyboardButton {
	return bale.InlineKeyboardButton{Text: "Edit", CallbackData: TransactionData(id, editAction(actEdit, viewKey))}
}

const (
	goneToast    = "This transaction was removed."
	updatedToast = "Updated"
)

// OpenEditMenu turns host, a message showing Transaction id in view viewKey,
// into the field sub-menu. It returns the toast for the tap.
func (b *Bot) OpenEditMenu(ctx context.Context, host *bale.Message, id int64, viewKey string) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil || !ok {
		return goneToast, err
	}
	btn := func(label, act string) bale.InlineKeyboardButton {
		return bale.InlineKeyboardButton{Text: label, CallbackData: TransactionData(id, editAction(act, viewKey))}
	}
	markup := &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{
		{btn("Amount", actEditAmount), btn("Direction", actEditDir)},
		{btn("Date", actEditDate), btn("Description", actEditDesc)},
		{btn("Back", actEditBack)},
	}}
	return "", b.redrawWith(ctx, host, tx, viewKey, markup)
}

// editBack puts the host's usual buttons back.
func (b *Bot) editBack(ctx context.Context, host *bale.Message, id int64, viewKey string) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil || !ok {
		return goneToast, err
	}
	return "", b.redraw(ctx, host.Chat.ID, host.MessageID, tx, viewKey)
}

func (b *Bot) editDirectionMenu(ctx context.Context, host *bale.Message, id int64, viewKey string) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil || !ok {
		return goneToast, err
	}
	row := make([]bale.InlineKeyboardButton, 0, len(directionChoices))
	for _, c := range directionChoices {
		row = append(row, bale.InlineKeyboardButton{Text: c.label, CallbackData: TransactionData(id, editAction(actSetDirPrefix+string(c.dir), viewKey))})
	}
	markup := &bale.InlineKeyboardMarkup{InlineKeyboard: [][]bale.InlineKeyboardButton{
		row,
		{{Text: "Back", CallbackData: TransactionData(id, editAction(actEdit, viewKey))}},
	}}
	return "", b.redrawWith(ctx, host, tx, viewKey, markup)
}

func (b *Bot) setDirection(ctx context.Context, host *bale.Message, id int64, viewKey string, dir storage.Direction) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil || !ok {
		return goneToast, err
	}
	if err := b.applyDirection(ctx, &tx, dir); err != nil {
		return "Couldn't save that, try again.", err
	}
	if toast, err := b.saveEdit(ctx, tx); toast != "" || err != nil {
		return toast, err
	}
	if host != nil {
		if err := b.redraw(ctx, host.Chat.ID, host.MessageID, tx, viewKey); err != nil {
			b.Log.Warn("edit: redraw", "err", err)
		}
	}
	return updatedToast, nil
}

// Prompts for the typed fields, and the reply to a value that doesn't parse.
var editPrompts = map[string]struct{ ask, invalid string }{
	storage.EditFieldAmount: {
		ask:     "New amount, in toman? For example 450,000 or ۴۵۰ هزار (450 alone means 450,000). Reply to this message.",
		invalid: "Couldn't read an amount in that. Reply to the question with an amount in toman, like 450,000.",
	},
	storage.EditFieldDate: {
		ask:     "New date? A Jalali date like 1405/07/04, 4 Mehr or دیروز. Reply to this message.",
		invalid: "Couldn't read a Jalali date up to today in that. Reply to the question with a date like 1405/07/04 or 4 Mehr.",
	},
	storage.EditFieldDescription: {
		ask:     "New description? Send - to clear it. Reply to this message.",
		invalid: fmt.Sprintf("That's too long. Reply to the question with a description of at most %d characters.", maxDescriptionRunes),
	},
}

const maxDescriptionRunes = 200

// askEditValue puts the host's usual buttons back and asks for the new
// value of field as a reply.
func (b *Bot) askEditValue(ctx context.Context, host *bale.Message, id int64, viewKey, field string) (string, error) {
	tx, ok, err := b.Transactions.TransactionByID(ctx, id)
	if err != nil || !ok {
		return goneToast, err
	}
	if host == nil {
		return staleButtonToast, nil
	}
	if err := b.redraw(ctx, host.Chat.ID, host.MessageID, tx, viewKey); err != nil {
		b.Log.Warn("edit: redraw", "err", err)
	}
	prompt, err := b.Send(ctx, host.Chat.ID, editPrompts[field].ask, nil)
	if err != nil {
		return "", err
	}
	return "", b.EditPrompts.SaveEditPrompt(ctx, storage.EditPrompt{
		ChatID:          host.Chat.ID,
		PromptMessageID: prompt.MessageID,
		TransactionID:   id,
		Field:           field,
		View:            viewKey,
		HostMessageID:   host.MessageID,
		CreatedAt:       b.Clock.Now(),
	})
}

func init() { RegisterReplyHandler(handleEditReply) }

// handleEditReply takes the Owner's reply to an edit prompt as the new
// value. A value that doesn't parse gets a hint; the prompt stays open.
func handleEditReply(ctx context.Context, b *Bot, m *bale.Message) (bool, error) {
	if b.EditPrompts == nil {
		return false, nil
	}
	p, ok, err := b.EditPrompts.EditPromptByMessage(ctx, m.Chat.ID, m.ReplyToMessage.MessageID)
	if err != nil || !ok {
		return false, err
	}
	tx, ok, err := b.Transactions.TransactionByID(ctx, p.TransactionID)
	if err != nil {
		return true, err
	}
	if !ok {
		return true, b.Reply(ctx, m, goneToast, nil)
	}
	if !b.applyTypedValue(&tx, p.Field, m.Text) {
		return true, b.Reply(ctx, m, editPrompts[p.Field].invalid, nil)
	}
	if toast, err := b.saveEdit(ctx, tx); toast != "" || err != nil {
		if err == nil {
			err = b.Reply(ctx, m, toast, nil)
		}
		return true, err
	}
	if err := b.redraw(ctx, p.ChatID, p.HostMessageID, tx, p.View); err != nil {
		b.Log.Warn("edit: redraw", "err", err)
	}
	return true, b.Reply(ctx, m, updatedToast+".", nil)
}

// applyTypedValue sets field on tx from the Owner's text. It reports false
// when the text is not a valid value.
func (b *Bot) applyTypedValue(tx *storage.Transaction, field, text string) bool {
	switch field {
	case storage.EditFieldAmount:
		amount, ok := inputrules.TypedAmount(text)
		if !ok {
			return false
		}
		// Flagged for a missing amount (or a Follow-up that ended without it).
		if tx.Flagged && tx.AmountToman == nil {
			tx.Flagged, tx.FlagReason = false, ""
		}
		tx.AmountToman = &amount
	case storage.EditFieldDate:
		d, ok := inputrules.TypedDate(text, b.Clock.Now())
		if !ok {
			return false
		}
		tx.OccurredAt = withDate(tx.OccurredAt, d)
	case storage.EditFieldDescription:
		desc := strings.TrimSpace(inputrules.Normalize(text))
		if desc == "-" {
			desc = ""
		}
		if utf8.RuneCountInString(desc) > maxDescriptionRunes {
			return false
		}
		tx.Description = desc
	default:
		return false
	}
	return true
}

// withDate moves t to Jalali date d, keeping its Tehran time of day.
func withDate(t time.Time, d jalali.Date) time.Time {
	t = t.In(clock.Tehran())
	return d.At(t.Hour(), t.Minute(), clock.Tehran())
}

// applyDirection sets tx's Direction. A different kind resets the Category
// to Uncategorized; internal clears it. Either way a pending background
// re-categorize is dropped. Picking a Direction settles an ambiguous one.
func (b *Bot) applyDirection(ctx context.Context, tx *storage.Transaction, dir storage.Direction) error {
	if tx.Flagged && tx.AmountToman != nil {
		tx.Flagged, tx.FlagReason = false, ""
	}
	if dir == tx.Direction {
		return nil
	}
	tx.Direction = dir
	tx.CategorizePending = false
	if dir == storage.DirectionInternal {
		tx.CategoryID = nil
		return nil
	}
	id, err := b.uncategorizedID(ctx)
	if err != nil {
		return err
	}
	tx.CategoryID = id
	return nil
}

// uncategorizedID is the CategoryID of the built-in Uncategorized row.
func (b *Bot) uncategorizedID(ctx context.Context) (*int64, error) {
	c, err := b.Categories.Uncategorized(ctx)
	if err != nil {
		return nil, err
	}
	return &c.ID, nil
}

// saveEdit stores tx. A non-empty toast means it could not be saved.
func (b *Bot) saveEdit(ctx context.Context, tx storage.Transaction) (string, error) {
	updated, err := b.Transactions.UpdateTransaction(ctx, tx)
	if err != nil {
		return "Couldn't save that, try again.", err
	}
	if !updated {
		return goneToast, nil
	}
	return "", nil
}

// redraw shows tx in its view, with the view's usual buttons, in message
// messageID of chatID.
func (b *Bot) redraw(ctx context.Context, chatID, messageID int64, tx storage.Transaction, viewKey string) error {
	v, ok := transactionViews[viewKey]
	if !ok {
		return fmt.Errorf("edit: unknown view %q", viewKey)
	}
	text, markup := v.Render(ctx, b, tx)
	return b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: markup})
}

// redrawWith shows tx in host with markup instead of the view's buttons.
func (b *Bot) redrawWith(ctx context.Context, host *bale.Message, tx storage.Transaction, viewKey string, markup *bale.InlineKeyboardMarkup) error {
	if host == nil {
		return nil
	}
	v, ok := transactionViews[viewKey]
	if !ok {
		return fmt.Errorf("edit: unknown view %q", viewKey)
	}
	text, _ := v.Render(ctx, b, tx)
	return b.Bale.EditMessageText(ctx, bale.EditMessageTextParams{ChatID: host.Chat.ID, MessageID: host.MessageID, Text: text, ReplyMarkup: markup})
}
